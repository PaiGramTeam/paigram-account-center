package router

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/servicehealth"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHealthRoutesKeepLivenessUpWhenReadinessFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	registerHealthRoutes(engine, servicehealth.CheckFunc(func(context.Context) error {
		return errors.New("database password leaked if returned")
	}))

	live := performHealthRequest(engine, "/livez")
	ready := performHealthRequest(engine, "/readyz")
	compatibility := performHealthRequest(engine, "/healthz")

	require.Equal(t, http.StatusOK, live.Code)
	require.Equal(t, http.StatusServiceUnavailable, ready.Code)
	require.NotContains(t, ready.Body.String(), "password")
	require.Equal(t, http.StatusServiceUnavailable, compatibility.Code)
}

func TestReadyRouteSucceedsWhenDependenciesAreReady(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	registerHealthRoutes(engine, servicehealth.CheckFunc(func(context.Context) error { return nil }))

	ready := performHealthRequest(engine, "/readyz")

	require.Equal(t, http.StatusOK, ready.Code)
}

func performHealthRequest(handler http.Handler, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}
