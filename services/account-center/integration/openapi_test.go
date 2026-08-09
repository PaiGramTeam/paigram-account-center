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

	type operation struct {
		RequestBody *struct {
			Required bool `json:"required"`
			Content  map[string]struct {
				Schema json.RawMessage `json:"schema"`
			} `json:"content"`
		} `json:"requestBody"`
		Responses map[string]json.RawMessage `json:"responses"`
		Access    struct {
			Public             bool     `json:"public"`
			Authenticated      bool     `json:"authenticated"`
			DynamicPermissions []string `json:"dynamicPermissions"`
		} `json:"x-access"`
	}
	var createOperation operation
	require.NoError(t, json.Unmarshal(document.Paths["/api/v1/auth/register"]["post"], &createOperation))
	require.Contains(t, createOperation.Responses, "201")
	require.Contains(t, createOperation.Responses, "400")
	require.Contains(t, createOperation.Responses, "409")
	require.NotContains(t, createOperation.Responses, "200")
	require.NotContains(t, createOperation.Responses, "202")
	require.NotNil(t, createOperation.RequestBody)
	require.True(t, createOperation.RequestBody.Required)
	require.Contains(t, createOperation.RequestBody.Content, "application/json")
	require.NotEmpty(t, createOperation.RequestBody.Content["application/json"].Schema)

	var loginOperation operation
	require.NoError(t, json.Unmarshal(document.Paths["/api/v1/auth/login"]["post"], &loginOperation))
	require.Contains(t, loginOperation.Responses, "200")
	require.Contains(t, loginOperation.Responses, "401")
	require.NotContains(t, loginOperation.Responses, "201")

	var tokenOperation operation
	require.NoError(t, json.Unmarshal(document.Paths["/api/v1/oauth/token"]["post"], &tokenOperation))
	require.NotNil(t, tokenOperation.RequestBody)
	require.True(t, tokenOperation.RequestBody.Required)
	require.Contains(t, tokenOperation.RequestBody.Content, "application/x-www-form-urlencoded")
	require.ElementsMatch(t, []string{"200", "400", "401", "408", "413", "415", "500"}, mapKeys(tokenOperation.Responses))

	var actionOperation operation
	require.NoError(t, json.Unmarshal(document.Paths["/api/v1/admin/system/platform-services/{id}/check"]["post"], &actionOperation))
	require.Nil(t, actionOperation.RequestBody)
	require.Contains(t, actionOperation.Responses, "200")
	require.Contains(t, actionOperation.Responses, "503")

	var adminOperation operation
	require.NoError(t, json.Unmarshal(document.Paths["/api/v1/admin/users"]["get"], &adminOperation))
	require.True(t, adminOperation.Access.Authenticated)
	require.False(t, adminOperation.Access.Public)
	require.Equal(t, []string{"casbin:path-and-method"}, adminOperation.Access.DynamicPermissions)

	var protectedOperation struct {
		Security []map[string][]string `json:"security"`
	}
	require.NoError(t, json.Unmarshal(document.Paths["/api/v1/me"]["get"], &protectedOperation))
	require.NotEmpty(t, protectedOperation.Security)

	var parameterizedOperation struct {
		Parameters []struct {
			Name     string `json:"name"`
			In       string `json:"in"`
			Required bool   `json:"required"`
		} `json:"parameters"`
	}
	require.NoError(t, json.Unmarshal(document.Paths["/api/v1/admin/users/{id}"]["get"], &parameterizedOperation))
	require.NotEmpty(t, parameterizedOperation.Parameters)
	require.Equal(t, "id", parameterizedOperation.Parameters[0].Name)
	require.Equal(t, "path", parameterizedOperation.Parameters[0].In)
	require.True(t, parameterizedOperation.Parameters[0].Required)
}

func mapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
