package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	initmigrate "platform-mihomo-service/initialize/migrate"
)

const connectTimeout = 10 * time.Second

type Config struct {
	DSN string
}

func Connect(cfg Config) (*gorm.DB, error) {
	if strings.TrimSpace(cfg.DSN) == "" {
		return nil, fmt.Errorf("database DSN is required")
	}
	if _, err := pgx.ParseConfig(cfg.DSN); err != nil {
		return nil, fmt.Errorf("parse PostgreSQL DSN: %w", err)
	}

	migrationDB, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL migration database: %w", err)
	}
	if err := initmigrate.Run(migrationDB); err != nil {
		_ = migrationDB.Close()
		return nil, fmt.Errorf("migrate PostgreSQL database: %w", err)
	}
	_ = migrationDB.Close()

	db, err := gorm.Open(gormpostgres.Open(cfg.DSN), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL database: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("obtain PostgreSQL database handle: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping PostgreSQL database: %w", err)
	}
	return db, nil
}
