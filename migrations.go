package notifications

import (
	"fmt"
	"slices"

	persistence "github.com/goliatone/go-persistence-bun"
)

const (
	MigrationSourceName  = "go-notifications"
	MigrationSourceKey   = "go-notifications"
	MigrationSourceOrder = 50
)

// MigrationSourceOptions configures host-composable placement while
// preserving the package's stable source identity and embedded filesystem.
// Once a host persists this graph, Order and Dependencies are durable and
// changing them is migration graph drift.
type MigrationSourceOptions struct {
	Order        int
	Dependencies []string
}

// OrderedMigrationSource exposes a source-stable migration identity suitable
// for composition in a host application's shared migration graph. Hosts may
// supply source keys that must run before this package.
func OrderedMigrationSource(dependencies ...string) (persistence.OrderedMigrationSource, error) {
	return OrderedMigrationSourceWithOptions(MigrationSourceOptions{
		Order: MigrationSourceOrder, Dependencies: dependencies,
	})
}

// OrderedMigrationSourceWithOptions returns the stable package migration
// source at a positive host-selected order. A zero order retains the
// backward-compatible default order 50.
func OrderedMigrationSourceWithOptions(config MigrationSourceOptions) (persistence.OrderedMigrationSource, error) {
	order := config.Order
	if order == 0 {
		order = MigrationSourceOrder
	}
	if order < 1 || order > persistence.MaxOrderedMigrationSourceOrder {
		return persistence.OrderedMigrationSource{}, &persistence.OrderedSourceGraphError{
			Kind:       persistence.ErrOrderedSourceInvalidConfig,
			SourceName: MigrationSourceName,
			SourceKey:  MigrationSourceKey,
			Message: fmt.Sprintf(
				"notifications: migration source order must be between 1 and %d",
				persistence.MaxOrderedMigrationSourceOrder,
			),
		}
	}
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
	dependencies := slices.Clone(config.Dependencies)
	if len(dependencies) > 0 {
		options = append(options, persistence.WithOrderedMigrationDependencies(dependencies...))
	}
	return persistence.NewStableOrderedMigrationSource(
		MigrationSourceName,
		root,
		MigrationSourceKey,
		order,
		options...,
	), nil
}

// RegisterMigrationsWithOptions registers a host-placed package source
// without executing migrations.
func RegisterMigrationsWithOptions(manager *persistence.Migrations, config MigrationSourceOptions) error {
	if manager == nil {
		return nil
	}
	source, err := OrderedMigrationSourceWithOptions(config)
	if err != nil {
		return err
	}
	return manager.RegisterOrderedMigrationSources(source)
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
