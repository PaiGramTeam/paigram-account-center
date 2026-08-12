//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/postgresschema"
	"github.com/stretchr/testify/require"
)

func TestMigratedSchemaMatchesSnapshot(t *testing.T) {
	stack := newIntegrationStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	entries, err := postgresschema.Capture(ctx, stack.SQLDB)
	require.NoError(t, err)
	actual, err := json.MarshalIndent(entries, "", "  ")
	require.NoError(t, err)
	actual = append(actual, '\n')

	snapshotPath := filepath.Join("testdata", "schema_snapshot.json")
	if os.Getenv("PAI_UPDATE_SCHEMA_SNAPSHOT") == "true" {
		require.NoError(t, os.MkdirAll(filepath.Dir(snapshotPath), 0o755))
		require.NoError(t, os.WriteFile(snapshotPath, actual, 0o600))
		return
	}
	expected, err := os.ReadFile(snapshotPath)
	require.NoError(t, err, "schema snapshot is missing; regenerate it intentionally")
	require.Equal(t, string(expected), string(actual), "migrated schema differs from its tracked snapshot")
}
