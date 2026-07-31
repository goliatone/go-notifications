package migrations

import (
	notifications "github.com/goliatone/go-notifications"
	persistence "github.com/goliatone/go-persistence-bun"
)

const (
	SourceName  = notifications.MigrationSourceName
	SourceKey   = notifications.MigrationSourceKey
	SourceOrder = notifications.MigrationSourceOrder
)

func OrderedSources() ([]persistence.OrderedMigrationSource, error) {
	source, err := notifications.OrderedMigrationSource()
	if err != nil {
		return nil, err
	}
	return []persistence.OrderedMigrationSource{source}, nil
}

func Register(manager *persistence.Migrations) error {
	return notifications.RegisterMigrations(manager)
}
