package botroute

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	pb "paigram/internal/grpc/pb/v1"
	"paigram/internal/model"
)

// Unregister implements UnregisterBotService. The request's service_id is
// used as a fencing token: if the currently registered route belongs to a
// different service_id, this call is rejected. This protects against a stale
// shutdown hook releasing a route that a newer service has already taken
// over. On success, the route row is deleted and an audit entry is written.
func (s *Service) Unregister(ctx context.Context, req *pb.UnregisterBotServiceRequest) (*pb.UnregisterBotServiceResponse, error) {
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

	var existing model.BotRoute
	err := s.db.Where("bot_id = ? AND platform = ?", req.GetBotId(), req.GetPlatform()).First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRouteNotFound
		}
		return nil, fmt.Errorf("load bot route: %w", err)
	}
	if existing.ServiceID != req.GetServiceId() {
		return nil, fmt.Errorf("%w: route owned by %q, caller is %q", ErrServiceIDMismatch, existing.ServiceID, req.GetServiceId())
	}

	if err := s.db.Delete(&existing).Error; err != nil {
		return nil, fmt.Errorf("delete bot route: %w", err)
	}

	s.recordAudit(ctx, req.GetBotId(), req.GetPlatform(), "unregister", req)

	return &pb.UnregisterBotServiceResponse{}, nil
}
