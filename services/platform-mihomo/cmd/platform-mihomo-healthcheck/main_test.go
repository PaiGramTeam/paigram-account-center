package main

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/tlstest"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/transporttls"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestCheckRuntimeHealthRequiresServingStatusOverTLS(t *testing.T) {
	bundle := tlstest.New(t, "runtime.internal")
	tlsConfig, err := transporttls.NewServerConfig(transporttls.ServerFiles{
		CertificateFile: bundle.ServerCertFile,
		PrivateKeyFile:  bundle.ServerKeyFile,
	}, transporttls.ServerAuthOnly)
	require.NoError(t, err)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	healthServer := health.NewServer()
	grpcServer := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig)))
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(grpcServer.Stop)

	options := healthcheckOptions{
		Target:     listener.Addr().String(),
		RootCAFile: bundle.CAFile,
		ServerName: bundle.ServerName,
		Timeout:    2 * time.Second,
	}
	require.NoError(t, checkRuntimeHealth(context.Background(), options))

	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	require.Error(t, checkRuntimeHealth(context.Background(), options))
	options.Service = "liveness"
	healthServer.SetServingStatus("liveness", healthpb.HealthCheckResponse_SERVING)
	require.NoError(t, checkRuntimeHealth(context.Background(), options))
}
