package botroute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	botv1 "github.com/PaiGramTeam/proto-contracts/bot/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"gorm.io/gorm"

	"paigram/internal/model"
)

// BotRouteAdminView is the admin REST projection of a bot route. Handlers
// are decoded to JSON-compatible Go values so the admin UI can render them
// without needing protobuf descriptors on the frontend.
type BotRouteAdminView struct {
	ID                  uint64                `json:"id"`
	BotID               string                `json:"bot_id"`
	Platform            string                `json:"platform"`
	ServiceID           string                `json:"service_id"`
	Endpoint            string                `json:"endpoint"`
	Version             string                `json:"version"`
	Handlers            []BotRouteHandlerView `json:"handlers"`
	LastHeartbeatAtUnix *int64                `json:"last_heartbeat_at_unix,omitempty"`
	CreatedAtUnix       int64                 `json:"created_at_unix"`
	UpdatedAtUnix       int64                 `json:"updated_at_unix"`
}

// BotRouteHandlerView is a stripped-down representation of HandlerDeclaration
// suitable for JSON rendering. The enum is exposed as its string name so
// admins don't need to know the numeric value.
type BotRouteHandlerView struct {
	Command     string `json:"command"`
	Description string `json:"description,omitempty"`
	Visibility  string `json:"visibility,omitempty"`
}

// ListBotRoutes returns every persisted route, sorted by (bot_id, platform).
// The admin UI is expected to do its own client-side filtering for now;
// server-side pagination can follow once the table grows.
func (s *Service) ListBotRoutes(ctx context.Context) ([]BotRouteAdminView, error) {
	var rows []model.BotRoute
	if err := s.db.WithContext(ctx).Order("bot_id ASC, platform ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list bot routes: %w", err)
	}

	views := make([]BotRouteAdminView, 0, len(rows))
	for _, row := range rows {
		view, err := buildBotRouteAdminView(row)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

// GetBotRouteByID returns one route by primary key. The caller receives
// ErrRouteNotFound (wrapping gorm.ErrRecordNotFound) when the id is unknown
// so the HTTP layer can map to a 404.
func (s *Service) GetBotRouteByID(ctx context.Context, id uint64) (*BotRouteAdminView, error) {
	var row model.BotRoute
	if err := s.db.WithContext(ctx).First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRouteNotFound
		}
		return nil, fmt.Errorf("get bot route: %w", err)
	}
	view, err := buildBotRouteAdminView(row)
	if err != nil {
		return nil, err
	}
	return &view, nil
}

// DeleteBotRoute removes a route by id and writes an "admin_delete" audit
// row. Audit failures are downgraded to warnings to match Register's
// log-and-continue contract.
func (s *Service) DeleteBotRoute(ctx context.Context, id uint64) error {
	var row model.BotRoute
	if err := s.db.WithContext(ctx).First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRouteNotFound
		}
		return fmt.Errorf("load bot route: %w", err)
	}
	if err := s.db.WithContext(ctx).Delete(&row).Error; err != nil {
		return fmt.Errorf("delete bot route: %w", err)
	}
	payload := map[string]any{
		"id":         row.ID,
		"service_id": row.ServiceID,
		"endpoint":   row.Endpoint,
	}
	s.recordAudit(ctx, row.BotID, row.Platform, "admin_delete", payload)
	return nil
}

func buildBotRouteAdminView(row model.BotRoute) (BotRouteAdminView, error) {
	handlers, err := decodeHandlerViews(row.HandlersJSON)
	if err != nil {
		return BotRouteAdminView{}, err
	}
	view := BotRouteAdminView{
		ID:            row.ID,
		BotID:         row.BotID,
		Platform:      row.Platform,
		ServiceID:     row.ServiceID,
		Endpoint:      row.Endpoint,
		Version:       row.Version,
		Handlers:      handlers,
		CreatedAtUnix: row.CreatedAt.Unix(),
		UpdatedAtUnix: row.UpdatedAt.Unix(),
	}
	if row.LastHeartbeatAt.Valid {
		ts := row.LastHeartbeatAt.Time.Unix()
		view.LastHeartbeatAtUnix = &ts
	}
	return view, nil
}

func decodeHandlerViews(raw []byte) ([]BotRouteHandlerView, error) {
	if len(raw) == 0 {
		return []BotRouteHandlerView{}, nil
	}
	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, fmt.Errorf("decode handlers: %w", err)
	}
	opts := protojson.UnmarshalOptions{DiscardUnknown: true}
	out := make([]BotRouteHandlerView, 0, len(parts))
	for _, part := range parts {
		h := &botv1.HandlerDeclaration{}
		if err := opts.Unmarshal(part, h); err != nil {
			return nil, fmt.Errorf("decode handler: %w", err)
		}
		out = append(out, BotRouteHandlerView{
			Command:     h.GetCommand(),
			Description: h.GetDescription(),
			Visibility:  h.GetVisibility().String(),
		})
	}
	return out, nil
}
