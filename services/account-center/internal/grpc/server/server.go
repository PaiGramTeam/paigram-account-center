package server

import (
	"fmt"
	"log"
	"net"
	"time"

	accountv1 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/account/v1"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/servicehealth"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/transporttls"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	grpccredentials "google.golang.org/grpc/credentials"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"gorm.io/gorm"

	"paigram/internal/config"
	"paigram/internal/grpc/interceptor"
	pb "paigram/internal/grpc/pb/v1"
	grpcservice "paigram/internal/grpc/service"
	"paigram/internal/healthcheck"
	"paigram/internal/observability"
	"paigram/internal/service/botaccess"
	"paigram/internal/service/botroute"
	"paigram/internal/service/credentials"
	"paigram/internal/serviceticket"
)

// GRPCServer represents the gRPC server
type GRPCServer struct {
	port            int
	db              *gorm.DB
	redisClient     *redis.Client
	cfg             *config.Config
	server          *grpc.Server
	authInterceptor *interceptor.AuthInterceptor
	health          *grpcHealthCoordinator
}

// NewGRPCServer creates a new gRPC server
func NewGRPCServer(port int, db *gorm.DB, redisClient *redis.Client, cfg *config.Config) (*GRPCServer, error) {
	if cfg == nil {
		return nil, fmt.Errorf("grpc config is required")
	}
	ticketSigner, err := serviceticket.NewFileSigner(
		cfg.Auth.ServiceTicketIssuer,
		time.Duration(cfg.Auth.ServiceTicketTTLSeconds)*time.Second,
		cfg.Auth.ServiceTicketSigningKeyFile,
	)
	if err != nil {
		return nil, fmt.Errorf("init service ticket signer: %w", err)
	}
	return NewGRPCServerWithTicketSigner(port, db, redisClient, cfg, ticketSigner)
}

func NewGRPCServerWithTicketSigner(port int, db *gorm.DB, redisClient *redis.Client, cfg *config.Config, ticketSigner serviceticket.Signer) (*GRPCServer, error) {
	return NewGRPCServerWithTicketSignerAndReadiness(
		port, db, redisClient, cfg, ticketSigner,
		healthcheck.NewReadiness(db, redisClient, cfg != nil && cfg.Redis.Enabled),
	)
}

func NewGRPCServerWithTicketSignerAndReadiness(port int, db *gorm.DB, redisClient *redis.Client, cfg *config.Config, ticketSigner serviceticket.Signer, readiness servicehealth.Checker) (*GRPCServer, error) {
	if cfg == nil {
		return nil, fmt.Errorf("grpc config is required")
	}

	// OAuth access tokens and platform service tickets use independent keys.
	credentialsRegistry := credentials.NewService(db)
	tokenSvc, err := credentials.NewTokenService(credentialsRegistry, credentials.TokenServiceConfig{
		Issuer:                cfg.Auth.OAuthIssuer,
		AccessTokenTTLSeconds: cfg.Auth.OAuthAccessTokenTTLSeconds,
		SigningKey:            []byte(cfg.Auth.OAuthSigningKey),
	})
	if err != nil {
		return nil, fmt.Errorf("init oauth token service: %w", err)
	}

	authInterceptor := interceptor.NewAuthInterceptor(tokenSvc)
	tlsConfig, err := transporttls.NewServerConfig(transporttls.ServerFiles{
		CertificateFile: cfg.GRPC.CertificateFile,
		PrivateKeyFile:  cfg.GRPC.PrivateKeyFile,
	}, transporttls.ServerAuthOnly)
	if err != nil {
		return nil, fmt.Errorf("init grpc TLS: %w", err)
	}

	// Create gRPC server with interceptors
	opts := []grpc.ServerOption{
		grpc.Creds(grpccredentials.NewTLS(tlsConfig)),
		grpc.ChainUnaryInterceptor(
			observability.UnaryServerInterceptor(),
			authInterceptor.Unary(),
		),
		grpc.ChainStreamInterceptor(
			observability.StreamServerInterceptor(),
			authInterceptor.Stream(),
		),
	}

	server := grpc.NewServer(opts...)
	healthCoordinator := newGRPCHealthCoordinator(readiness, defaultHealthRefreshInterval)
	healthpb.RegisterHealthServer(server, healthCoordinator.Server())

	botAccessGroup, err := botaccess.NewServiceGroupWithSigner(db, ticketSigner)
	if err != nil {
		return nil, fmt.Errorf("init bot access services: %w", err)
	}
	accountv1.RegisterBotAccessServiceServer(server, grpcservice.NewBotAccessService(&botAccessGroup.BindingAccessService, &botAccessGroup.TicketService, db))

	botRouteService := botroute.NewService(db, zap.L())
	pb.RegisterBotRouteServiceServer(server, grpcservice.NewBotRouteService(botRouteService))

	// Register reflection service for debugging
	reflection.Register(server)

	return &GRPCServer{
		port:            port,
		db:              db,
		redisClient:     redisClient,
		cfg:             cfg,
		server:          server,
		authInterceptor: authInterceptor,
		health:          healthCoordinator,
	}, nil
}

// Start starts the gRPC server
func (s *GRPCServer) Start() error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	log.Printf("gRPC server listening on port %d", s.port)
	s.health.Start()
	defer s.health.Shutdown()

	// This is a blocking call
	if err := s.server.Serve(lis); err != nil {
		return fmt.Errorf("failed to serve: %w", err)
	}

	return nil
}

// Stop gracefully stops the gRPC server
func (s *GRPCServer) BeginShutdown() {
	s.health.Shutdown()
}

// Stop gracefully stops the gRPC server.
func (s *GRPCServer) Stop() {
	log.Println("Stopping gRPC server...")
	s.BeginShutdown()
	s.server.GracefulStop()
}
