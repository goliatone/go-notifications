package notifications

import (
	persistence "github.com/goliatone/go-persistence-bun"
)

const (
	MigrationSourceName  = "go-notifications"
	MigrationSourceKey   = "go-notifications"
	MigrationSourceOrder = 50
)

// OrderedMigrationSource exposes a source-stable migration identity suitable
// for composition in a host application's shared migration graph. Hosts may
// supply source keys that must run before this package.
func OrderedMigrationSource(dependencies ...string) (persistence.OrderedMigrationSource, error) {
	root, err := GetMigrationsFS()
	if err != nil {
		return persistence.OrderedMigrationSource{}, err
	}
	options := []persistence.OrderedMigrationSourceOption{
		persistence.WithOrderedMigrationDialectOptions(
			persistence.WithDialectSourceLabel(MigrationSourceName),
			persistence.WithValidationTargets("postgres", "sqlite"),
			persistence.WithDialectValidationContract(persistence.DialectValidationContract{
				MandatoryTargets: []string{"postgres", "sqlite"},
			}),
			persistence.WithValidateOnMigrate(true),
		),
	}
	if len(dependencies) > 0 {
		options = append(options, persistence.WithOrderedMigrationDependencies(dependencies...))
	}
	return persistence.NewStableOrderedMigrationSource(
		MigrationSourceName,
		root,
		MigrationSourceKey,
		MigrationSourceOrder,
		options...,
	), nil
}

// RegisterMigrations registers this package's ordered source without running
// migrations or taking ownership of host database lifecycle.
func RegisterMigrations(manager *persistence.Migrations) error {
	if manager == nil {
		return nil
	}
	source, err := OrderedMigrationSource()
	if err != nil {
		return err
	}
	return manager.RegisterOrderedMigrationSources(source)
}
