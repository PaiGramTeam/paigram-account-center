package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

type MetricsServer struct {
	httpServer *http.Server
	listener   net.Listener
}

func NewMetricsServer(address string, metricsHandler http.Handler) *MetricsServer {
	mux := http.NewServeMux()
	mux.Handle("/metrics", metricsHandler)
	return &MetricsServer{httpServer: &http.Server{
		Addr:              address,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}}
}

func (s *MetricsServer) Start(context.Context) error {
	listener := s.listener
	if listener == nil {
		var err error
		listener, err = net.Listen("tcp", s.httpServer.Addr)
		if err != nil {
			return err
		}
	}
	err := s.httpServer.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *MetricsServer) Stop(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
