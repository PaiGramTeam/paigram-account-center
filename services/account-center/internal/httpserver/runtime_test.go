package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	deleteResponse := httptest.NewRecorder()
	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/v1/widgets/widget-1", nil)
	engine.ServeHTTP(deleteResponse, deleteRequest)
	require.Equal(t, http.StatusNoContent, deleteResponse.Code)
	require.Empty(t, deleteResponse.Body.String())
	require.Equal(t, 3, humaCalls)

	refreshResponse := httptest.NewRecorder()
	refreshRequest := httptest.NewRequest(http.MethodPost, "/api/v1/widgets/widget-1/refresh", nil)
	engine.ServeHTTP(refreshResponse, refreshRequest)
	require.Equal(t, http.StatusAccepted, refreshResponse.Code)
	require.Empty(t, refreshResponse.Body.String())
	require.Equal(t, 4, humaCalls)

	specResponse := httptest.NewRecorder()
	specRequest := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	engine.ServeHTTP(specResponse, specRequest)
	require.Equal(t, http.StatusOK, specResponse.Code)
	var spec struct {
		Paths map[string]map[string]struct {
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

	routes := runtime.Catalog.Routes()
	require.Len(t, routes, 4)
	require.Equal(t, "/api/v1/widgets/{widgetId}", routes[0].Path)
	require.True(t, routes[0].Access.Authenticated)
}

func TestCompatibilitySuccessResponsesCoverGinStatusVariants(t *testing.T) {
	require.Contains(t, compatibilitySuccessResponses(http.MethodPost), "201")
	require.Contains(t, compatibilitySuccessResponses(http.MethodPost), "202")
	require.Contains(t, compatibilitySuccessResponses(http.MethodDelete), "204")
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
