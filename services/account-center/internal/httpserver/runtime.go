package httpserver

import (
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/gin-gonic/gin"
)

type OpenAPIOptions struct {
	Enabled bool
	Path    string
}

type Options struct {
	Title   string
	Version string
	OpenAPI OpenAPIOptions
	Body    BodyOptions
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
	bodyOptions, err := validateBodyOptions(options.Body)
	if err != nil {
		return nil, err
	}

	installHumaErrorFactory()
	config := huma.DefaultConfig(options.Title, options.Version)
	config.CreateHooks = nil
	config.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearerAuth": {
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "opaque",
			Description:  "Paigram opaque access token.",
		},
		"refreshCookie": {
			Type:        "apiKey",
			In:          "cookie",
			Name:        "ac_refresh",
			Description: "HttpOnly browser refresh credential managed by the Account Center.",
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
	contracts := newRouteContracts()
	v1 := &Group{
		router:    engine.Group("/api/v1"),
		engine:    engine,
		api:       api,
		catalog:   catalog,
		contracts: contracts,
		prefix:    "/api/v1",
		access:    Access{Public: true},
		body:      bodyOptions,
	}
	return &Runtime{Engine: engine, API: api, V1: v1, Catalog: catalog}, nil
}

// Group keeps Gin middleware composition and Huma operation registration in lockstep.
type Group struct {
	router    *gin.RouterGroup
	engine    *gin.Engine
	api       huma.API
	catalog   *Catalog
	contracts *routeContracts
	prefix    string
	access    Access
	body      BodyOptions
	errors    []int
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
		router:    g.router.Group(path, handlers...),
		engine:    g.engine,
		api:       g.api,
		catalog:   g.catalog,
		contracts: g.contracts,
		prefix:    joinPath(g.prefix, humaPath(path)),
		access:    g.access,
		body:      g.body,
		errors:    append([]int(nil), g.errors...),
	}
}

func (g *Group) WithErrorStatuses(statuses ...int) *Group {
	clone := *g
	clone.errors = append(append([]int(nil), g.errors...), statuses...)
	return &clone
}

func (g *Group) WithAccess(access Access) *Group {
	validateAccess(access)
	clone := *g
	clone.access = access
	return &clone
}

func WithAccess[T RouteGroup[T]](group T, access Access) T {
	if humaGroup, ok := any(group).(*Group); ok {
		return any(humaGroup.WithAccess(access)).(T)
	}
	return group
}

func (g *Group) RegisterContract(method, path string, contract Contract) {
	fullPath := joinPath(g.prefix, humaPath(path))
	contract = contract.WithErrorStatuses(g.errors...)
	g.contracts.add(method, fullPath, contract)
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
	fullPath := joinPath(g.prefix, humaPath(path))
	contract, hasContract := g.contracts.get(method, fullPath)
	if !hasContract {
		panic("HTTP operation contract is required: " + routeKey(method, fullPath))
	}
	contract = contract.prepare(g.api.OpenAPI().Components.Schemas, g.body, fullPath)
	op := huma.Operation{
		OperationID:   operationID(method, fullPath),
		Method:        method,
		Path:          fullPath,
		DefaultStatus: contract.successStatus,
		Tags:          []string{operationTag(fullPath)},
	}
	op.Extensions = map[string]any{"x-access": accessExtension(g.access)}
	contract.apply(&op)
	if !g.access.Public {
		op.Security = []map[string][]string{{"bearerAuth": {}}}
	}
	endpoint := handlers[len(handlers)-1]
	adapter := &ginRegistrationAdapter{
		engine:     g.engine,
		router:     g.router,
		path:       path,
		middleware: append([]gin.HandlerFunc(nil), handlers[:len(handlers)-1]...),
	}
	registrationAPI := &registrationAPI{API: g.api, adapter: adapter}
	registrationAPI.contract = &contract
	if contract.readsRequestBody() {
		registerGinBridge(registrationAPI, op, endpoint, &contract, func(input *ginBridgeBodyInput) []byte { return input.RawBody })
	} else {
		registerGinBridge(registrationAPI, op, endpoint, &contract, func(_ *ginBridgeInput) []byte { return nil })
	}
	g.catalog.add(Route{OperationID: op.OperationID, Method: method, Path: fullPath, Access: g.access})
	return g.router
}
