//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

var ginPathParameterPattern = regexp.MustCompile(`:([A-Za-z_][A-Za-z0-9_-]*)`)

func TestOpenAPICoversEveryBusinessRoute(t *testing.T) {
	stack := newIntegrationStack(t)
	engine, ok := stack.Router.(*gin.Engine)
	require.True(t, ok)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	stack.Router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)

	var document struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &document))

	for _, route := range engine.Routes() {
		if !strings.HasPrefix(route.Path, "/api/v1") {
			continue
		}
		path := ginPathParameterPattern.ReplaceAllString(route.Path, `{$1}`)
		operations, exists := document.Paths[path]
		require.Truef(t, exists, "OpenAPI path is missing for %s %s", route.Method, route.Path)
		_, exists = operations[strings.ToLower(route.Method)]
		require.Truef(t, exists, "OpenAPI operation is missing for %s %s", route.Method, route.Path)
	}

	require.Contains(t, document.Paths, "/api/v1/auth/register")
	require.Contains(t, document.Paths, "/api/v1/me")
	require.Contains(t, document.Paths, "/api/v1/admin/users/{id}")
}
