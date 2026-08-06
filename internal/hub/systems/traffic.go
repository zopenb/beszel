package systems

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"time"
	_ "time/tzdata"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

var shanghaiLocation = func() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		panic(err)
	}
	return location
}()

type trafficCheckpoint struct {
	Counters      map[string][2]string
	SampledAt     time.Time
	InitializedAt time.Time
	ScheduleKey   string
}

type trafficUsage struct {
	CycleStart           string `json:"cycle_start"`
	CycleEnd             string `json:"cycle_end"`
	SentBytes            string `json:"sent_bytes"`
	RecvBytes            string `json:"recv_bytes"`
	ObservedFrom         string `json:"observed_from"`
	LastSampleAt         string `json:"last_sample_at"`
	Complete             bool   `json:"complete"`
	ResetCount           int    `json:"reset_count"`
	InterfaceChangeCount int    `json:"interface_change_count"`
}

func trafficQuotaEnabled(value string) bool {
	quota, ok := new(big.Int).SetString(value, 10)
	return ok && quota.Sign() > 0
}

func clearTrafficAccounting(app core.App, systemRecord *core.Record) error {
	systemRecord.Set("traffic_usage", nil)
	_, err := app.DB().Delete("traffic_checkpoints", dbx.HashExp{"system": systemRecord.Id}).Execute()
	return err
}

func trafficAccountingNeedsReset(record, original *core.Record) bool {
	return !trafficQuotaEnabled(record.GetString("traffic_quota_bytes")) ||
		record.GetInt("traffic_cycle_day") != original.GetInt("traffic_cycle_day")
}

func updateTrafficUsage(app core.App, systemRecord *core.Record, interfaces map[string][4]uint64, sampledAt time.Time) error {
	quota := systemRecord.GetString("traffic_quota_bytes")
	if !trafficQuotaEnabled(quota) {
		return clearTrafficAccounting(app, systemRecord)
	}
	// A temporary collection failure must not replace a valid checkpoint with
	// an empty baseline or advance the accounting timestamp.
	if len(interfaces) == 0 {
		return nil
	}

	cycleDay := systemRecord.GetInt("traffic_cycle_day")
	if cycleDay < 1 || cycleDay > 31 {
		cycleDay = 1
		systemRecord.Set("traffic_cycle_day", cycleDay)
	}
	mode := systemRecord.GetString("traffic_count_mode")
	if mode != "egress" {
		mode = "combined"
		systemRecord.Set("traffic_count_mode", mode)
	}
	sampledAt = sampledAt.In(shanghaiLocation)
	cycleStart, cycleEnd := trafficCycle(sampledAt, cycleDay)
	scheduleKey := fmt.Sprintf("%02d", cycleDay)
	counters := trafficCounters(interfaces)

	checkpointRecord, checkpoint, err := loadTrafficCheckpoint(app, systemRecord.Id)
	if err != nil {
		return err
	}
	if checkpointRecord == nil || checkpoint.ScheduleKey != scheduleKey {
		usage := newTrafficUsage(new(big.Int), new(big.Int), cycleStart, cycleEnd, sampledAt, sampledAt, false, 0, 0)
		systemRecord.Set("traffic_usage", usage)
		return saveTrafficCheckpoint(app, checkpointRecord, systemRecord.Id, counters, sampledAt, sampledAt, scheduleKey)
	}
	// Duplicate and delayed samples must not replace a newer checkpoint or erase usage.
	if !sampledAt.After(checkpoint.SampledAt) {
		return nil
	}

	usage := getTrafficUsage(systemRecord)
	sent := decimalInt(usage.SentBytes)
	recv := decimalInt(usage.RecvBytes)
	sentDelta, recvDelta, resets, interfaceChanges := trafficDelta(checkpoint.Counters, counters)
	if sampledAt.Sub(checkpoint.SampledAt) > 5*time.Minute {
		checkpoint.InitializedAt = sampledAt
	}
	if checkpoint.SampledAt.Before(cycleStart) {
		sentDelta = splitTrafficDelta(sentDelta, checkpoint.SampledAt, sampledAt, cycleStart)
		recvDelta = splitTrafficDelta(recvDelta, checkpoint.SampledAt, sampledAt, cycleStart)
		sent.SetInt64(0)
		recv.SetInt64(0)
		usage.ResetCount = 0
		usage.InterfaceChangeCount = 0
	}
	sent.Add(sent, sentDelta)
	recv.Add(recv, recvDelta)
	initializedAt := checkpoint.InitializedAt
	if resets > 0 || interfaceChanges > 0 {
		initializedAt = sampledAt
	}
	complete := !initializedAt.After(cycleStart)
	systemRecord.Set("traffic_usage", newTrafficUsage(sent, recv, cycleStart, cycleEnd, initializedAt, sampledAt, complete, usage.ResetCount+resets, usage.InterfaceChangeCount+interfaceChanges))
	return saveTrafficCheckpoint(app, checkpointRecord, systemRecord.Id, counters, sampledAt, initializedAt, scheduleKey)
}

func trafficCycle(at time.Time, day int) (time.Time, time.Time) {
	at = at.In(shanghaiLocation)
	start := clampedDay(at.Year(), at.Month(), day)
	if at.Before(start) {
		previous := time.Date(at.Year(), at.Month()-1, 1, 0, 0, 0, 0, shanghaiLocation)
		start = clampedDay(previous.Year(), previous.Month(), day)
	}
	next := time.Date(start.Year(), start.Month()+1, 1, 0, 0, 0, 0, shanghaiLocation)
	return start, clampedDay(next.Year(), next.Month(), day)
}

func clampedDay(year int, month time.Month, day int) time.Time {
	lastDay := time.Date(year, month+1, 0, 0, 0, 0, 0, shanghaiLocation).Day()
	return time.Date(year, month, min(day, lastDay), 0, 0, 0, 0, shanghaiLocation)
}

func trafficCounters(interfaces map[string][4]uint64) map[string][2]string {
	counters := make(map[string][2]string, len(interfaces))
	for name, values := range interfaces {
		counters[name] = [2]string{strconv.FormatUint(values[2], 10), strconv.FormatUint(values[3], 10)}
	}
	return counters
}

func trafficDelta(previous, current map[string][2]string) (sent, recv *big.Int, resets, interfaceChanges int) {
	sent, recv = new(big.Int), new(big.Int)
	for name := range previous {
		if _, exists := current[name]; !exists {
			interfaceChanges++
		}
	}
	for name, currentValues := range current {
		previousValues, exists := previous[name]
		if !exists {
			interfaceChanges++
			continue
		}
		reset := false
		for i := range 2 {
			currentValue, currentOK := new(big.Int).SetString(currentValues[i], 10)
			previousValue, previousOK := new(big.Int).SetString(previousValues[i], 10)
			if !currentOK || !previousOK {
				continue
			}
			delta := new(big.Int)
			if currentValue.Cmp(previousValue) >= 0 {
				delta.Sub(currentValue, previousValue)
			} else {
				// A reset loses bytes between the prior sample and reset, but bytes since reset remain known.
				delta.Set(currentValue)
				reset = true
			}
			if i == 0 {
				sent.Add(sent, delta)
			} else {
				recv.Add(recv, delta)
			}
		}
		if reset {
			resets++
		}
	}
	return sent, recv, resets, interfaceChanges
}

func splitTrafficDelta(delta *big.Int, from, to, boundary time.Time) *big.Int {
	if !boundary.After(from) || !boundary.Before(to) {
		return delta
	}
	totalNanos := new(big.Int).SetInt64(to.Sub(from).Nanoseconds())
	currentNanos := new(big.Int).SetInt64(to.Sub(boundary).Nanoseconds())
	before := new(big.Int).Mul(new(big.Int).Set(delta), new(big.Int).Sub(totalNanos, currentNanos))
	before.Quo(before, totalNanos)
	return new(big.Int).Sub(delta, before)
}

func newTrafficUsage(sent, recv *big.Int, start, end, observedFrom, sampledAt time.Time, complete bool, resets, interfaceChanges int) trafficUsage {
	return trafficUsage{
		CycleStart: start.UTC().Format(time.RFC3339), CycleEnd: end.UTC().Format(time.RFC3339), SentBytes: sent.String(), RecvBytes: recv.String(),
		ObservedFrom: observedFrom.UTC().Format(time.RFC3339), LastSampleAt: sampledAt.UTC().Format(time.RFC3339), Complete: complete,
		ResetCount: resets, InterfaceChangeCount: interfaceChanges,
	}
}

func getTrafficUsage(record *core.Record) trafficUsage {
	var usage trafficUsage
	data, _ := json.Marshal(record.GetRaw("traffic_usage"))
	_ = json.Unmarshal(data, &usage)
	return usage

}

func decimalInt(value string) *big.Int {
	result, ok := new(big.Int).SetString(value, 10)
	if !ok || result.Sign() < 0 {
		return new(big.Int)
	}
	return result
}

func loadTrafficCheckpoint(app core.App, systemID string) (*core.Record, trafficCheckpoint, error) {
	record, err := app.FindFirstRecordByFilter("traffic_checkpoints", "system = {:system}", dbx.Params{"system": systemID})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, trafficCheckpoint{}, nil
		}
		return nil, trafficCheckpoint{}, err
	}
	checkpoint := trafficCheckpoint{
		SampledAt: record.GetDateTime("sampled_at").Time(), InitializedAt: record.GetDateTime("initialized_at").Time(),
		ScheduleKey: record.GetString("schedule_key"),
	}
	data, _ := json.Marshal(record.GetRaw("counters"))
	if err := json.Unmarshal(data, &checkpoint.Counters); err != nil {
		return nil, trafficCheckpoint{}, err
	}
	return record, checkpoint, nil
}

func saveTrafficCheckpoint(app core.App, record *core.Record, systemID string, counters map[string][2]string, sampledAt, initializedAt time.Time, scheduleKey string) error {
	if record == nil {
		collection, err := app.FindCachedCollectionByNameOrId("traffic_checkpoints")
		if err != nil {
			return err
		}
		record = core.NewRecord(collection)
		record.Set("system", systemID)
	}
	record.Set("counters", counters)
	record.Set("sampled_at", sampledAt)
	record.Set("initialized_at", initializedAt)
	record.Set("schedule_key", scheduleKey)
	return app.SaveNoValidate(record)
}
