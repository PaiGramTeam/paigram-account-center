package httpserver

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

var (
	ginPathParameterPattern  = regexp.MustCompile(`:([A-Za-z_][A-Za-z0-9_-]*)`)
	humaPathParameterPattern = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_-]*)\}`)
)

func validateAccess(access Access) {
	classifications := 0
	if access.Public {
		classifications++
	}
	if access.Authenticated {
		classifications++
	}
	if strings.TrimSpace(access.Permission) != "" {
		classifications++
	}
	if classifications != 1 {
		panic("exactly one HTTP access classification is required")
	}
	if len(access.DynamicPermissions) > 0 && !access.Authenticated {
		panic("dynamic HTTP permissions require authenticated access")
	}
}

func accessExtension(access Access) map[string]any {
	result := map[string]any{
		"public":        access.Public,
		"authenticated": access.Authenticated,
	}
	if access.Permission != "" {
		result["permission"] = access.Permission
	}
	if len(access.DynamicPermissions) > 0 {
		result["dynamicPermissions"] = append([]string(nil), access.DynamicPermissions...)
	}
	return result
}

func humaPath(path string) string {
	return ginPathParameterPattern.ReplaceAllString(path, `{$1}`)
}

func pathParameters(path string) []*huma.Param {
	matches := humaPathParameterPattern.FindAllStringSubmatch(path, -1)
	parameters := make([]*huma.Param, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		name := match[1]
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		parameters = append(parameters, &huma.Param{
			Name:     name,
			In:       "path",
			Required: true,
			Schema:   &huma.Schema{Type: "string"},
		})
	}
	return parameters
}

func compatibilitySuccessResponses(method, path string) map[string]*huma.Response {
	jsonResponse := func(description string) *huma.Response {
		return &huma.Response{
			Description: description,
			Content: map[string]*huma.MediaType{
				"application/json": {Schema: &huma.Schema{Type: "object", AdditionalProperties: true}},
			},
		}
	}
	status := compatibilitySuccessStatus(method, path)
	if status == http.StatusNoContent {
		return map[string]*huma.Response{"204": {Description: "Request completed without a response body"}}
	}
	return map[string]*huma.Response{strconv.Itoa(status): jsonResponse(http.StatusText(status))}
}

func compatibilitySuccessStatus(method, path string) int {
	if status, exists := compatibilityStatusOverrides[routeKey(method, path)]; exists {
		return status
	}
	if method == http.MethodDelete {
		return http.StatusNoContent
	}
	return http.StatusOK
}

var compatibilityStatusOverrides = map[string]int{
	"GET /api/v1/auth/telegram/start":                      http.StatusFound,
	"GET /api/v1/auth/telegram/callback":                   http.StatusFound,
	"POST /api/v1/auth/register":                           http.StatusCreated,
	"POST /api/v1/me/emails":                               http.StatusCreated,
	"POST /api/v1/me/platform-accounts":                    http.StatusCreated,
	"POST /api/v1/admin/users":                             http.StatusCreated,
	"POST /api/v1/admin/system/platform-services":          http.StatusCreated,
	"POST /api/v1/admin/service-credentials":               http.StatusCreated,
	"DELETE /api/v1/me/emails/{emailId}":                   http.StatusOK,
	"DELETE /api/v1/me/login-methods/{provider}":           http.StatusOK,
	"DELETE /api/v1/me/security/2fa":                       http.StatusOK,
	"DELETE /api/v1/admin/users/{id}/sessions/{sessionId}": http.StatusOK,
	"DELETE /api/v1/admin/roles/{id}":                      http.StatusOK,
}

func joinPath(prefix, path string) string {
	if path == "" {
		return prefix
	}
	if prefix == "" || prefix == "/" {
		return "/" + strings.TrimPrefix(path, "/")
	}
	return strings.TrimRight(prefix, "/") + "/" + strings.TrimLeft(path, "/")
}

func operationID(method, path string) string {
	value := strings.Trim(path, "/")
	value = strings.NewReplacer("/", "-", "{", "", "}", "", "_", "-").Replace(value)
	if value == "" {
		value = "root"
	}
	return strings.ToLower(method) + "-" + value
}

func operationTag(path string) string {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	if len(segments) > 2 {
		return segments[2]
	}
	return "system"
}
