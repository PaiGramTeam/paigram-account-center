package server

import (
	"context"
	"errors"
	"log/slog"
	"time"

	mihomov2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/mihomo/v2"
	platformv2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v2"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/servicehealth"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/transporttls"
	kratosgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"platform-mihomo-service/internal/conf"
	"platform-mihomo-service/internal/observability"
	"platform-mihomo-service/internal/service"
)

type serviceInfoProvider interface {
	GetServiceInfo() map[string]grpc.ServiceInfo
}

type GRPCServers struct {
	Control *kratosgrpc.Server
	Runtime *kratosgrpc.Server
	Health  *GRPCHealthCoordinator
}

func NewGRPCServersWithReadiness(
	bc *conf.Bootstrap,
	controlSvc *service.PlatformControlService,
	runtimeSvc *service.MihomoRuntimeService,
	readiness servicehealth.Checker,
) (*GRPCServers, error) {
	if controlSvc == nil || runtimeSvc == nil {
		return nil, errors.New("v2 control and runtime services are required")
	}
	if readiness == nil {
		return nil, errors.New("readiness checker is required")
	}
	controlConf := bc.GetServer().GetControl()
	runtimeConf := bc.GetServer().GetRuntime()
	controlTLS, err := transporttls.NewOptionalServerConfig(serverTLSFiles(controlConf))
	if err != nil {
		return nil, err
	}
	runtimeTLS, err := transporttls.NewOptionalServerConfig(serverTLSFiles(runtimeConf))
	if err != nil {
		return nil, err
	}

	controlOptions := []kratosgrpc.ServerOption{
		kratosgrpc.CustomHealth(),
		kratosgrpc.Network(controlConf.GetNetwork()),
		kratosgrpc.Address(controlConf.GetAddr()),
		kratosgrpc.Timeout(time.Duration(controlConf.GetTimeoutSeconds()) * time.Second),
		kratosgrpc.Middleware(
			observability.CorrelationMiddleware(),
			observability.RequestLoggingMiddleware(slog.Default()),
			observability.SanitizedRecoveryMiddleware(slog.Default()),
			controlSvc.ServiceTicketMiddleware(),
		),
	}
	if controlTLS != nil {
		controlOptions = append(controlOptions, kratosgrpc.TLSConfig(controlTLS))
	}
	controlServer := kratosgrpc.NewServer(controlOptions...)
	healthCoordinator := newGRPCHealthCoordinator(readiness, defaultHealthRefreshInterval)
	_ = healthCoordinator.Refresh(context.Background())
	registerHealthServer(controlServer, healthCoordinator.Server())
	platformv2.RegisterPlatformControlServiceServer(controlServer, controlSvc)

	runtimeOptions := []kratosgrpc.ServerOption{
		kratosgrpc.CustomHealth(),
		kratosgrpc.Network(runtimeConf.GetNetwork()),
		kratosgrpc.Address(runtimeConf.GetAddr()),
		kratosgrpc.Timeout(time.Duration(runtimeConf.GetTimeoutSeconds()) * time.Second),
		kratosgrpc.Middleware(
			observability.CorrelationMiddleware(),
			observability.RequestLoggingMiddleware(slog.Default()),
			observability.SanitizedRecoveryMiddleware(slog.Default()),
			controlSvc.ServiceTicketMiddleware(),
		),
	}
	if runtimeTLS != nil {
		runtimeOptions = append(runtimeOptions, kratosgrpc.TLSConfig(runtimeTLS))
	}
	runtimeServer := kratosgrpc.NewServer(runtimeOptions...)
	registerHealthServer(runtimeServer, healthCoordinator.Server())
	mihomov2.RegisterMihomoRuntimeServiceServer(runtimeServer, runtimeSvc)

	return &GRPCServers{Control: controlServer, Runtime: runtimeServer, Health: healthCoordinator}, nil
}

func serverTLSFiles(server *conf.Server_GRPC) transporttls.ServerFiles {
	tlsFiles := server.GetTls()
	return transporttls.ServerFiles{
		CertificateFile: tlsFiles.GetCertificateFile(),
		PrivateKeyFile:  tlsFiles.GetPrivateKeyFile(),
		ClientCAFile:    tlsFiles.GetClientCaFile(),
	}
}

func registerHealthServer(registrar grpc.ServiceRegistrar, healthServer healthpb.HealthServer) {
	if serviceInfo, ok := registrar.(serviceInfoProvider); ok {
		if _, exists := serviceInfo.GetServiceInfo()[healthpb.Health_ServiceDesc.ServiceName]; exists {
			return
		}
	}

	healthpb.RegisterHealthServer(registrar, healthServer)
}
