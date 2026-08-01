package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

const systemSubscriptionExpiryFieldID = "subscription_exp"

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("systems")
		if err != nil {
			return err
		}
		collection.Fields.Add(&core.DateField{
			Id:   systemSubscriptionExpiryFieldID,
			Name: "subscription_expires",
		})
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("systems")
		if err != nil {
			return err
		}
		collection.Fields.RemoveById(systemSubscriptionExpiryFieldID)
		return app.Save(collection)
	})
}
