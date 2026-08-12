package server

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	mihomov2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/mihomo/v2"
	platformv2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v2"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/tlstest"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/transporttls"
	kratosgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

func TestProductionGRPCListenersEnforceIndependentTrustPolicies(t *testing.T) {
	bootstrap, controlBundle, runtimeBundle := newSecureBootstrapFixture(t)
	controlService, runtimeService := testV2Services()
	servers, err := NewGRPCServers(bootstrap, controlService, runtimeService)
	require.NoError(t, err)
	startTestGRPCServer(t, servers.Control)
	startTestGRPCServer(t, servers.Runtime)

	controlConnection := dialTestTLS(t, servers.Control, controlBundle, true)
	runtimeConnection := dialTestTLS(t, servers.Runtime, runtimeBundle, false)
	assertServing(t, controlConnection)
	assertServing(t, runtimeConnection)

	_, err = mihomov2.NewMihomoRuntimeServiceClient(controlConnection).DescribePlatform(context.Background(), &mihomov2.DescribePlatformRequest{})
	require.Equal(t, codes.Unimplemented, status.Code(err))
	_, err = platformv2.NewPlatformControlServiceClient(runtimeConnection).GetBindingState(context.Background(), &platformv2.GetBindingStateRequest{})
	require.Equal(t, codes.Unimplemented, status.Code(err))

	missingClientCertificate, err := transporttls.NewClientConfigLoader(transporttls.ClientFiles{
		RootCAFile: controlBundle.CAFile,
		ServerName: controlBundle.ServerName,
	})
	require.NoError(t, err)
	missingClientConfig, err := missingClientCertificate.Load()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = grpc.DialContext(ctx, endpointHost(t, servers.Control), grpc.WithTransportCredentials(credentials.NewTLS(missingClientConfig)), grpc.WithBlock())
	require.Error(t, err)
}

func TestProductionGRPCListenersExposeDynamicReadiness(t *testing.T) {
	bootstrap, controlBundle, runtimeBundle := newSecureBootstrapFixture(t)
	controlService, runtimeService := testV2Services()
	readiness := &mutableReadiness{}
	servers, err := NewGRPCServersWithReadiness(bootstrap, controlService, runtimeService, readiness)
	require.NoError(t, err)
	startTestGRPCServer(t, servers.Control)
	startTestGRPCServer(t, servers.Runtime)

	controlConnection := dialTestTLS(t, servers.Control, controlBundle, true)
	runtimeConnection := dialTestTLS(t, servers.Runtime, runtimeBundle, false)
	assertServing(t, controlConnection)
	assertServing(t, runtimeConnection)

	readiness.Set(errors.New("database unavailable"))
	require.Error(t, servers.Health.Refresh(context.Background()))
	assertHealthStatus(t, controlConnection, healthpb.HealthCheckResponse_NOT_SERVING)
	assertHealthStatus(t, runtimeConnection, healthpb.HealthCheckResponse_NOT_SERVING)
}

func dialTestTLS(t *testing.T, server endpointServer, bundle tlstest.Bundle, includeClientCertificate bool) *grpc.ClientConn {
	t.Helper()
	files := transporttls.ClientFiles{RootCAFile: bundle.CAFile, ServerName: bundle.ServerName}
	if includeClientCertificate {
		files.CertificateFile = bundle.ClientCertFile
		files.PrivateKeyFile = bundle.ClientKeyFile
	}
	loader, err := transporttls.NewClientConfigLoader(files)
	require.NoError(t, err)
	config, err := loader.Load()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, err := grpc.DialContext(ctx, endpointHost(t, server), grpc.WithTransportCredentials(credentials.NewTLS(config)), grpc.WithBlock(), grpc.WithReturnConnectionError())
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	return connection
}

type endpointServer interface {
	Endpoint() (*url.URL, error)
}

func endpointHost(t *testing.T, server endpointServer) string {
	t.Helper()
	endpoint, err := server.Endpoint()
	require.NoError(t, err)
	return endpoint.Host
}

func startTestGRPCServer(t *testing.T, server *kratosgrpc.Server) {
	t.Helper()
	_, err := server.Endpoint()
	require.NoError(t, err)
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Start(context.Background()) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, server.Stop(ctx))
		err := <-serveErr
		require.True(t, err == nil || err == grpc.ErrServerStopped, "server error: %v", err)
	})
}

func assertServing(t *testing.T, connection *grpc.ClientConn) {
	assertHealthStatus(t, connection, healthpb.HealthCheckResponse_SERVING)
}

func assertHealthStatus(t *testing.T, connection *grpc.ClientConn, want healthpb.HealthCheckResponse_ServingStatus) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response, err := healthpb.NewHealthClient(connection).Check(ctx, &healthpb.HealthCheckRequest{})
	require.NoError(t, err)
	require.Equal(t, want, response.GetStatus())
}
