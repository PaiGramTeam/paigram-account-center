package botroute

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	botv1 "github.com/PaiGramTeam/proto-contracts/bot/v1"
	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	pb "paigram/internal/grpc/pb/v1"
	"paigram/internal/model"
)

// ErrInvalidRouteRequest is returned by Register / Unregister when the
// caller's request fails non-RPC-level validation (empty required field,
// inconsistent service id, ...). gRPC adapters map this to InvalidArgument.
var ErrInvalidRouteRequest = errors.New("invalid bot route request")

// ErrRouteNotFound is returned by Unregister / GetRoute when no row exists
// for the (bot_id, platform) tuple. gRPC adapters map this to NotFound.
var ErrRouteNotFound = errors.New("bot route not found")

// ErrServiceIDMismatch is returned by Unregister when the request's
// service_id does not match the currently registered owner of the route.
// This protects against late shutdown callbacks releasing a route that has
// already been taken over by a newer service. gRPC adapters map this to
// FailedPrecondition.
var ErrServiceIDMismatch = errors.New("bot route service_id mismatch")

// Register implements RegisterBotService. It validates the request, upserts
// the (bot_id, platform) row with the new service binding, and writes an
// append-only audit entry. Audit failures are logged but never returned to
// the caller — primary route writes must remain durable even if the audit
// pipeline is impaired.
func (s *Service) Register(ctx context.Context, req *pb.RegisterBotServiceRequest) (*pb.RegisterBotServiceResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: nil request", ErrInvalidRouteRequest)
	}
	if req.GetBotId() == "" {
		return nil, fmt.Errorf("%w: bot_id is required", ErrInvalidRouteRequest)
	}
	if req.GetPlatform() == "" {
		return nil, fmt.Errorf("%w: platform is required", ErrInvalidRouteRequest)
	}
	if req.GetServiceId() == "" {
		return nil, fmt.Errorf("%w: service_id is required", ErrInvalidRouteRequest)
	}
	if req.GetEndpoint() == "" {
		return nil, fmt.Errorf("%w: endpoint is required", ErrInvalidRouteRequest)
	}

	handlersJSON, err := marshalHandlerDeclarations(req.GetHandlers())
	if err != nil {
		return nil, fmt.Errorf("marshal handlers: %w", err)
	}

	now := time.Now().UTC()
	route := &model.BotRoute{
		BotID:           req.GetBotId(),
		Platform:        req.GetPlatform(),
		ServiceID:       req.GetServiceId(),
		Endpoint:        req.GetEndpoint(),
		HandlersJSON:    handlersJSON,
		Version:         req.GetVersion(),
		LastHeartbeatAt: sql.NullTime{Time: now, Valid: true},
	}
	if err := model.UpsertBotRoute(s.db, route).Error; err != nil {
		return nil, fmt.Errorf("upsert bot route: %w", err)
	}

	s.recordAudit(ctx, req.GetBotId(), req.GetPlatform(), "register", req)

	return &pb.RegisterBotServiceResponse{RegisteredAtUnix: now.Unix()}, nil
}

// marshalHandlerDeclarations serializes the repeated HandlerDeclaration
// payload as a JSON array, preserving the protobuf enum names for
// human-readable storage. Empty input encodes as "[]".
func marshalHandlerDeclarations(handlers []*botv1.HandlerDeclaration) ([]byte, error) {
	if len(handlers) == 0 {
		return []byte("[]"), nil
	}
	marshaler := protojson.MarshalOptions{UseEnumNumbers: false, EmitUnpopulated: false}
	parts := make([]json.RawMessage, 0, len(handlers))
	for _, h := range handlers {
		raw, err := marshaler.Marshal(h)
		if err != nil {
			return nil, err
		}
		parts = append(parts, json.RawMessage(raw))
	}
	return json.Marshal(parts)
}

// recordAudit writes an append-only audit row. Failures are logged but not
// surfaced to the caller; this is intentional — the primary route write has
// already succeeded by the time we reach here, and a failed audit must not
// retroactively appear as a failed registration to the caller.
//
// Proto message payloads are serialized with protojson so the stored
// representation uses the proto snake_case field names (matching the
// adjacent handlers_json column produced by marshalHandlerDeclarations).
// Non-proto payloads (e.g. admin map[string]any event descriptors) fall
// back to encoding/json.
func (s *Service) recordAudit(ctx context.Context, botID, platform, action string, payload any) {
	body, err := marshalAuditPayload(payload)
	if err != nil {
		body = []byte("{}")
	}
	row := model.BotRouteAudit{
		BotID:    botID,
		Platform: platform,
		Action:   action,
		Payload:  body,
		Actor:    actorFromContext(ctx),
	}
	if err := s.db.Create(&row).Error; err != nil {
		s.logger.Warn("bot route audit insert failed",
			zap.String("bot_id", botID),
			zap.String("platform", platform),
			zap.String("action", action),
			zap.Error(err),
		)
	}
}

// marshalAuditPayload serializes audit payloads with the right encoder:
// protojson for proto messages (so audit rows match proto field naming),
// encoding/json for everything else.
func marshalAuditPayload(payload any) ([]byte, error) {
	if msg, ok := payload.(proto.Message); ok {
		return protojson.Marshal(msg)
	}
	return json.Marshal(payload)
}
