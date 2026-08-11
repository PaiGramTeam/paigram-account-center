package migrate

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"

	migrate "github.com/golang-migrate/migrate/v4"
	postgresmigrate "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed sql/*.sql
var migrationFiles embed.FS

func Run(sqlDB *sql.DB) (err error) {
	sourceDriver, err := iofs.New(migrationFiles, "sql")
	if err != nil {
		return fmt.Errorf("initialize embedded migration source: %w", err)
	}

	databaseDriver, err := postgresmigrate.WithInstance(sqlDB, &postgresmigrate.Config{})
	if err != nil {
		_ = sourceDriver.Close()
		return fmt.Errorf("initialize PostgreSQL migration driver: %w", err)
	}

	runner, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", databaseDriver)
	if err != nil {
		_ = sourceDriver.Close()
		_ = databaseDriver.Close()
		return fmt.Errorf("initialize migration runner: %w", err)
	}

	defer func() {
		sourceErr, databaseErr := runner.Close()
		if err != nil {
			return
		}
		switch {
		case sourceErr != nil && databaseErr != nil:
			err = fmt.Errorf("close migration source: %v; database: %w", sourceErr, databaseErr)
		case sourceErr != nil:
			err = fmt.Errorf("close migration source: %w", sourceErr)
		case databaseErr != nil:
			err = fmt.Errorf("close migration database: %w", databaseErr)
		}
	}()

	if err = runner.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
