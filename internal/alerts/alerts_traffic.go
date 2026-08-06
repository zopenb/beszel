package alerts

import (
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

type trafficUsage struct {
	SentBytes  string `json:"sent_bytes"`
	RecvBytes  string `json:"recv_bytes"`
	CycleStart string `json:"cycle_start"`
	CycleEnd   string `json:"cycle_end"`
	Complete   bool   `json:"complete"`
}

func (am *AlertManager) handleTrafficQuotaAlerts(systemRecord *core.Record) error {
	am.trafficMu.Lock()
	defer am.trafficMu.Unlock()

	alerts := am.alertsCache.GetAlertsByName(systemRecord.Id, "TrafficQuota")
	if len(alerts) == 0 {
		return nil
	}
	usage, quota, details, available := trafficQuotaUsage(systemRecord)
	for _, alert := range alerts {
		threshold := int64(alert.Value)
		triggered := available && alert.Value == float64(threshold) && threshold >= 1 && threshold <= 100 &&
			new(big.Int).Mul(new(big.Int).Set(usage), big.NewInt(100)).Cmp(new(big.Int).Mul(new(big.Int).Set(quota), big.NewInt(threshold))) >= 0
		if triggered == alert.Triggered {
			continue
		}
		if err := am.setAlertTriggered(alert, triggered); err != nil {
			return err
		}
		percent := new(big.Rat)
		if available {
			percent.Mul(new(big.Rat).SetFrac(usage, quota), big.NewRat(100, 1))
		}
		state := "reached"
		message := fmt.Sprintf("Traffic usage is %s%% of quota: %s / %s bytes (threshold %d%%).", percent.FloatString(2), usage.String(), quota.String(), threshold)
		if cycle := trafficCycleDescription(details); cycle != "" {
			message += " Billing cycle: " + cycle + " (Asia/Shanghai)."
		}
		if available && !details.Complete {
			message += " Accounting is incomplete because observation started or changed during this cycle."
		}
		if !triggered {
			state = "fell below"
			if !available {
				message = fmt.Sprintf("Traffic quota accounting is unavailable (threshold %d%%).", threshold)
			}
		}
		systemName := systemRecord.GetString("name")
		if err := am.SendAlert(AlertMessageData{
			UserID: alert.UserID, SystemID: systemRecord.Id,
			Title:   fmt.Sprintf("%s traffic quota %s threshold", systemName, state),
			Message: message,
			Link:    am.hub.MakeLink("system", systemRecord.Id), LinkText: "View " + systemName,
		}); err != nil {
			am.hub.Logger().Error("Failed to send traffic quota alert", "err", err)
		}
	}
	return nil
}

func (am *AlertManager) HandleTrafficQuotaAlerts(systemRecord *core.Record) error {
	return am.handleTrafficQuotaAlerts(systemRecord)
}

func trafficQuotaUsage(systemRecord *core.Record) (usage, quota *big.Int, details trafficUsage, available bool) {
	quota, ok := new(big.Int).SetString(systemRecord.GetString("traffic_quota_bytes"), 10)
	if !ok || quota.Sign() <= 0 {
		return new(big.Int), new(big.Int), details, false
	}
	data, err := json.Marshal(systemRecord.GetRaw("traffic_usage"))
	if err != nil || json.Unmarshal(data, &details) != nil {
		return new(big.Int), quota, details, false
	}
	sent, sentOK := new(big.Int).SetString(details.SentBytes, 10)
	recv, recvOK := new(big.Int).SetString(details.RecvBytes, 10)
	if !sentOK || sent.Sign() < 0 || !recvOK || recv.Sign() < 0 {
		return new(big.Int), quota, details, false
	}
	usage = new(big.Int).Set(sent)
	if systemRecord.GetString("traffic_count_mode") != "egress" {
		usage.Add(usage, recv)
	}
	return usage, quota, details, true
}

func trafficCycleDescription(usage trafficUsage) string {
	start, startErr := time.Parse(time.RFC3339, usage.CycleStart)
	end, endErr := time.Parse(time.RFC3339, usage.CycleEnd)
	if startErr != nil || endErr != nil {
		return ""
	}
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return ""
	}
	return start.In(location).Format("2006-01-02 15:04 MST") + " to " + end.In(location).Format("2006-01-02 15:04 MST")
}
