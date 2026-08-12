package server

import (
	"testing"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/tlstest"
	"github.com/stretchr/testify/require"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"paigram/internal/config"
	"paigram/internal/testutil"
)

func TestNewGRPCServerBuildsRegisteredServer(t *testing.T) {
	t.Parallel()

	authConfig, _ := testutil.NewAuthConfig(t)
	tlsBundle := tlstest.New(t, "account.internal")
	grpcServer, err := NewGRPCServer(50051, nil, nil, &config.Config{
		Auth: authConfig,
		GRPC: config.GRPCConfig{CertificateFile: tlsBundle.ServerCertFile, PrivateKeyFile: tlsBundle.ServerKeyFile},
	})

	require.NoError(t, err)
	require.NotNil(t, grpcServer)
	require.NotNil(t, grpcServer.server)
	require.Contains(t, grpcServer.server.GetServiceInfo(), healthpb.Health_ServiceDesc.ServiceName)
}

func TestNewGRPCServerRejectsMissingTLSIdentity(t *testing.T) {
	authConfig, _ := testutil.NewAuthConfig(t)
	_, err := NewGRPCServer(50051, nil, nil, &config.Config{Auth: authConfig})
	require.Error(t, err)
}
