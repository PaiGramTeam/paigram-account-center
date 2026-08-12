package server

import (
	"testing"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/tlstest"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/health/grpc_health_v1"

	"platform-mihomo-service/internal/conf"
)

func TestGRPCServersSeparateControlAndRuntimeSurfaces(t *testing.T) {
	controlService, runtimeService := testV2Services()
	servers, err := NewGRPCServersWithReadiness(testSecureBootstrap(t), controlService, runtimeService, testReadyChecker())
	require.NoError(t, err)

	controlServices := servers.Control.GetServiceInfo()
	require.Contains(t, controlServices, "paigram.platform.v2.PlatformControlService")
	require.Contains(t, controlServices, grpc_health_v1.Health_ServiceDesc.ServiceName)
	require.NotContains(t, controlServices, "paigram.mihomo.v2.MihomoRuntimeService")

	runtimeServices := servers.Runtime.GetServiceInfo()
	require.Contains(t, runtimeServices, "paigram.mihomo.v2.MihomoRuntimeService")
	require.Contains(t, runtimeServices, grpc_health_v1.Health_ServiceDesc.ServiceName)
	require.NotContains(t, runtimeServices, "paigram.platform.v2.PlatformControlService")
}

func testSecureBootstrap(t *testing.T) *conf.Bootstrap {
	t.Helper()
	bootstrap, _, _ := newSecureBootstrapFixture(t)
	return bootstrap
}

func newSecureBootstrapFixture(t *testing.T) (*conf.Bootstrap, tlstest.Bundle, tlstest.Bundle) {
	t.Helper()
	control := tlstest.New(t, "control.internal")
	runtime := tlstest.New(t, "runtime.internal")
	return &conf.Bootstrap{Server: &conf.Server{
		Control: secureGRPCConfig(control, true),
		Runtime: secureGRPCConfig(runtime, false),
	}}, control, runtime
}

func secureGRPCConfig(bundle tlstest.Bundle, includeClientCA bool) *conf.Server_GRPC {
	clientCAFile := ""
	if includeClientCA {
		clientCAFile = bundle.CAFile
	}
	return &conf.Server_GRPC{
		Network:        "tcp",
		Addr:           "127.0.0.1:0",
		TimeoutSeconds: 5,
		Tls: &conf.Server_TLS{
			CertificateFile: bundle.ServerCertFile,
			PrivateKeyFile:  bundle.ServerKeyFile,
			ClientCaFile:    clientCAFile,
		},
	}
}
