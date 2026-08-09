package testutil

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type PostgreSQLEnv struct {
	DSN string
}

func LoadPostgreSQLTestEnv(t *testing.T) PostgreSQLEnv {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv("PAI_TEST_DATABASE_DSN"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("PAI_DATABASE_DSN"))
	}
	if dsn == "" {
		t.Skip("PostgreSQL test environment is not configured: set PAI_TEST_DATABASE_DSN or PAI_DATABASE_DSN")
	}
	if _, err := pgx.ParseConfig(dsn); err != nil {
		t.Fatalf("parse PostgreSQL test DSN: %v", err)
	}
	return PostgreSQLEnv{DSN: dsn}
}

func OpenPostgreSQLTestDB(t *testing.T, prefix string, models ...any) *gorm.DB {
	t.Helper()

	env := LoadPostgreSQLTestEnv(t)
	rootDB := openPostgreSQLRootDB(t, env)
	dbName := uniqueDBName(prefix)
	createPostgreSQLDatabase(t, rootDB, dbName)
	t.Cleanup(func() {
		dropPostgreSQLDatabase(t, rootDB, dbName)
		_ = rootDB.Close()
	})

	dsn := postgresDSNForDatabase(t, env.DSN, dbName)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	if len(models) > 0 {
		require.NoError(t, db.AutoMigrate(models...))
	}

	return db
}

func openPostgreSQLRootDB(t *testing.T, env PostgreSQLEnv) *sql.DB {
	t.Helper()
	dsn := postgresDSNForDatabase(t, env.DSN, "postgres")
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	require.NoError(t, db.PingContext(ctx))

	return db
}

func createPostgreSQLDatabase(t *testing.T, db *sql.DB, dbName string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err := db.ExecContext(ctx, "CREATE DATABASE "+quotePostgreSQLIdentifier(dbName))
	require.NoError(t, err)
}

func dropPostgreSQLDatabase(t *testing.T, db *sql.DB, dbName string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err := db.ExecContext(ctx, "DROP DATABASE IF EXISTS "+quotePostgreSQLIdentifier(dbName)+" WITH (FORCE)")
	require.NoError(t, err)
}

func postgresDSNForDatabase(t *testing.T, dsn, database string) string {
	t.Helper()
	cfg, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	cfg.Database = database
	return cfg.ConnString()
}

func quotePostgreSQLIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func uniqueDBName(prefix string) string {
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)
	prefix = sanitize(prefix)
	if len(prefix) > 40 {
		prefix = prefix[:40]
	}
	return fmt.Sprintf("t_%s_%s", prefix, hex.EncodeToString(buf))
}

func sanitize(value string) string {
	value = strings.ToLower(value)
	replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_", "-", "_", ":", "_", ".", "_")
	value = replacer.Replace(value)
	value = strings.Trim(value, "_")
	if value == "" {
		return "postgres"
	}
	return value
}
