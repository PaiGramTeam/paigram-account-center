//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"paigram/internal/model"
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
		Security  []map[string][]string      `json:"security"`
		Access    struct {
			Public             bool     `json:"public"`
			Authenticated      bool     `json:"authenticated"`
			DynamicPermissions []string `json:"dynamicPermissions"`
			RequiredRoles      []string `json:"requiredRoles"`
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
	require.Contains(t, loginOperation.Responses, "429")
	require.NotContains(t, loginOperation.Responses, "201")
	var loginSuccess struct {
		Content map[string]struct {
			Schema struct {
				OneOf []json.RawMessage `json:"oneOf"`
			} `json:"schema"`
		} `json:"content"`
	}
	require.NoError(t, json.Unmarshal(loginOperation.Responses["200"], &loginSuccess))
	require.Len(t, loginSuccess.Content["application/json"].Schema.OneOf, 2)

	var refreshOperation operation
	require.NoError(t, json.Unmarshal(document.Paths["/api/v1/auth/refresh"]["post"], &refreshOperation))
	require.Contains(t, refreshOperation.Responses, "429")
	require.Nil(t, refreshOperation.RequestBody)
	require.Equal(t, []map[string][]string{{"refreshCookie": {}}}, refreshOperation.Security)

	var logoutOperation operation
	require.NoError(t, json.Unmarshal(document.Paths["/api/v1/auth/logout"]["post"], &logoutOperation))
	require.NotContains(t, logoutOperation.Responses, "429")
	require.Nil(t, logoutOperation.RequestBody)
	require.Equal(t, []map[string][]string{{"bearerAuth": {}}, {"refreshCookie": {}}}, logoutOperation.Security)

	var tokenOperation operation
	require.NoError(t, json.Unmarshal(document.Paths["/api/v1/oauth/token"]["post"], &tokenOperation))
	require.NotNil(t, tokenOperation.RequestBody)
	require.True(t, tokenOperation.RequestBody.Required)
	require.Contains(t, tokenOperation.RequestBody.Content, "application/x-www-form-urlencoded")
	require.ElementsMatch(t, []string{"200", "400", "401", "500"}, mapKeys(tokenOperation.Responses))

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
	require.Equal(t, []string{"admin"}, adminOperation.Access.RequiredRoles)
	require.Contains(t, adminOperation.Responses, "429")

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
			Schema   struct {
				Type string `json:"type"`
			} `json:"schema"`
		} `json:"parameters"`
	}
	require.NoError(t, json.Unmarshal(document.Paths["/api/v1/admin/users/{id}"]["get"], &parameterizedOperation))
	require.NotEmpty(t, parameterizedOperation.Parameters)
	require.Equal(t, "id", parameterizedOperation.Parameters[0].Name)
	require.Equal(t, "path", parameterizedOperation.Parameters[0].In)
	require.True(t, parameterizedOperation.Parameters[0].Required)
	require.Equal(t, "integer", parameterizedOperation.Parameters[0].Schema.Type)

	var listUsersOperation struct {
		Parameters []struct {
			Name   string `json:"name"`
			In     string `json:"in"`
			Schema struct {
				Type string `json:"type"`
			} `json:"schema"`
		} `json:"parameters"`
	}
	require.NoError(t, json.Unmarshal(document.Paths["/api/v1/admin/users"]["get"], &listUsersOperation))
	parameters := make(map[string]string, len(listUsersOperation.Parameters))
	for _, parameter := range listUsersOperation.Parameters {
		require.Equal(t, "query", parameter.In)
		parameters[parameter.Name] = parameter.Schema.Type
	}
	require.Equal(t, "integer", parameters["page"])
	require.Equal(t, "integer", parameters["page_size"])
	require.Equal(t, "string", parameters["sort_by"])
	require.Equal(t, "string", parameters["order"])
	require.Equal(t, "string", parameters["status"])
	require.Equal(t, "string", parameters["search"])

	require.Contains(t, recorder.Body.String(), "totp_code")
	require.Contains(t, recorder.Body.String(), "trust_device")
}

func TestSpecializedHumaContractsPreserveWireSemantics(t *testing.T) {
	stack := newIntegrationStack(t)
	userID, _, _, email, password := registerAndLogin(
		t, stack, "huma-contract-2fa@example.com", "HumaContractPass123!",
	)
	require.NoError(t, stack.DB.Create(&model.UserTwoFactor{
		UserID:    userID,
		Secret:    "unused-for-challenge",
		EnabledAt: time.Now().UTC(),
	}).Error)

	challenge := performJSONRequest(t, stack.Router, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"email":    email,
		"password": password,
	}, map[string]string{"User-Agent": "HumaContractChallenge/1.0"})
	require.Equal(t, http.StatusOK, challenge.Code, challenge.Body.String())
	challengeData := decodeResponseData(t, challenge)
	require.Equal(t, true, challengeData["requires_totp"])
	require.NotEmpty(t, challengeData["message"])
	require.NotContains(t, challengeData, "access_token")

	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(
		`{"email":"missing@example.com","password":"password123","totp_code":"123456","trust_device":true}`,
	))
	login.Header.Set("Content-Type", "application/json")
	loginResponse := httptest.NewRecorder()
	stack.Router.ServeHTTP(loginResponse, login)
	require.Equal(t, http.StatusUnauthorized, loginResponse.Code)

	token := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/token", strings.NewReader(`{"grant_type":"client_credentials"}`))
	token.Header.Set("Content-Type", "application/json")
	tokenResponse := httptest.NewRecorder()
	stack.Router.ServeHTTP(tokenResponse, token)
	require.Equal(t, http.StatusBadRequest, tokenResponse.Code)
	require.Equal(t, "no-store", tokenResponse.Header().Get("Cache-Control"))
	var oauthError struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	require.NoError(t, json.Unmarshal(tokenResponse.Body.Bytes(), &oauthError))
	require.Equal(t, "invalid_request", oauthError.Error)
	require.NotEmpty(t, oauthError.ErrorDescription)

	largeToken := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/oauth/token",
		strings.NewReader("grant_type=client_credentials&client_id="+strings.Repeat("x", 9<<20)),
	)
	largeToken.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	largeTokenResponse := httptest.NewRecorder()
	stack.Router.ServeHTTP(largeTokenResponse, largeToken)
	require.Equal(t, http.StatusBadRequest, largeTokenResponse.Code)
	require.Equal(t, "no-store", largeTokenResponse.Header().Get("Cache-Control"))
	require.NoError(t, json.Unmarshal(largeTokenResponse.Body.Bytes(), &oauthError))
	require.Equal(t, "invalid_request", oauthError.Error)
	require.Equal(t, "malformed form body", oauthError.ErrorDescription)
}

func mapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
