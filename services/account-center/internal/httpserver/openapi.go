package httpserver

import (
	"regexp"
	"strings"
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
	if len(access.RequiredRoles) > 0 && !access.Authenticated {
		panic("HTTP role requirements require authenticated access")
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
	if len(access.RequiredRoles) > 0 {
		result["requiredRoles"] = append([]string(nil), access.RequiredRoles...)
	}
	return result
}

func humaPath(path string) string {
	return ginPathParameterPattern.ReplaceAllString(path, `{$1}`)
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
