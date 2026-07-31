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
// for composition in a host application's shared migration graph.
func OrderedMigrationSource() (persistence.OrderedMigrationSource, error) {
	root, err := GetMigrationsFS()
	if err != nil {
		return persistence.OrderedMigrationSource{}, err
	}
	return persistence.NewStableOrderedMigrationSource(
		MigrationSourceName,
		root,
		MigrationSourceKey,
		MigrationSourceOrder,
		persistence.WithOrderedMigrationDialectOptions(
			persistence.WithDialectSourceLabel(MigrationSourceName),
			persistence.WithValidationTargets("postgres", "sqlite"),
			persistence.WithDialectValidationContract(persistence.DialectValidationContract{
				MandatoryTargets: []string{"postgres", "sqlite"},
			}),
			persistence.WithValidateOnMigrate(true),
		),
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
