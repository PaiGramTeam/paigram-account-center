package server

import (
	"time"

	mihomov2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/mihomo/v2"
	platformv2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v2"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	kratosgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"platform-mihomo-service/internal/conf"
	"platform-mihomo-service/internal/service"
)

type serviceInfoProvider interface {
	GetServiceInfo() map[string]grpc.ServiceInfo
}

func NewGRPCServer(
	bc *conf.Bootstrap,
	controlSvc *service.PlatformControlService,
	runtimeSvc *service.MihomoRuntimeService,
) *kratosgrpc.Server {
	if controlSvc == nil || runtimeSvc == nil {
		panic("v2 control and runtime services are required")
	}
	grpcConf := bc.GetServer().GetGrpc()

	srv := kratosgrpc.NewServer(
		kratosgrpc.Network(grpcConf.GetNetwork()),
		kratosgrpc.Address(grpcConf.GetAddr()),
		kratosgrpc.Timeout(time.Duration(grpcConf.GetTimeoutSeconds())*time.Second),
		kratosgrpc.Middleware(recovery.Recovery(), controlSvc.ServiceTicketMiddleware()),
	)
	registerHealthServer(srv)

	platformv2.RegisterPlatformControlServiceServer(srv, controlSvc)
	mihomov2.RegisterMihomoRuntimeServiceServer(srv, runtimeSvc)

	return srv
}

func registerHealthServer(registrar grpc.ServiceRegistrar) {
	if serviceInfo, ok := registrar.(serviceInfoProvider); ok {
		if _, exists := serviceInfo.GetServiceInfo()[healthpb.Health_ServiceDesc.ServiceName]; exists {
			return
		}
	}

	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(registrar, healthServer)
}
