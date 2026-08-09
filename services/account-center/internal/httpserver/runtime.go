package httpserver

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/gin-gonic/gin"
)

var (
	ginPathParameterPattern  = regexp.MustCompile(`:([A-Za-z_][A-Za-z0-9_-]*)`)
	humaPathParameterPattern = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_-]*)\}`)
)

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
	v1 := &Group{
		router:  engine.Group("/api/v1"),
		engine:  engine,
		api:     api,
		catalog: catalog,
		prefix:  "/api/v1",
		access:  Access{Public: true},
	}
	return &Runtime{Engine: engine, API: api, V1: v1, Catalog: catalog}, nil
}

// Group keeps Gin middleware composition and Huma operation registration in lockstep.
type Group struct {
	router  *gin.RouterGroup
	engine  *gin.Engine
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
		engine:  g.engine,
		api:     g.api,
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
	fullPath := joinPath(g.prefix, humaPath(path))
	op := huma.Operation{
		OperationID:        operationID(method, fullPath),
		Method:             method,
		Path:               fullPath,
		DefaultStatus:      http.StatusOK,
		SkipValidateParams: true,
		SkipValidateBody:   true,
		Tags:               []string{operationTag(fullPath)},
		Parameters:         pathParameters(fullPath),
		Responses:          compatibilitySuccessResponses(method),
	}
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
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		registrationAPI.optionalRequestBody = true
		registerGinBridge(registrationAPI, op, endpoint, func(input *ginBridgeBodyInput) []byte { return input.RawBody })
	default:
		registerGinBridge(registrationAPI, op, endpoint, func(_ *ginBridgeInput) []byte { return nil })
	}
	g.catalog.add(Route{OperationID: op.OperationID, Method: method, Path: fullPath, Access: g.access})
	return g.router
}

type ginBridgeInput struct{}

type ginBridgeBodyInput struct {
	RawBody []byte `contentType:"*/*"`
}

type ginBridgeOutput struct {
	Status int `status:""`
	Body   []byte
}

type ginContextKey struct{}

type humaContextKey struct{}

func registerGinBridge[I any](api huma.API, op huma.Operation, endpoint gin.HandlerFunc, requestBody func(*I) []byte) {
	huma.Register[I, ginBridgeOutput](api, op, func(ctx context.Context, input *I) (*ginBridgeOutput, error) {
		ginContext, ok := ctx.Value(ginContextKey{}).(*gin.Context)
		if !ok {
			return nil, errors.New("gin context is unavailable")
		}
		humaContext, ok := ctx.Value(humaContextKey{}).(huma.Context)
		if !ok {
			return nil, errors.New("huma context is unavailable")
		}
		body := requestBody(input)
		if body != nil {
			ginContext.Request.Body = io.NopCloser(bytes.NewReader(body))
			ginContext.Request.ContentLength = int64(len(body))
		}

		originalWriter := ginContext.Writer
		captured := newBufferedGinWriter(originalWriter)
		ginContext.Writer = captured
		func() {
			defer func() { ginContext.Writer = originalWriter }()
			endpoint(ginContext)
		}()
		copyHeaders(humaContext, captured.Header())
		return &ginBridgeOutput{Status: captured.Status(), Body: append([]byte(nil), captured.body.Bytes()...)}, nil
	})
}

type registrationAPI struct {
	huma.API
	adapter             huma.Adapter
	optionalRequestBody bool
}

func (a *registrationAPI) Adapter() huma.Adapter {
	return a.adapter
}

func (a *registrationAPI) DocumentOperation(op *huma.Operation) {
	if a.optionalRequestBody && op.RequestBody != nil {
		op.RequestBody.Required = false
	}
	if !op.Hidden {
		a.OpenAPI().AddOperation(op)
	}
}

type ginRegistrationAdapter struct {
	engine     *gin.Engine
	router     *gin.RouterGroup
	path       string
	middleware []gin.HandlerFunc
}

func (a *ginRegistrationAdapter) Handle(op *huma.Operation, handler func(huma.Context)) {
	bridge := func(c *gin.Context) {
		originalRequest := c.Request
		humaContext := humagin.NewContext(op, c)
		requestContext := context.WithValue(c.Request.Context(), ginContextKey{}, c)
		requestContext = context.WithValue(requestContext, humaContextKey{}, humaContext)
		c.Request = c.Request.WithContext(requestContext)
		defer func() { c.Request = originalRequest }()
		handler(humaContext)
	}
	handlers := append(append([]gin.HandlerFunc(nil), a.middleware...), bridge)
	a.router.Handle(op.Method, a.path, handlers...)
}

type bufferedGinWriter struct {
	original gin.ResponseWriter
	header   http.Header
	body     bytes.Buffer
	status   int
	size     int
	written  bool
}

func newBufferedGinWriter(original gin.ResponseWriter) *bufferedGinWriter {
	header := make(http.Header, len(original.Header()))
	for name, values := range original.Header() {
		header[name] = append([]string(nil), values...)
	}
	return &bufferedGinWriter{original: original, header: header, status: http.StatusOK, size: -1}
}

func (w *bufferedGinWriter) Header() http.Header {
	return w.header
}

func (w *bufferedGinWriter) Write(data []byte) (int, error) {
	w.WriteHeaderNow()
	written, err := w.body.Write(data)
	w.size += written
	return written, err
}

func (w *bufferedGinWriter) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

func (w *bufferedGinWriter) WriteHeader(status int) {
	if w.written {
		return
	}
	w.status = status
}

func (w *bufferedGinWriter) WriteHeaderNow() {
	if w.written {
		return
	}
	w.written = true
	w.size = 0
}

func (w *bufferedGinWriter) Status() int {
	return w.status
}

func (w *bufferedGinWriter) Size() int {
	return w.size
}

func (w *bufferedGinWriter) Written() bool {
	return w.written
}

func (w *bufferedGinWriter) Flush() {
	w.WriteHeaderNow()
}

func (w *bufferedGinWriter) CloseNotify() <-chan bool {
	return w.original.CloseNotify()
}

func (w *bufferedGinWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.original.Hijack()
}

func (w *bufferedGinWriter) Pusher() http.Pusher {
	return w.original.Pusher()
}

func copyHeaders(ctx huma.Context, headers http.Header) {
	for name, values := range headers {
		for index, value := range values {
			if index == 0 {
				ctx.SetHeader(name, value)
			} else {
				ctx.AppendHeader(name, value)
			}
		}
	}
}

func (a *ginRegistrationAdapter) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	a.engine.ServeHTTP(writer, request)
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

func compatibilitySuccessResponses(method string) map[string]*huma.Response {
	jsonResponse := func(description string) *huma.Response {
		return &huma.Response{
			Description: description,
			Content: map[string]*huma.MediaType{
				"application/json": {Schema: &huma.Schema{Type: "object", AdditionalProperties: true}},
			},
		}
	}
	responses := map[string]*huma.Response{"200": jsonResponse("Successful response")}
	switch method {
	case http.MethodPost:
		responses["201"] = jsonResponse("Resource created")
		responses["202"] = jsonResponse("Request accepted")
	case http.MethodDelete:
		responses["204"] = &huma.Response{Description: "Request completed without a response body"}
	}
	return responses
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
