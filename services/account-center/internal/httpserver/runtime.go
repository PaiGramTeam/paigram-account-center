package httpserver

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/gin-gonic/gin"
)

var pathParameterPattern = regexp.MustCompile(`:([A-Za-z_][A-Za-z0-9_-]*)`)

type OpenAPIOptions struct {
	Enabled bool
	Path    string
}

type Options struct {
	Title   string
	Version string
	OpenAPI OpenAPIOptions
}

// Runtime couples the Gin engine with the Huma API and its route catalog.
type Runtime struct {
	Engine  *gin.Engine
	API     huma.API
	V1      *Group
	Catalog *Catalog
}

// Attach installs Huma on an already-configured Gin engine.
func Attach(engine *gin.Engine, options Options) (*Runtime, error) {
	if engine == nil {
		return nil, errors.New("gin engine is required")
	}
	if strings.TrimSpace(options.Title) == "" || strings.TrimSpace(options.Version) == "" {
		return nil, errors.New("OpenAPI title and version are required")
	}

	config := huma.DefaultConfig(options.Title, options.Version)
	config.CreateHooks = nil
	config.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearerAuth": {
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "JWT",
			Description:  "Paigram access token.",
		},
	}
	if options.OpenAPI.Enabled {
		path := strings.TrimSpace(options.OpenAPI.Path)
		if path == "" {
			path = "/openapi"
		}
		config.OpenAPIPath = path
	} else {
		config.OpenAPIPath = ""
		config.DocsPath = ""
		config.SchemasPath = ""
	}

	api := humagin.New(engine, config)
	catalog := newCatalog()
	v1API := huma.NewGroup(api, "/api/v1")
	v1 := &Group{
		router:  engine.Group("/api/v1"),
		api:     v1API,
		catalog: catalog,
		prefix:  "/api/v1",
		access:  Access{Public: true},
	}
	return &Runtime{Engine: engine, API: api, V1: v1, Catalog: catalog}, nil
}

// Group keeps Gin middleware composition and Huma documentation in lockstep.
type Group struct {
	router  *gin.RouterGroup
	api     huma.API
	catalog *Catalog
	prefix  string
	access  Access
}

// RouteGroup is the shared surface implemented by Gin and the Huma-aware group.
type RouteGroup[T any] interface {
	Group(string, ...gin.HandlerFunc) T
	Use(...gin.HandlerFunc) gin.IRoutes
	GET(string, ...gin.HandlerFunc) gin.IRoutes
	POST(string, ...gin.HandlerFunc) gin.IRoutes
	PUT(string, ...gin.HandlerFunc) gin.IRoutes
	PATCH(string, ...gin.HandlerFunc) gin.IRoutes
	DELETE(string, ...gin.HandlerFunc) gin.IRoutes
}

func (g *Group) Group(path string, handlers ...gin.HandlerFunc) *Group {
	return &Group{
		router:  g.router.Group(path, handlers...),
		api:     huma.NewGroup(g.api, humaPath(path)),
		catalog: g.catalog,
		prefix:  joinPath(g.prefix, humaPath(path)),
		access:  g.access,
	}
}

func (g *Group) WithAccess(access Access) *Group {
	validateAccess(access)
	clone := *g
	clone.access = access
	return &clone
}

func (g *Group) Use(middleware ...gin.HandlerFunc) gin.IRoutes {
	return g.router.Use(middleware...)
}

func (g *Group) GET(path string, handlers ...gin.HandlerFunc) gin.IRoutes {
	return g.register(http.MethodGet, path, handlers...)
}

func (g *Group) POST(path string, handlers ...gin.HandlerFunc) gin.IRoutes {
	return g.register(http.MethodPost, path, handlers...)
}

func (g *Group) PUT(path string, handlers ...gin.HandlerFunc) gin.IRoutes {
	return g.register(http.MethodPut, path, handlers...)
}

func (g *Group) PATCH(path string, handlers ...gin.HandlerFunc) gin.IRoutes {
	return g.register(http.MethodPatch, path, handlers...)
}

func (g *Group) DELETE(path string, handlers ...gin.HandlerFunc) gin.IRoutes {
	return g.register(http.MethodDelete, path, handlers...)
}

func (g *Group) register(method, path string, handlers ...gin.HandlerFunc) gin.IRoutes {
	if len(handlers) == 0 {
		panic("HTTP operation handler is required")
	}
	validateAccess(g.access)
	relativePath := humaPath(path)
	fullPath := joinPath(g.prefix, relativePath)
	op := huma.Operation{
		OperationID: operationID(method, fullPath),
		Method:      method,
		Path:        relativePath,
		Tags:        []string{operationTag(fullPath)},
		Responses: map[string]*huma.Response{
			"200": {Description: "Successful response"},
		},
	}
	if !g.access.Public {
		op.Security = []map[string][]string{{"bearerAuth": {}}}
	}
	if documenter, ok := g.api.(huma.OperationDocumenter); ok {
		documenter.DocumentOperation(&op)
	} else {
		g.api.OpenAPI().AddOperation(&op)
	}
	routes := g.router.Handle(method, path, handlers...)
	g.catalog.add(Route{OperationID: op.OperationID, Method: method, Path: fullPath, Access: g.access})
	return routes
}

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
}

func humaPath(path string) string {
	return pathParameterPattern.ReplaceAllString(path, `{$1}`)
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
