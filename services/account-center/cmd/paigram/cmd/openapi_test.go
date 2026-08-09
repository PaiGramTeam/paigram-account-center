package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteOpenAPIDocument(t *testing.T) {
	output := filepath.Join(t.TempDir(), "nested", "openapi.json")
	require.NoError(t, writeOpenAPIDocument(output, []byte("first\n")))
	require.NoError(t, writeOpenAPIDocument(output, []byte("second\n")))

	contents, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Equal(t, "second\n", string(contents))
}
