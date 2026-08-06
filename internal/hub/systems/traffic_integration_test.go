//go:build testing

package systems_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/hub/systems"
	_ "github.com/henrygd/beszel/internal/migrations"
	"github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/pocketbase/core"
	pbtests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type trafficUsage struct {
	SentBytes string `json:"sent_bytes"`
	RecvBytes string `json:"recv_bytes"`
	Complete  bool   `json:"complete"`
}

func TestTrafficAccountingLifecycle(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()
	require.NoError(t, hub.GetSystemManager().Initialize())

	user, err := tests.CreateUser(hub, "traffic@example.com", "password123")
	require.NoError(t, err)
	record, err := tests.CreateRecord(hub, "systems", map[string]any{
		"name": "traffic", "host": "127.0.0.1", "users": []string{user.Id},
		"traffic_quota_bytes": "18446744073709551615", "traffic_cycle_day": 6,
	})
	require.NoError(t, err)
	require.Equal(t, "18446744073709551615", record.GetString("traffic_quota_bytes"))
	_, err = hub.FindCollectionByNameOrId("traffic_checkpoints")
	require.NoError(t, err)

	shanghai, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	first := time.Date(2026, 8, 6, 0, 1, 0, 0, shanghai)
	require.NoError(t, systems.UpdateTrafficUsageAt(hub, record, map[string][4]uint64{"eth0": {0, 0, 100, 200}}, first))
	usage := readTrafficUsage(t, record)
	assert.Equal(t, "0", usage.SentBytes)
	assert.Equal(t, "0", usage.RecvBytes)
	assert.False(t, usage.Complete)
	count, err := hub.CountRecords("traffic_checkpoints")
	require.NoError(t, err)
	assert.Equal(t, 1, int(count))

	require.NoError(t, systems.UpdateTrafficUsageAt(hub, record, map[string][4]uint64{"eth0": {0, 0, 130, 270}}, first.Add(time.Minute)))
	usage = readTrafficUsage(t, record)
	assert.Equal(t, "30", usage.SentBytes)
	assert.Equal(t, "70", usage.RecvBytes)

	// Duplicate and delayed samples must not erase accumulated usage or replace the checkpoint.
	require.NoError(t, systems.UpdateTrafficUsageAt(hub, record, map[string][4]uint64{"eth0": {0, 0, 110, 220}}, first))
	usage = readTrafficUsage(t, record)
	assert.Equal(t, "30", usage.SentBytes)
	assert.Equal(t, "70", usage.RecvBytes)

	// Missing interface data must not advance or replace the valid checkpoint.
	require.NoError(t, systems.UpdateTrafficUsageAt(hub, record, nil, first.Add(90*time.Second)))
	require.NoError(t, systems.UpdateTrafficUsageAt(hub, record, map[string][4]uint64{"eth0": {0, 0, 140, 290}}, first.Add(2*time.Minute)))
	usage = readTrafficUsage(t, record)
	assert.Equal(t, "40", usage.SentBytes)
	assert.Equal(t, "90", usage.RecvBytes)

	// Quota and mode changes preserve accumulated usage.
	record.Set("traffic_quota_bytes", "999")
	record.Set("traffic_count_mode", "egress")
	require.NoError(t, hub.Save(record))
	require.NoError(t, systems.UpdateTrafficUsageAt(hub, record, map[string][4]uint64{"eth0": {0, 0, 150, 370}}, first.Add(3*time.Minute)))
	usage = readTrafficUsage(t, record)
	assert.Equal(t, "50", usage.SentBytes)
	assert.Equal(t, "170", usage.RecvBytes)

	// A rejected billing-day update must preserve both usage and checkpoint.
	record.Set("traffic_cycle_day", 7)
	record.Set("name", "")
	require.Error(t, hub.Save(record))
	record, err = hub.FindRecordById("systems", record.Id)
	require.NoError(t, err)
	usage = readTrafficUsage(t, record)
	assert.Equal(t, "50", usage.SentBytes)
	count, err = hub.CountRecords("traffic_checkpoints")
	require.NoError(t, err)
	assert.Equal(t, 1, int(count))

	// Billing-day changes and disabling clear both projection and checkpoint.
	record.Set("traffic_cycle_day", 7)
	require.NoError(t, hub.Save(record))
	assert.Equal(t, "null", record.GetString("traffic_usage"))
	count, err = hub.CountRecords("traffic_checkpoints")
	require.NoError(t, err)
	assert.Zero(t, count)

	record.Set("traffic_quota_bytes", "0")
	require.NoError(t, hub.Save(record))
	assert.Equal(t, "null", record.GetString("traffic_usage"))
}

func TestTrafficAccountingCrossesCycleBoundary(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()
	require.NoError(t, hub.GetSystemManager().Initialize())

	user, err := tests.CreateUser(hub, "boundary@example.com", "password123")
	require.NoError(t, err)
	record, err := tests.CreateRecord(hub, "systems", map[string]any{
		"name": "boundary", "host": "127.0.0.2", "users": []string{user.Id},
		"traffic_quota_bytes": "1000", "traffic_cycle_day": 6,
	})
	require.NoError(t, err)
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	before := time.Date(2026, 8, 5, 23, 59, 0, 0, shanghai)
	require.NoError(t, systems.UpdateTrafficUsageAt(hub, record, map[string][4]uint64{"eth0": {0, 0, 100, 100}}, before))
	require.NoError(t, systems.UpdateTrafficUsageAt(hub, record, map[string][4]uint64{"eth0": {0, 0, 106, 104}}, before.Add(3*time.Minute)))

	usage := readTrafficUsage(t, record)
	assert.Equal(t, "4", usage.SentBytes)
	assert.Equal(t, "3", usage.RecvBytes)
	assert.True(t, usage.Complete)
}

func TestTrafficCheckpointCollectionIsPrivateAndUnique(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()
	collection, err := hub.FindCollectionByNameOrId("traffic_checkpoints")
	require.NoError(t, err)
	assert.Nil(t, collection.ListRule)
	assert.Nil(t, collection.ViewRule)
	assert.Nil(t, collection.CreateRule)
	assert.Nil(t, collection.UpdateRule)
	assert.Nil(t, collection.DeleteRule)
	require.Len(t, collection.Indexes, 1)
	assert.Contains(t, collection.Indexes[0], "UNIQUE")
	assert.Contains(t, collection.Indexes[0], "`system`")
}

func TestTrafficUsageCannotBeMutatedThroughAPI(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()
	require.NoError(t, hub.GetSystemManager().Initialize())
	user, err := tests.CreateUser(hub, "api-traffic@example.com", "password123")
	require.NoError(t, err)
	record, err := tests.CreateRecord(hub, "systems", map[string]any{
		"name": "api-traffic", "host": "127.0.0.3", "users": []string{user.Id},
		"traffic_quota_bytes": "1000",
	})
	require.NoError(t, err)
	require.NoError(t, systems.UpdateTrafficUsageAt(hub, record, map[string][4]uint64{"eth0": {0, 0, 10, 20}}, time.Now()))
	token, err := user.NewAuthToken()
	require.NoError(t, err)

	scenario := tests.ApiScenario{
		Name:               "traffic usage is server managed",
		Method:             http.MethodPatch,
		URL:                "/api/collections/systems/records/" + record.Id,
		Body:               bytes.NewBufferString(`{"traffic_usage":{"sent_bytes":"999999"}}`),
		Headers:            map[string]string{"Authorization": token, "Content-Type": "application/json"},
		ExpectedStatus:     http.StatusOK,
		ExpectedContent:    []string{`"sent_bytes":"0"`},
		NotExpectedContent: []string{`"sent_bytes":"999999"`},
		TestAppFactory:     func(testing.TB) *pbtests.TestApp { return hub.TestApp },
	}
	scenario.Test(t)
	fresh, err := hub.FindRecordById("systems", record.Id)
	require.NoError(t, err)
	assert.Equal(t, "0", readTrafficUsage(t, fresh).SentBytes)
}

func readTrafficUsage(t *testing.T, record *core.Record) trafficUsage {
	t.Helper()
	data, err := json.Marshal(record.GetRaw("traffic_usage"))
	require.NoError(t, err)
	var usage trafficUsage
	require.NoError(t, json.Unmarshal(data, &usage))
	return usage
}
