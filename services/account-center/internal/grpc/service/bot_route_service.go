package service

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"paigram/internal/grpc/interceptor"
	pb "paigram/internal/grpc/pb/v1"
	"paigram/internal/service/botroute"
	"paigram/internal/service/credentials"
)

// botRouteScopeRead is the scope required to read a bot's route table
// (telegram-service's RouteCache._fetch_once consumes this).
const botRouteScopeRead = "bot.route.read"

// botRouteScopeManage is the scope required to register/unregister bot
// services (paigram-genshin's startup/shutdown hooks consume this).
//
// Per Path D §10 Q2 + Option D: ownership is enforced by SCOPE ASSIGNMENT,
// not by a row-level owner check. The operator who provisions a credential
// decides which bot ids it may manage via the scope grant; the service
// trusts that decision. A future bot_ownership join table is out of
// scope for Path D.
const botRouteScopeManage = "bot.route.manage"

// BotRouteService is the gRPC adapter for botroute.Service.
type BotRouteService struct {
	pb.UnimplementedBotRouteServiceServer

	svc *botroute.Service
}

// NewBotRouteService wires the gRPC adapter to a configured business service.
func NewBotRouteService(svc *botroute.Service) *BotRouteService {
	return &BotRouteService{svc: svc}
}

// RegisterBotService delegates after enforcing the bot.route.manage
// scope. The bot_id from the request body is used verbatim as the
// audit actor (the legacy "caller must equal request.bot_id" check is
// dropped per Path D §3.3 + Q2; scope grant is the authorization).
func (s *BotRouteService) RegisterBotService(ctx context.Context, req *pb.RegisterBotServiceRequest) (*pb.RegisterBotServiceResponse, error) {
	if _, err := requireRouteScope(ctx, botRouteScopeManage); err != nil {
		return nil, err
	}
	ctx = botroute.WithActor(ctx, req.GetBotId())

	resp, err := s.svc.Register(ctx, req)
	if err != nil {
		return nil, mapBotRouteError(err)
	}
	return resp, nil
}

// UnregisterBotService mirrors RegisterBotService.
func (s *BotRouteService) UnregisterBotService(ctx context.Context, req *pb.UnregisterBotServiceRequest) (*pb.UnregisterBotServiceResponse, error) {
	if _, err := requireRouteScope(ctx, botRouteScopeManage); err != nil {
		return nil, err
	}
	ctx = botroute.WithActor(ctx, req.GetBotId())

	resp, err := s.svc.Unregister(ctx, req)
	if err != nil {
		return nil, mapBotRouteError(err)
	}
	return resp, nil
}

// GetBotRoute looks up a route. The scope required is the lighter
// bot.route.read; managers obviously satisfy it transitively.
func (s *BotRouteService) GetBotRoute(ctx context.Context, req *pb.GetBotRouteRequest) (*pb.GetBotRouteResponse, error) {
	if _, err := requireRouteScope(ctx, botRouteScopeRead, botRouteScopeManage); err != nil {
		return nil, err
	}
	ctx = botroute.WithActor(ctx, req.GetBotId())

	resp, err := s.svc.GetRoute(ctx, req)
	if err != nil {
		return nil, mapBotRouteError(err)
	}
	return resp, nil
}

// requireRouteScope returns the validated claims iff the caller carries
// AT LEAST ONE of the listed scopes (matches how OAuth scope-OR
// semantics are typically encoded). All callers also implicitly satisfy
// the check if the token carries admin.all.
func requireRouteScope(ctx context.Context, anyOf ...string) (*credentials.AccessClaims, error) {
	claims, ok := interceptor.CredentialClaimsFromContext(ctx)
	if !ok || claims == nil {
		return nil, status.Error(codes.Unauthenticated, "credential claims missing")
	}
	for _, scope := range anyOf {
		if credentials.HasScope(claims, scope) {
			return claims, nil
		}
	}
	return nil, status.Error(codes.PermissionDenied, "required bot route scope missing")
}

// mapBotRouteError converts business-layer sentinel errors into gRPC status
// codes. Unknown errors degrade to codes.Internal with the underlying message
// preserved so logs/observability can correlate.
func mapBotRouteError(err error) error {
	switch {
	case errors.Is(err, botroute.ErrInvalidRouteRequest):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, botroute.ErrRouteNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, botroute.ErrServiceIDMismatch):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return status.Errorf(codes.Internal, "bot route operation failed: %v", err)
	}
}
