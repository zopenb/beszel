//go:build testing

package alerts_test

import (
	"math/big"
	"testing"

	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/dbx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrafficQuotaAlertExactIntegerTransitions(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "up")
	require.NoError(t, err)
	systemRecord := systems[0]
	managedSystem, err := hub.GetSystemManager().GetSystemFromStore(systemRecord.Id)
	require.NoError(t, err)
	managedSystem.StopUpdater()

	settings, err := hub.FindFirstRecordByFilter("user_settings", "user={:user}", dbx.Params{"user": user.Id})
	require.NoError(t, err)
	settings.Set("settings", `{"emails":["test@example.com"],"webhooks":[]}`)
	require.NoError(t, hub.Save(settings))

	quota := "18446744073709551614"
	half, ok := new(big.Int).SetString(quota, 10)
	require.True(t, ok)
	half.Quo(half, big.NewInt(2))
	systemRecord.Set("traffic_quota_bytes", quota)
	systemRecord.Set("traffic_count_mode", "combined")
	systemRecord.Set("traffic_usage", map[string]any{"sent_bytes": new(big.Int).Sub(new(big.Int).Set(half), big.NewInt(1)).String(), "recv_bytes": "0"})
	require.NoError(t, hub.Save(systemRecord))
	systemRecord, err = hub.FindRecordById("systems", systemRecord.Id)
	require.NoError(t, err)

	alertRecord, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name": "TrafficQuota", "system": systemRecord.Id, "user": user.Id, "value": 50, "min": 1,
	})
	require.NoError(t, err)
	require.NoError(t, hub.HandleTrafficQuotaAlerts(systemRecord))
	assert.False(t, alertRecord.GetBool("triggered"))
	assert.Zero(t, hub.TestMailer.TotalSend())

	systemRecord.Set("traffic_usage", map[string]any{
		"sent_bytes": half.String(), "recv_bytes": "0", "cycle_start": "2026-08-05T16:00:00Z", "cycle_end": "2026-09-05T16:00:00Z", "complete": false,
	})
	require.NoError(t, hub.Save(systemRecord))
	alertRecord, err = hub.FindRecordById("alerts", alertRecord.Id)
	require.NoError(t, err)
	assert.True(t, alertRecord.GetBool("triggered"), "equality must trigger without float rounding")
	assert.Equal(t, 1, hub.TestMailer.TotalSend())
	message := hub.TestMailer.LastMessage().Text
	assert.Contains(t, message, half.String()+" / "+quota+" bytes")
	assert.Contains(t, message, "2026-08-06 00:00 CST to 2026-09-06 00:00 CST")
	assert.Contains(t, message, "Accounting is incomplete")

	require.NoError(t, hub.HandleSystemAlerts(systemRecord, nil))
	assert.Equal(t, 1, hub.TestMailer.TotalSend(), "unchanged state must not notify twice")

	systemRecord.Set("traffic_usage", map[string]any{"sent_bytes": new(big.Int).Sub(new(big.Int).Set(half), big.NewInt(1)).String(), "recv_bytes": "0"})
	require.NoError(t, hub.Save(systemRecord))
	alertRecord, err = hub.FindRecordById("alerts", alertRecord.Id)
	require.NoError(t, err)
	assert.False(t, alertRecord.GetBool("triggered"))
	assert.Equal(t, 2, hub.TestMailer.TotalSend())

	history, err := hub.FindFirstRecordByFilter("alerts_history", "alert_id={:alert}", dbx.Params{"alert": alertRecord.Id})
	require.NoError(t, err)
	assert.False(t, history.GetDateTime("resolved").IsZero())
}

func TestTrafficQuotaAlertResolvesWhenUnavailable(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "up")
	require.NoError(t, err)
	systemRecord := systems[0]
	managedSystem, err := hub.GetSystemManager().GetSystemFromStore(systemRecord.Id)
	require.NoError(t, err)
	managedSystem.StopUpdater()
	systemRecord, err = hub.FindRecordById("systems", systemRecord.Id)
	require.NoError(t, err)

	systemRecord.Set("traffic_quota_bytes", "100")
	systemRecord.Set("traffic_count_mode", "egress")
	systemRecord.Set("traffic_usage", map[string]any{"sent_bytes": "100", "recv_bytes": "999"})
	require.NoError(t, hub.Save(systemRecord))
	systemRecord, err = hub.FindRecordById("systems", systemRecord.Id)
	require.NoError(t, err)
	alertRecord, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name": "TrafficQuota", "system": systemRecord.Id, "user": user.Id, "value": 100, "min": 1,
	})
	require.NoError(t, err)
	require.NoError(t, hub.HandleTrafficQuotaAlerts(systemRecord))
	alertRecord, err = hub.FindRecordById("alerts", alertRecord.Id)
	require.NoError(t, err)
	require.True(t, alertRecord.GetBool("triggered"))

	systemRecord.Set("traffic_usage", nil)
	require.NoError(t, hub.Save(systemRecord))
	alertRecord, err = hub.FindRecordById("alerts", alertRecord.Id)
	require.NoError(t, err)
	assert.False(t, alertRecord.GetBool("triggered"), "missing usage must resolve")

	alertRecord.Set("triggered", true)
	require.NoError(t, hub.Save(alertRecord))
	systemRecord.Set("traffic_quota_bytes", "0")
	require.NoError(t, hub.Save(systemRecord))
	alertRecord, err = hub.FindRecordById("alerts", alertRecord.Id)
	require.NoError(t, err)
	assert.False(t, alertRecord.GetBool("triggered"), "disabled quota must resolve")
}
