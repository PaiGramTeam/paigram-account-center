package platformtransport

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

func TestControlDialerUsesMutualTLS(t *testing.T) {
	bundle := tlstest.New(t, "control.internal")
	endpoint := startMutualTLSServer(t, bundle)
	dial, err := NewControlDialer(ControlConfig{
		RootCAFile:      bundle.CAFile,
		CertificateFile: bundle.ClientCertFile,
		PrivateKeyFile:  bundle.ClientKeyFile,
		ServerName:      bundle.ServerName,
		Timeout:         2 * time.Second,
	})
	require.NoError(t, err)

	connection, err := dial(context.Background(), endpoint)
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	response, err := healthpb.NewHealthClient(connection).Check(context.Background(), &healthpb.HealthCheckRequest{})
	require.NoError(t, err)
	require.Equal(t, healthpb.HealthCheckResponse_SERVING, response.GetStatus())
}

func TestControlDialerFailsClosedForMissingOrWrongIdentity(t *testing.T) {
	bundle := tlstest.New(t, "control.internal")
	endpoint := startMutualTLSServer(t, bundle)

	_, err := NewControlDialer(ControlConfig{RootCAFile: bundle.CAFile, ServerName: bundle.ServerName, Timeout: time.Second})
	require.Error(t, err)

	wrongNameDialer, err := NewControlDialer(ControlConfig{
		RootCAFile:      bundle.CAFile,
		CertificateFile: bundle.ClientCertFile,
		PrivateKeyFile:  bundle.ClientKeyFile,
		ServerName:      "wrong.internal",
		Timeout:         time.Second,
	})
	require.NoError(t, err)
	_, err = wrongNameDialer(context.Background(), endpoint)
	require.Error(t, err)

	untrustedClient := tlstest.New(t, "untrusted.internal")
	untrustedDialer, err := NewControlDialer(ControlConfig{
		RootCAFile:      bundle.CAFile,
		CertificateFile: untrustedClient.ClientCertFile,
		PrivateKeyFile:  untrustedClient.ClientKeyFile,
		ServerName:      bundle.ServerName,
		Timeout:         time.Second,
	})
	require.NoError(t, err)
	_, err = untrustedDialer(context.Background(), endpoint)
	require.Error(t, err)

	validDialer, err := NewControlDialer(ControlConfig{
		RootCAFile:      bundle.CAFile,
		CertificateFile: bundle.ClientCertFile,
		PrivateKeyFile:  bundle.ClientKeyFile,
		ServerName:      bundle.ServerName,
		Timeout:         time.Second,
	})
	require.NoError(t, err)
	_, err = validDialer(context.Background(), "https://"+endpoint)
	require.Error(t, err)
}

func startMutualTLSServer(t *testing.T, bundle tlstest.Bundle) string {
	t.Helper()
	config, err := transporttls.NewServerConfig(transporttls.ServerFiles{
		CertificateFile: bundle.ServerCertFile,
		PrivateKeyFile:  bundle.ServerKeyFile,
		ClientCAFile:    bundle.CAFile,
	}, transporttls.MutualTLS)
	require.NoError(t, err)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(config)))
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(server, healthServer)
	serveError := make(chan error, 1)
	go func() { serveError <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
		err := <-serveError
		require.True(t, err == nil || err == grpc.ErrServerStopped, "serve error: %v", err)
	})
	return listener.Addr().String()
}
