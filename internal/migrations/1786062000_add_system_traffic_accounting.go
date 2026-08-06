package migrations

import (
	"errors"
	"slices"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

const (
	trafficQuotaFieldID     = "traffic_quota"
	trafficCycleDayFieldID  = "traffic_cycle"
	trafficCountModeFieldID = "traffic_mode"
	trafficUsageFieldID     = "traffic_usage"
	trafficCheckpointsID    = "traffic_ckpts"
	trafficCheckpointRelID  = "traffic_system"
	trafficCheckpointDataID = "traffic_counts"
	trafficCheckpointAtID   = "traffic_sampled"
	trafficInitializedAtID  = "traffic_init"
	trafficScheduleKeyID    = "traffic_sched"
)

func init() {
	m.Register(func(app core.App) error {
		systems, err := app.FindCollectionByNameOrId("systems")
		if err != nil {
			return err
		}
		minDay, maxDay := float64(1), float64(31)
		systems.Fields.Add(
			&core.TextField{Id: trafficQuotaFieldID, Name: "traffic_quota_bytes", Pattern: `^[0-9]*$`, Max: 20},
			&core.NumberField{Id: trafficCycleDayFieldID, Name: "traffic_cycle_day", Min: &minDay, Max: &maxDay, OnlyInt: true},
			&core.SelectField{Id: trafficCountModeFieldID, Name: "traffic_count_mode", Values: []string{"egress", "combined"}, MaxSelect: 1},
			&core.JSONField{Id: trafficUsageFieldID, Name: "traffic_usage"},
		)
		if err := app.Save(systems); err != nil {
			return err
		}
		if _, err := app.DB().NewQuery("UPDATE systems SET traffic_cycle_day = 1 WHERE traffic_cycle_day = 0").Execute(); err != nil {
			return err
		}
		if _, err := app.DB().NewQuery("UPDATE systems SET traffic_count_mode = 'combined' WHERE traffic_count_mode = ''").Execute(); err != nil {
			return err
		}
		alerts, err := app.FindCollectionByNameOrId("alerts")
		if err != nil {
			return err
		}
		alertName, ok := alerts.Fields.GetByName("name").(*core.SelectField)
		if !ok {
			return errors.New("alerts name field is not a select field")
		}
		if !slices.Contains(alertName.Values, "TrafficQuota") {
			alertName.Values = append(alertName.Values, "TrafficQuota")
			if err := app.Save(alerts); err != nil {
				return err
			}
		}

		checkpoints := core.NewBaseCollection("traffic_checkpoints", trafficCheckpointsID)
		checkpoints.Fields.Add(
			&core.RelationField{Id: trafficCheckpointRelID, Name: "system", CollectionId: systems.Id, CascadeDelete: true, MinSelect: 1, MaxSelect: 1},
			&core.JSONField{Id: trafficCheckpointDataID, Name: "counters", Required: true},
			&core.DateField{Id: trafficCheckpointAtID, Name: "sampled_at", Required: true},
			&core.DateField{Id: trafficInitializedAtID, Name: "initialized_at", Required: true},
			&core.TextField{Id: trafficScheduleKeyID, Name: "schedule_key", Required: true},
		)
		checkpoints.Indexes = append(checkpoints.Indexes, "CREATE UNIQUE INDEX `idx_traffic_checkpoints_system` ON `traffic_checkpoints` (`system`)")
		return app.Save(checkpoints)
	}, func(app core.App) error {
		if checkpoints, err := app.FindCollectionByNameOrId(trafficCheckpointsID); err == nil {
			if err := app.Delete(checkpoints); err != nil {
				return err
			}
		}
		if _, err := app.DB().NewQuery("DELETE FROM alerts_history WHERE name = 'TrafficQuota'").Execute(); err != nil {
			return err
		}
		if _, err := app.DB().NewQuery("DELETE FROM alerts WHERE name = 'TrafficQuota'").Execute(); err != nil {
			return err
		}
		alerts, err := app.FindCollectionByNameOrId("alerts")
		if err != nil {
			return err
		}
		alertName, ok := alerts.Fields.GetByName("name").(*core.SelectField)
		if !ok {
			return errors.New("alerts name field is not a select field")
		}
		alertName.Values = slices.DeleteFunc(alertName.Values, func(value string) bool { return value == "TrafficQuota" })
		if err := app.Save(alerts); err != nil {
			return err
		}
		systems, err := app.FindCollectionByNameOrId("systems")
		if err != nil {
			return err
		}
		for _, id := range []string{trafficQuotaFieldID, trafficCycleDayFieldID, trafficCountModeFieldID, trafficUsageFieldID} {
			systems.Fields.RemoveById(id)
		}
		return app.Save(systems)
	})
}
