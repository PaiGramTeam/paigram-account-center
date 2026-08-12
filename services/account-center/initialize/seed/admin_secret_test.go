package seed

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveAdminConfigPrefersExternalPasswordFile(t *testing.T) {
	t.Setenv("ADMIN_EMAIL", "operator@example.test")
	t.Setenv("ADMIN_PASSWORD", "environment-password")
	passwordFile := filepath.Join(t.TempDir(), "admin-password")
	require.NoError(t, os.WriteFile(passwordFile, []byte("file-password\n"), 0o600))
	t.Setenv("ADMIN_PASSWORD_FILE", passwordFile)

	resolved, err := resolveAdminConfig()
	require.NoError(t, err)
	require.Equal(t, "operator@example.test", resolved.Email)
	require.Equal(t, "file-password", resolved.Password)
}
