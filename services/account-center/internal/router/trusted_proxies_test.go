package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTrustedProxyControlsForwardedClientAddress(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, test := range []struct {
		name       string
		remoteAddr string
		want       string
	}{
		{name: "trusted proxy", remoteAddr: "10.77.20.10:12345", want: "203.0.113.8"},
		{name: "untrusted peer", remoteAddr: "10.77.20.11:12345", want: "10.77.20.11"},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := gin.New()
			require.NoError(t, configureTrustedProxies(engine, []string{"10.77.20.10"}))
			engine.GET("/client-ip", func(c *gin.Context) {
				c.String(http.StatusOK, c.ClientIP())
			})

			request := httptest.NewRequest(http.MethodGet, "/client-ip", nil)
			request.RemoteAddr = test.remoteAddr
			request.Header.Set("X-Forwarded-For", "203.0.113.8")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)

			require.Equal(t, http.StatusOK, response.Code)
			require.Equal(t, test.want, response.Body.String())
		})
	}
}

func TestConfigureTrustedProxiesRejectsInvalidEntry(t *testing.T) {
	engine := gin.New()

	require.Error(t, configureTrustedProxies(engine, []string{"not-an-address"}))
}
