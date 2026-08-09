package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

	widgets := runtime.V1.Group("/widgets").WithAccess(Access{Authenticated: true})
	widgets.GET("/:widgetId", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"id": c.Param("widgetId")})
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/widgets/widget-1", nil)
	engine.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"id":"widget-1"}`, response.Body.String())

	specResponse := httptest.NewRecorder()
	specRequest := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	engine.ServeHTTP(specResponse, specRequest)
	require.Equal(t, http.StatusOK, specResponse.Code)
	var spec struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(specResponse.Body.Bytes(), &spec))
	require.Contains(t, spec.Paths, "/api/v1/widgets/{widgetId}")
	require.Contains(t, spec.Paths["/api/v1/widgets/{widgetId}"], "get")

	routes := runtime.Catalog.Routes()
	require.Len(t, routes, 1)
	require.Equal(t, "/api/v1/widgets/{widgetId}", routes[0].Path)
	require.True(t, routes[0].Access.Authenticated)
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
