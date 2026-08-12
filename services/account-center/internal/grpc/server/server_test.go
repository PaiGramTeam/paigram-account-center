package server

import (
	"testing"

	"github.com/stretchr/testify/require"

	"paigram/internal/config"
	"paigram/internal/testutil"
)

func TestNewGRPCServerBuildsRegisteredServer(t *testing.T) {
	t.Parallel()

	authConfig, _ := testutil.NewAuthConfig(t)
	grpcServer, err := NewGRPCServer(50051, nil, nil, &config.Config{Auth: authConfig})

	require.NoError(t, err)
	require.NotNil(t, grpcServer)
	require.NotNil(t, grpcServer.server)
}
