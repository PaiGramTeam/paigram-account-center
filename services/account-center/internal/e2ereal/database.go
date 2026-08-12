//go:build integration

package e2ereal

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/platformaction"
)

const platformDatabaseName = "paigram_e2e_platform"

func createPlatformDatabase(ctx context.Context, adminDSN, accountDSN string) (string, error) {
	admin, err := sql.Open("pgx", adminDSN)
	if err != nil {
		return "", fmt.Errorf("open PostgreSQL admin connection: %w", err)
	}
	defer admin.Close()
	if _, err := admin.ExecContext(ctx, `CREATE DATABASE "`+platformDatabaseName+`"`); err != nil {
		return "", fmt.Errorf("create Platform database: %w", err)
	}
	return replaceDatabase(accountDSN, platformDatabaseName)
}

func registerMihomoPlatform(ctx context.Context, accountDSN string) error {
	actions := append(platformaction.MihomoDelegationActions(), platformaction.MihomoControlActions()...)
	actionsJSON, err := json.Marshal(actions)
	if err != nil {
		return fmt.Errorf("encode supported platform actions: %w", err)
	}
	credentialSchema := `{"type":"object","required":["cookie_bundle","device_id","device_fp"],"properties":{"cookie_bundle":{"type":"string","minLength":1},"device_id":{"type":"string","minLength":1},"device_fp":{"type":"string","minLength":1},"device_name":{"type":"string"}}}`
	db, err := sql.Open("pgx", accountDSN)
	if err != nil {
		return fmt.Errorf("open Account database: %w", err)
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, `
INSERT INTO platform_services (
  platform_key, display_name, service_key, service_audience, discovery_type,
  control_endpoint, runtime_endpoint, runtime_server_name, enabled,
  supported_actions_json, credential_schema_json
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, TRUE, $9, $10)
ON CONFLICT (platform_key) DO UPDATE SET
  display_name = EXCLUDED.display_name,
  service_key = EXCLUDED.service_key,
  service_audience = EXCLUDED.service_audience,
  discovery_type = EXCLUDED.discovery_type,
  control_endpoint = EXCLUDED.control_endpoint,
  runtime_endpoint = EXCLUDED.runtime_endpoint,
  runtime_server_name = EXCLUDED.runtime_server_name,
  enabled = TRUE,
  supported_actions_json = EXCLUDED.supported_actions_json,
  credential_schema_json = EXCLUDED.credential_schema_json,
  updated_at = CURRENT_TIMESTAMP`,
		"mihomo", "Mihomo", "platform-mihomo-service", "platform-mihomo-service", "static",
		"127.0.0.1:19000", "127.0.0.1:19001", "platform-runtime.internal", string(actionsJSON), credentialSchema,
	)
	if err != nil {
		return fmt.Errorf("register Mihomo platform: %w", err)
	}
	return nil
}

func replaceDatabase(dsn, database string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("parse PostgreSQL DSN: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" || strings.TrimSpace(database) == "" {
		return "", fmt.Errorf("PostgreSQL DSN and database are required")
	}
	parsed.Path = "/" + database
	return parsed.String(), nil
}
