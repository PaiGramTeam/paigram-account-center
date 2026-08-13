package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/correlation"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestRequestLoggerWritesCorrelationWithoutRequestSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, observed := observer.New(zapcore.InfoLevel)
	engine := gin.New()
	engine.Use(Correlation(), RequestLogger(zap.New(core)))
	engine.GET("/accounts/:account", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/accounts/private-account?token=secret-query", nil)
	request.Header.Set(correlation.RequestIDHeader, "request-log")
	request.Header.Set(correlation.TraceParentHeader, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	request.Header.Set("Authorization", "Bearer secret-ticket")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)

	entry := observed.All()[0]
	fields := entry.ContextMap()
	assert.Equal(t, "request-log", fields["request_id"])
	assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", fields["trace_id"])
	assert.Equal(t, "/accounts/:account", fields["route"])
	assert.EqualValues(t, http.StatusNoContent, fields["status_code"])
	assert.NotContains(t, entry.Message, "secret")
	assert.NotContains(t, fields, "authorization")
	assert.NotContains(t, fields, "query")
}

func TestRequestLoggerUsesStatusSeverity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, observed := observer.New(zapcore.DebugLevel)
	engine := gin.New()
	engine.Use(Correlation(), RequestLogger(zap.New(core)))
	engine.GET("/client-error", func(c *gin.Context) { c.Status(http.StatusBadRequest) })
	engine.GET("/server-error", func(c *gin.Context) { c.Status(http.StatusInternalServerError) })

	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/client-error", nil))
	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/server-error", nil))

	entries := observed.All()
	assert.Equal(t, zapcore.WarnLevel, entries[0].Level)
	assert.Equal(t, zapcore.ErrorLevel, entries[1].Level)
}
