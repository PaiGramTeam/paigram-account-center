package botroute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	botv1 "github.com/PaiGramTeam/proto-contracts/bot/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"gorm.io/gorm"

	pb "paigram/internal/grpc/pb/v1"
	"paigram/internal/model"
)

// GetRoute implements GetBotRoute. It returns the currently registered
// (service_id, endpoint, version, handlers, last_heartbeat) for the given
// (bot_id, platform) tuple. Callers that don't find a route should not
// retry blindly; absence here is authoritative.
func (s *Service) GetRoute(ctx context.Context, req *pb.GetBotRouteRequest) (*pb.GetBotRouteResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: nil request", ErrInvalidRouteRequest)
	}
	if req.GetBotId() == "" {
		return nil, fmt.Errorf("%w: bot_id is required", ErrInvalidRouteRequest)
	}
	if req.GetPlatform() == "" {
		return nil, fmt.Errorf("%w: platform is required", ErrInvalidRouteRequest)
	}

	var row model.BotRoute
	err := s.db.Where("bot_id = ? AND platform = ?", req.GetBotId(), req.GetPlatform()).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRouteNotFound
		}
		return nil, fmt.Errorf("load bot route: %w", err)
	}

	handlers, err := unmarshalHandlerDeclarations(row.HandlersJSON)
	if err != nil {
		return nil, fmt.Errorf("decode handlers: %w", err)
	}

	resp := &pb.GetBotRouteResponse{
		ServiceId: row.ServiceID,
		Endpoint:  row.Endpoint,
		Version:   row.Version,
		Handlers:  handlers,
	}
	if row.LastHeartbeatAt.Valid {
		resp.LastHeartbeatAtUnix = row.LastHeartbeatAt.Time.Unix()
	}
	return resp, nil
}

// unmarshalHandlerDeclarations reverses marshalHandlerDeclarations from
// register_service.go. An empty or null payload decodes to a nil slice
// rather than an error.
func unmarshalHandlerDeclarations(raw []byte) ([]*botv1.HandlerDeclaration, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, err
	}
	out := make([]*botv1.HandlerDeclaration, 0, len(parts))
	for _, part := range parts {
		h := &botv1.HandlerDeclaration{}
		opts := protojson.UnmarshalOptions{DiscardUnknown: true}
		if err := opts.Unmarshal(part, h); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, nil
}
