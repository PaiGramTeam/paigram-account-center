package server

import (
	"fmt"
	"log"
	"net"

	accountv1 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/account/v1"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"gorm.io/gorm"

	"paigram/internal/config"
	"paigram/internal/grpc/interceptor"
	pb "paigram/internal/grpc/pb/v1"
	grpcservice "paigram/internal/grpc/service"
	"paigram/internal/observability"
	"paigram/internal/service/botaccess"
	"paigram/internal/service/botroute"
	"paigram/internal/service/credentials"
)

// GRPCServer represents the gRPC server
type GRPCServer struct {
	port            int
	db              *gorm.DB
	redisClient     *redis.Client
	cfg             *config.Config
	server          *grpc.Server
	authInterceptor *interceptor.AuthInterceptor
}

// NewGRPCServer creates a new gRPC server
func NewGRPCServer(port int, db *gorm.DB, redisClient *redis.Client, cfg *config.Config) (*GRPCServer, error) {
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

	// Create gRPC server with interceptors
	opts := []grpc.ServerOption{
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

	botAccessGroup, err := botaccess.NewServiceGroup(db, cfg.Auth)
	if err != nil {
		return nil, fmt.Errorf("init bot access services: %w", err)
	}
	accountv1.RegisterBotAccessServiceServer(server, grpcservice.NewBotAccessService(&botAccessGroup.AccountRefService, &botAccessGroup.TicketService, db))

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
	}, nil
}

// Start starts the gRPC server
func (s *GRPCServer) Start() error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	log.Printf("gRPC server listening on port %d", s.port)

	// This is a blocking call
	if err := s.server.Serve(lis); err != nil {
		return fmt.Errorf("failed to serve: %w", err)
	}

	return nil
}

// Stop gracefully stops the gRPC server
func (s *GRPCServer) Stop() {
	log.Println("Stopping gRPC server...")
	s.server.GracefulStop()
}
