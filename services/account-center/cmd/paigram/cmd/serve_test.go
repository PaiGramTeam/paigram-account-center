package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type testHTTPServer struct {
	called bool
	err    error
}

func (s *testHTTPServer) Shutdown(context.Context) error {
	s.called = true
	return s.err
}

type testGRPCServer struct {
	called bool
	began  bool
	events *[]string
}

func (s *testGRPCServer) BeginShutdown() {
	s.began = true
	if s.events != nil {
		*s.events = append(*s.events, "grpc-not-serving")
	}
}

func (s *testGRPCServer) Stop() {
	s.called = true
	if s.events != nil {
		*s.events = append(*s.events, "grpc-stop")
	}
}

type testShutdowner struct {
	called bool
}

func (s *testShutdowner) Shutdown() {
	s.called = true
}

type testCloser struct {
	called bool
	err    error
}

func (s *testCloser) Close() error {
	s.called = true
	return s.err
}

func TestShutdownServicesStopsAllTargets(t *testing.T) {
	httpServer := &testHTTPServer{}
	grpcServer := &testGRPCServer{}
	asynqServer := &testShutdowner{}
	asynqScheduler := &testShutdowner{}
	emailService := &testCloser{}

	err := shutdownServices(context.Background(), shutdownTargets{
		httpServer:     httpServer,
		grpcServer:     grpcServer,
		asynqServer:    asynqServer,
		asynqScheduler: asynqScheduler,
		emailService:   emailService,
	})

	require.NoError(t, err)
	require.True(t, httpServer.called)
	require.True(t, grpcServer.called)
	require.True(t, asynqServer.called)
	require.True(t, asynqScheduler.called)
	require.True(t, emailService.called)
}

func TestShutdownServicesAggregatesErrors(t *testing.T) {
	httpErr := errors.New("http shutdown failed")
	emailErr := errors.New("email close failed")

	err := shutdownServices(context.Background(), shutdownTargets{
		httpServer:   &testHTTPServer{err: httpErr},
		emailService: &testCloser{err: emailErr},
	})

	require.Error(t, err)
	require.ErrorIs(t, err, httpErr)
	require.ErrorIs(t, err, emailErr)
}

func TestShutdownServicesMarksNotReadyBeforeDrainingTransports(t *testing.T) {
	events := []string{}
	readiness := &orderedShutdowner{name: "http-not-ready", events: &events}
	grpcServer := &testGRPCServer{events: &events}
	httpServer := &orderedHTTPServer{events: &events}

	require.NoError(t, shutdownServices(context.Background(), shutdownTargets{
		readiness:  readiness,
		httpServer: httpServer,
		grpcServer: grpcServer,
	}))

	require.Equal(t, []string{"http-not-ready", "grpc-not-serving", "http-stop", "grpc-stop"}, events)
}

type orderedShutdowner struct {
	name   string
	events *[]string
}

func (s *orderedShutdowner) Shutdown() {
	*s.events = append(*s.events, s.name)
}

type orderedHTTPServer struct {
	events *[]string
}

func (s *orderedHTTPServer) Shutdown(context.Context) error {
	*s.events = append(*s.events, "http-stop")
	return nil
}

// TestHTTPServerHasTimeouts verifies that the HTTP server constructed
// for production use has Slowloris-resistant timeouts and a bounded
// header size. V16: HTTP server has no timeouts.
func TestHTTPServerHasTimeouts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	srv := buildHTTPServer("127.0.0.1:0", engine)

	require.NotNil(t, srv)
	require.Equal(t, "127.0.0.1:0", srv.Addr)
	require.NotZero(t, srv.ReadHeaderTimeout, "ReadHeaderTimeout must be set to mitigate Slowloris")
	require.NotZero(t, srv.ReadTimeout, "ReadTimeout must be set")
	require.NotZero(t, srv.WriteTimeout, "WriteTimeout must be set")
	require.NotZero(t, srv.IdleTimeout, "IdleTimeout must be set")
	require.Greater(t, srv.MaxHeaderBytes, 0, "MaxHeaderBytes must be bounded")
}
