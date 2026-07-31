package notifications

import (
	"embed"
	"io/fs"
)

//go:embed data/sql/migrations
var migrationFiles embed.FS

// GetMigrationsFS returns the dialect-rooted embedded migration tree.
func GetMigrationsFS() (fs.FS, error) {
	return fs.Sub(migrationFiles, "data/sql/migrations")
}
