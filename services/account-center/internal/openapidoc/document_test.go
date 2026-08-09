package openapidoc

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateCoversAccountCenterRoutes(t *testing.T) {
	document, err := Generate()
	require.NoError(t, err)
	require.True(t, bytes.HasSuffix(document, []byte("\n")))

	var specification struct {
		OpenAPI string                     `json:"openapi"`
		Paths   map[string]json.RawMessage `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(document, &specification))
	require.NotEmpty(t, specification.OpenAPI)
	require.Contains(t, specification.Paths, "/api/v1/auth/login")
	require.Contains(t, specification.Paths, "/api/v1/me")
	require.Contains(t, specification.Paths, "/api/v1/admin/users/{id}")
}

func TestNormalizeRejectsIncompleteDocuments(t *testing.T) {
	_, err := normalize([]byte(`{"openapi":"3.1.0","paths":{}}`))
	require.EqualError(t, err, "OpenAPI document has no version or paths")
}
