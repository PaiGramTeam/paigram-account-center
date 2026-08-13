package server

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricsServerServesAndStops(t *testing.T) {
	server := NewMetricsServer("127.0.0.1:0", http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte("metric 1\n"))
	}))
	listener, err := net.Listen("tcp", server.httpServer.Addr)
	require.NoError(t, err)
	server.listener = listener
	done := make(chan error, 1)
	go func() { done <- server.Start(context.Background()) }()

	response, err := http.Get("http://" + listener.Addr().String() + "/metrics")
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, "metric 1\n", string(body))

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, server.Stop(stopCtx))
	require.NoError(t, <-done)
}
