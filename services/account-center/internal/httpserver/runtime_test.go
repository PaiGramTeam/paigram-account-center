package httpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRuntimeRegistersGinRouteAndHumaOperation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	runtime, err := Attach(engine, Options{
		Title:   "Paigram Account Center API",
		Version: "1.0.0",
		OpenAPI: OpenAPIOptions{Enabled: true, Path: "/openapi"},
	})
	require.NoError(t, err)
	humaCalls := 0
	runtime.API.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		humaCalls++
		next(ctx)
	})

	widgets := runtime.V1.Group("/widgets").WithAccess(Access{Authenticated: true})
	widgets.RegisterContract(http.MethodGet, "/:widgetId", ResponseContract(
		struct {
			ID string `json:"id"`
		}{}, http.StatusOK, http.StatusBadRequest,
	).WithParameters(PathString("widgetId")))
	widgets.RegisterContract(http.MethodPost, "", JSONContract(
		struct {
			Name string `json:"name" minLength:"1"`
		}{},
		struct {
			Name string `json:"name"`
		}{},
		http.StatusCreated,
		http.StatusBadRequest,
	))
	widgets.RegisterContract(http.MethodDelete, "/:widgetId", ResponseContract(
		nil, http.StatusNoContent, http.StatusBadRequest,
	).WithParameters(PathString("widgetId")))
	widgets.RegisterContract(http.MethodPost, "/:widgetId/refresh", ResponseContract(
		nil, http.StatusAccepted, http.StatusBadRequest,
	).WithParameters(PathString("widgetId")))
	widgets.GET("/:widgetId", func(c *gin.Context) {
		c.Set("route_middleware", true)
		c.Next()
	}, func(c *gin.Context) {
		value, exists := c.Get("route_middleware")
		require.True(t, exists)
		require.Equal(t, true, value)
		c.JSON(http.StatusOK, gin.H{"id": c.Param("widgetId")})
	})
	widgets.POST("", func(c *gin.Context) {
		var input struct {
			Name string `json:"name"`
		}
		require.NoError(t, c.ShouldBindJSON(&input))
		c.Header("X-Bridge", "huma")
		c.JSON(http.StatusCreated, gin.H{"name": input.Name})
	})
	widgets.DELETE("/:widgetId", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	widgets.POST("/:widgetId/refresh", func(c *gin.Context) {
		c.Status(http.StatusAccepted)
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/widgets/widget-1", nil)
	engine.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"id":"widget-1"}`, response.Body.String())
	require.Equal(t, 1, humaCalls)

	createResponse := httptest.NewRecorder()
	createRequest := httptest.NewRequest(http.MethodPost, "/api/v1/widgets", strings.NewReader(`{"name":"created"}`))
	createRequest.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(createResponse, createRequest)
	require.Equal(t, http.StatusCreated, createResponse.Code)
	require.Equal(t, "huma", createResponse.Header().Get("X-Bridge"))
	require.JSONEq(t, `{"name":"created"}`, createResponse.Body.String())
	require.Equal(t, 2, humaCalls)

	invalidResponse := httptest.NewRecorder()
	invalidRequest := httptest.NewRequest(http.MethodPost, "/api/v1/widgets", strings.NewReader(`{}`))
	invalidRequest.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(invalidResponse, invalidRequest)
	require.Equal(t, http.StatusBadRequest, invalidResponse.Code)
	var invalidBody humaCompatibilityError
	require.NoError(t, json.Unmarshal(invalidResponse.Body.Bytes(), &invalidBody))
	require.Equal(t, http.StatusBadRequest, invalidBody.Code)
	require.Equal(t, "request body validation failed", invalidBody.Message)
	require.NotNil(t, invalidBody.Data)
	require.Equal(t, 3, humaCalls)

	deleteResponse := httptest.NewRecorder()
	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/v1/widgets/widget-1", nil)
	engine.ServeHTTP(deleteResponse, deleteRequest)
	require.Equal(t, http.StatusNoContent, deleteResponse.Code)
	require.Empty(t, deleteResponse.Body.String())
	require.Equal(t, 4, humaCalls)

	refreshResponse := httptest.NewRecorder()
	refreshRequest := httptest.NewRequest(http.MethodPost, "/api/v1/widgets/widget-1/refresh", nil)
	engine.ServeHTTP(refreshResponse, refreshRequest)
	require.Equal(t, http.StatusAccepted, refreshResponse.Code)
	require.Empty(t, refreshResponse.Body.String())
	require.Equal(t, 5, humaCalls)

	specResponse := httptest.NewRecorder()
	specRequest := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	engine.ServeHTTP(specResponse, specRequest)
	require.Equal(t, http.StatusOK, specResponse.Code)
	var spec struct {
		Paths map[string]map[string]struct {
			RequestBody *struct {
				Required bool `json:"required"`
				Content  map[string]struct {
					Schema json.RawMessage `json:"schema"`
				} `json:"content"`
			} `json:"requestBody"`
			Responses  map[string]json.RawMessage `json:"responses"`
			Parameters []struct {
				Name     string `json:"name"`
				In       string `json:"in"`
				Required bool   `json:"required"`
			} `json:"parameters"`
		} `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(specResponse.Body.Bytes(), &spec))
	require.Contains(t, spec.Paths, "/api/v1/widgets/{widgetId}")
	require.Contains(t, spec.Paths["/api/v1/widgets/{widgetId}"], "get")
	parameters := spec.Paths["/api/v1/widgets/{widgetId}"]["get"].Parameters
	require.Len(t, parameters, 1)
	require.Equal(t, "widgetId", parameters[0].Name)
	require.Equal(t, "path", parameters[0].In)
	require.True(t, parameters[0].Required)
	createOperation := spec.Paths["/api/v1/widgets"]["post"]
	require.NotNil(t, createOperation.RequestBody)
	require.True(t, createOperation.RequestBody.Required)
	require.Contains(t, createOperation.RequestBody.Content, "application/json")
	require.NotEmpty(t, createOperation.RequestBody.Content["application/json"].Schema)
	require.Contains(t, createOperation.Responses, "201")
	require.Contains(t, createOperation.Responses, "400")
	require.NotContains(t, createOperation.Responses, "200")
	require.NotContains(t, createOperation.Responses, "202")
	refreshOperation := spec.Paths["/api/v1/widgets/{widgetId}/refresh"]["post"]
	require.Nil(t, refreshOperation.RequestBody)
	require.Contains(t, refreshOperation.Responses, "202")

	routes := runtime.Catalog.Routes()
	require.Len(t, routes, 4)
	require.Equal(t, "/api/v1/widgets/{widgetId}", routes[0].Path)
	require.True(t, routes[0].Access.Authenticated)
}

func TestRuntimeUsesConfiguredBodyLimitAndCompatibilityError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	runtime, err := Attach(engine, Options{
		Title: "Test API", Version: "1.0.0", Body: BodyOptions{MaxBytes: 8},
	})
	require.NoError(t, err)
	widgets := runtime.V1.Group("/widgets").WithAccess(Access{Public: true})
	widgets.RegisterContract(http.MethodPost, "", JSONContract(
		struct {
			Name string `json:"name"`
		}{}, nil, http.StatusNoContent, http.StatusBadRequest, http.StatusRequestEntityTooLarge,
	))
	handlerCalled := false
	widgets.POST("", func(c *gin.Context) {
		handlerCalled = true
		c.Status(http.StatusNoContent)
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/widgets", strings.NewReader(`{"name":"too long"}`))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(response, request)
	require.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
	require.False(t, handlerCalled)
	var body humaCompatibilityError
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Equal(t, http.StatusRequestEntityTooLarge, body.Code)
	require.NotEmpty(t, body.Message)
}

func TestRuntimeValidatesFormContractBeforeGinHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	runtime, err := Attach(engine, Options{Title: "Test API", Version: "1.0.0"})
	require.NoError(t, err)
	tokens := runtime.V1.Group("/tokens").WithAccess(Access{Public: true})
	tokens.RegisterContract(http.MethodPost, "", FormContract(
		struct {
			ClientID string `json:"client_id" minLength:"1"`
			Audience string `json:"audience" minLength:"1"`
		}{}, nil, http.StatusNoContent, http.StatusBadRequest,
	))
	handlerCalls := 0
	tokens.POST("", func(c *gin.Context) {
		handlerCalls++
		require.NoError(t, c.Request.ParseForm())
		require.Equal(t, "client-1", c.Request.Form.Get("client_id"))
		c.Status(http.StatusNoContent)
	})

	validForm := url.Values{"client_id": {"client-1"}, "audience": {"paigram"}}
	validResponse := httptest.NewRecorder()
	validRequest := httptest.NewRequest(http.MethodPost, "/api/v1/tokens", strings.NewReader(validForm.Encode()))
	validRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	engine.ServeHTTP(validResponse, validRequest)
	require.Equal(t, http.StatusNoContent, validResponse.Code)
	require.Equal(t, 1, handlerCalls)

	invalidForm := url.Values{"client_id": {"client-1"}}
	invalidResponse := httptest.NewRecorder()
	invalidRequest := httptest.NewRequest(http.MethodPost, "/api/v1/tokens", strings.NewReader(invalidForm.Encode()))
	invalidRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	engine.ServeHTTP(invalidResponse, invalidRequest)
	require.Equal(t, http.StatusBadRequest, invalidResponse.Code)
	require.Equal(t, 1, handlerCalls)
}

func TestRuntimePreservesHandlerManagedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	runtime, err := Attach(engine, Options{
		Title: "Test API", Version: "1.0.0", Body: BodyOptions{MaxBytes: 8},
	})
	require.NoError(t, err)
	tokens := runtime.V1.Group("/tokens").WithAccess(Access{Public: true})
	tokens.RegisterContract(http.MethodPost, "", FormContract(
		struct {
			ClientID string `json:"client_id" minLength:"1"`
		}{}, nil, http.StatusNoContent, http.StatusBadRequest,
	).WithHandlerManagedBody())

	payload := "client_id=" + strings.Repeat("x", 32)
	var received string
	tokens.POST("", func(c *gin.Context) {
		body, readErr := io.ReadAll(c.Request.Body)
		require.NoError(t, readErr)
		received = string(body)
		c.Status(http.StatusNoContent)
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tokens", strings.NewReader(payload))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	engine.ServeHTTP(response, request)
	require.Equal(t, http.StatusNoContent, response.Code)
	require.Equal(t, payload, received)
}

func TestRuntimeValidatesTypedPathAndQueryParameters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	runtime, err := Attach(engine, Options{Title: "Test API", Version: "1.0.0"})
	require.NoError(t, err)
	widgets := runtime.V1.Group("/widgets").WithAccess(Access{Public: true})
	contract := ResponseContract(nil, http.StatusNoContent).WithParameters(
		QueryInteger("page", 1, 1, 100),
	)
	widgets.RegisterContract(http.MethodGet, "/:id", contract)
	handlerCalled := false
	widgets.GET("/:id", func(c *gin.Context) {
		handlerCalled = true
		c.Status(http.StatusNoContent)
	})

	for _, target := range []string{"/api/v1/widgets/not-an-id", "/api/v1/widgets/1?page=invalid"} {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
		require.Equal(t, http.StatusBadRequest, response.Code)
		require.False(t, handlerCalled)
	}

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/widgets/1?page=2", nil))
	require.Equal(t, http.StatusNoContent, response.Code)
	require.True(t, handlerCalled)
}

func TestRuntimeRequiresExplicitContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	runtime, err := Attach(engine, Options{Title: "Test API", Version: "1.0.0"})
	require.NoError(t, err)
	require.PanicsWithValue(t, "HTTP operation contract is required: GET /api/v1/widgets", func() {
		runtime.V1.GET("/widgets", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	})
}

func TestRuntimeCanDisableDocumentationRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	_, err := Attach(engine, Options{
		Title:   "Paigram Account Center API",
		Version: "1.0.0",
		OpenAPI: OpenAPIOptions{Enabled: false},
	})
	require.NoError(t, err)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	engine.ServeHTTP(response, request)
	require.Equal(t, http.StatusNotFound, response.Code)
}
