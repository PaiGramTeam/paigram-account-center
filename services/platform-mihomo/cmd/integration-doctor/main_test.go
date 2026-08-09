package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFindRepoRootFindsAncestorWithGoModAndEnvFile(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module platform-mihomo-service\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, ".env.integration.local"), []byte("PAI_TEST_DATABASE_ADDR=127.0.0.1:3306\n"), 0o600))

	start := filepath.Join(repoRoot, "cmd", "integration-doctor")
	require.NoError(t, os.MkdirAll(start, 0o755))

	resolved, err := findRepoRoot(start)
	require.NoError(t, err)
	require.Equal(t, repoRoot, resolved)
}
