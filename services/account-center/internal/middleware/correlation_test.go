package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/correlation"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestCorrelationPropagatesValidHTTPHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(Correlation())
	var captured correlation.Fields
	engine.GET("/test", func(c *gin.Context) {
		captured = correlation.FromContext(c.Request.Context())
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	request.Header.Set(correlation.RequestIDHeader, "request-123")
	request.Header.Set(correlation.TraceParentHeader, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	request.Header.Set(correlation.OperationIDHeader, "operation-123")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)

	assert.Equal(t, "request-123", captured.RequestID)
	assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", captured.TraceID)
	assert.Equal(t, "operation-123", captured.OperationID)
	assert.Equal(t, captured.RequestID, response.Header().Get(correlation.RequestIDHeader))
	assert.Equal(t, captured.TraceParent, response.Header().Get(correlation.TraceParentHeader))
}

func TestCorrelationDoesNotReflectInvalidHTTPHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(Correlation())
	var captured correlation.Fields
	engine.GET("/test", func(c *gin.Context) {
		captured = correlation.FromContext(c.Request.Context())
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	request.Header.Add(correlation.RequestIDHeader, "request-1")
	request.Header.Add(correlation.RequestIDHeader, "request-2")
	request.Header.Set(correlation.TraceParentHeader, "invalid")
	request.Header.Set(correlation.OperationIDHeader, "secret=value")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)

	assert.Regexp(t, `^[0-9a-f]{32}$`, captured.RequestID)
	assert.Regexp(t, `^00-[0-9a-f]{32}-[0-9a-f]{16}-01$`, captured.TraceParent)
	assert.Empty(t, captured.OperationID)
	assert.NotEqual(t, "request-1", response.Header().Get(correlation.RequestIDHeader))
}
