package migrate

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Up runs all pending migrations from the embedded filesystem.
// ErrNoChange is treated as success.
func Up(db *sql.DB, fs embed.FS, migrationsPath string) error {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("migrate: create driver: %w", err)
	}

	d, err := iofs.New(fs, migrationsPath)
	if err != nil {
		return fmt.Errorf("migrate: iofs create: %w", err)
	}

	m, err := migrate.NewWithInstance(
		"iofs", d,
		"postgres", driver,
	)
	if err != nil {
		return fmt.Errorf("migrate: create instance: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate: up: %w", err)
	}

	return nil
}
