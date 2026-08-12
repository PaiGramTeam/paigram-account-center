package platform

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"strings"

	"paigram/internal/dberror"
	"paigram/internal/model"
)

const staticPlatformDiscoveryType = "static"

func (s *PlatformService) ListPlatformServices(ctx context.Context) ([]PlatformServiceAdminView, error) {
	var rows []model.PlatformService
	if err := s.db.WithContext(ctx).Order("platform_key ASC").Find(&rows).Error; err != nil {
		return nil, err
	}

	views := make([]PlatformServiceAdminView, 0, len(rows))
	for _, row := range rows {
		view, err := s.buildPlatformServiceAdminView(ctx, row)
		if err != nil {
			return nil, err
		}
		views = append(views, *view)
	}

	return views, nil

}

func (s *PlatformService) GetPlatformService(ctx context.Context, id uint64) (*PlatformServiceAdminView, error) {
	row, err := s.loadPlatformService(ctx, id)
	if err != nil {
		return nil, err
	}

	return s.buildPlatformServiceAdminView(ctx, *row)
}

func (s *PlatformService) GetPlatformServiceConfig(ctx context.Context, id uint64) (*UpdatePlatformServiceInput, error) {
	row, err := s.loadPlatformService(ctx, id)
	if err != nil {
		return nil, err
	}

	supportedActions, err := parseStringListJSON(row.SupportedActionsJSON)
	if err != nil {
		return nil, err
	}
	credentialSchema, err := parseObjectJSON(row.CredentialSchemaJSON)
	if err != nil {
		return nil, err
	}

	return &UpdatePlatformServiceInput{
		PlatformKey:       row.PlatformKey,
		DisplayName:       row.DisplayName,
		ServiceKey:        row.ServiceKey,
		ServiceAudience:   row.ServiceAudience,
		DiscoveryType:     row.DiscoveryType,
		ControlEndpoint:   row.ControlEndpoint,
		RuntimeEndpoint:   row.RuntimeEndpoint,
		RuntimeServerName: row.RuntimeServerName,
		Enabled:           row.Enabled,
		SupportedActions:  supportedActions,
		CredentialSchema:  credentialSchema,
	}, nil
}

func (s *PlatformService) CreatePlatformService(ctx context.Context, input CreatePlatformServiceInput) (*PlatformServiceAdminView, error) {
	row, err := buildPlatformServiceModel(input)
	if err != nil {
		return nil, err
	}

	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		if dberror.IsUniqueViolation(err) {
			return nil, ErrPlatformServiceConflict
		}
		return nil, err
	}

	return s.buildPlatformServiceAdminView(ctx, *row)
}

func (s *PlatformService) UpdatePlatformService(ctx context.Context, id uint64, input UpdatePlatformServiceInput) (*PlatformServiceAdminView, error) {
	row, err := s.loadPlatformService(ctx, id)
	if err != nil {
		return nil, err
	}

	if err := applyPlatformServiceUpdate(row, input); err != nil {
		return nil, err
	}

	if err := s.db.WithContext(ctx).Save(row).Error; err != nil {
		if dberror.IsUniqueViolation(err) {
			return nil, ErrPlatformServiceConflict
		}
		return nil, err
	}

	return s.buildPlatformServiceAdminView(ctx, *row)
}

func (s *PlatformService) DeletePlatformService(ctx context.Context, id uint64) error {
	row, err := s.loadPlatformService(ctx, id)
	if err != nil {
		return err
	}

	var bindings int64
	if err := s.db.WithContext(ctx).Model(&model.PlatformAccountBinding{}).
		Where("platform = ? AND platform_service_key = ?", row.PlatformKey, row.ServiceKey).
		Count(&bindings).Error; err != nil {
		return err
	}
	if bindings > 0 {
		return ErrPlatformServiceReferenced
	}

	return s.db.WithContext(ctx).Delete(&model.PlatformService{}, id).Error
}

func (s *PlatformService) CheckPlatformService(ctx context.Context, id uint64) (*PlatformServiceAdminView, error) {
	row, err := s.loadPlatformService(ctx, id)
	if err != nil {
		return nil, err
	}

	return s.buildPlatformServiceAdminView(ctx, *row)
}

func (s *PlatformService) loadPlatformService(ctx context.Context, id uint64) (*model.PlatformService, error) {
	var row model.PlatformService
	if err := s.db.WithContext(ctx).First(&row, id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func buildPlatformServiceModel(input CreatePlatformServiceInput) (*model.PlatformService, error) {
	supportedActionsJSON, credentialSchemaJSON, err := marshalPlatformServiceConfig(input.SupportedActions, input.CredentialSchema)
	if err != nil {
		return nil, err
	}

	row := &model.PlatformService{
		PlatformKey:          input.PlatformKey,
		DisplayName:          input.DisplayName,
		ServiceKey:           input.ServiceKey,
		ServiceAudience:      input.ServiceAudience,
		DiscoveryType:        input.DiscoveryType,
		ControlEndpoint:      input.ControlEndpoint,
		RuntimeEndpoint:      input.RuntimeEndpoint,
		RuntimeServerName:    input.RuntimeServerName,
		Enabled:              input.Enabled,
		SupportedActionsJSON: supportedActionsJSON,
		CredentialSchemaJSON: credentialSchemaJSON,
	}
	normalizePlatformServiceRow(row)
	if err := validatePlatformServiceRow(row); err != nil {
		return nil, err
	}

	return row, nil
}

func applyPlatformServiceUpdate(row *model.PlatformService, input UpdatePlatformServiceInput) error {
	supportedActionsJSON, credentialSchemaJSON, err := marshalPlatformServiceConfig(input.SupportedActions, input.CredentialSchema)
	if err != nil {
		return err
	}
	originalPlatformKey := row.PlatformKey
	originalServiceKey := row.ServiceKey

	row.PlatformKey = input.PlatformKey
	row.DisplayName = input.DisplayName
	row.ServiceKey = input.ServiceKey
	row.ServiceAudience = input.ServiceAudience
	row.DiscoveryType = input.DiscoveryType
	row.ControlEndpoint = input.ControlEndpoint
	row.RuntimeEndpoint = input.RuntimeEndpoint
	row.RuntimeServerName = input.RuntimeServerName
	row.Enabled = input.Enabled
	row.SupportedActionsJSON = supportedActionsJSON
	row.CredentialSchemaJSON = credentialSchemaJSON
	normalizePlatformServiceRow(row)
	if row.PlatformKey != originalPlatformKey || row.ServiceKey != originalServiceKey {
		return ErrInvalidPlatformServiceConfig
	}

	return validatePlatformServiceRow(row)
}

func normalizePlatformServiceRow(row *model.PlatformService) {
	if row == nil {
		return
	}

	row.PlatformKey = strings.TrimSpace(row.PlatformKey)
	row.DisplayName = strings.TrimSpace(row.DisplayName)
	row.ServiceKey = strings.TrimSpace(row.ServiceKey)
	row.ServiceAudience = strings.TrimSpace(row.ServiceAudience)
	row.DiscoveryType = strings.TrimSpace(row.DiscoveryType)
	row.ControlEndpoint = strings.TrimSpace(row.ControlEndpoint)
	row.RuntimeEndpoint = strings.TrimSpace(row.RuntimeEndpoint)
	row.RuntimeServerName = strings.TrimSpace(row.RuntimeServerName)
}

func validatePlatformServiceRow(row *model.PlatformService) error {
	if row == nil {
		return ErrInvalidPlatformServiceConfig
	}
	if strings.TrimSpace(row.PlatformKey) == "" || strings.TrimSpace(row.DisplayName) == "" || strings.TrimSpace(row.ServiceKey) == "" || strings.TrimSpace(row.ServiceAudience) == "" || strings.TrimSpace(row.ControlEndpoint) == "" || strings.TrimSpace(row.RuntimeEndpoint) == "" || strings.TrimSpace(row.RuntimeServerName) == "" {
		return ErrInvalidPlatformServiceConfig
	}
	if row.DiscoveryType != staticPlatformDiscoveryType {
		return ErrInvalidPlatformServiceConfig
	}
	if !validGRPCEndpoint(row.ControlEndpoint) || !validGRPCEndpoint(row.RuntimeEndpoint) || !validTLSServerName(row.RuntimeServerName) {
		return ErrInvalidPlatformServiceConfig
	}

	return nil
}

func validGRPCEndpoint(endpoint string) bool {
	if endpoint == "" || strings.Contains(endpoint, "://") || strings.ContainsAny(endpoint, "/?#") {
		return false
	}
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil || strings.TrimSpace(host) == "" || strings.ContainsAny(host, " \t\r\n") {
		return false
	}
	numericPort, err := strconv.ParseUint(port, 10, 16)
	return err == nil && numericPort > 0
}

func validTLSServerName(serverName string) bool {
	return serverName != "" && strings.TrimSpace(serverName) == serverName &&
		!strings.Contains(serverName, "://") && !strings.ContainsAny(serverName, "/?#: \t\r\n")
}

func marshalPlatformServiceConfig(supportedActions []string, credentialSchema map[string]any) (string, string, error) {
	if supportedActions == nil {
		supportedActions = []string{}
	}
	if credentialSchema == nil {
		credentialSchema = map[string]any{}
	}

	supportedActionsJSON, err := json.Marshal(supportedActions)
	if err != nil {
		return "", "", ErrInvalidPlatformServiceConfig
	}
	credentialSchemaJSON, err := json.Marshal(credentialSchema)
	if err != nil {
		return "", "", ErrInvalidPlatformServiceConfig
	}

	return string(supportedActionsJSON), string(credentialSchemaJSON), nil
}

func (s *PlatformService) buildPlatformServiceAdminView(ctx context.Context, row model.PlatformService) (*PlatformServiceAdminView, error) {
	supportedActions, err := parseStringListJSON(row.SupportedActionsJSON)
	if err != nil {
		return nil, err
	}
	credentialSchema, err := parseObjectJSON(row.CredentialSchemaJSON)
	if err != nil {
		return nil, err
	}

	view := &PlatformServiceAdminView{
		ID:                row.ID,
		PlatformKey:       row.PlatformKey,
		DisplayName:       row.DisplayName,
		ServiceKey:        row.ServiceKey,
		ServiceAudience:   row.ServiceAudience,
		DiscoveryType:     row.DiscoveryType,
		ControlEndpoint:   row.ControlEndpoint,
		RuntimeEndpoint:   row.RuntimeEndpoint,
		RuntimeServerName: row.RuntimeServerName,
		Enabled:           row.Enabled,
		SupportedActions:  supportedActions,
		CredentialSchema:  credentialSchema,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}

	s.decorateRuntimeState(ctx, row, view)
	return view, nil
}

func (s *PlatformService) decorateRuntimeState(ctx context.Context, row model.PlatformService, view *PlatformServiceAdminView) {
	if !row.Enabled {
		view.ConfigState = ConfigStateDisabled
		view.RuntimeState = RuntimeStateDisabled
		view.CheckedAt = nil
		view.Error = ""
		return
	}

	view.ConfigState = ConfigStateEnabled
	if s.healthChecker == nil || row.DiscoveryType != staticPlatformDiscoveryType || strings.TrimSpace(row.ControlEndpoint) == "" || strings.TrimSpace(row.ServiceAudience) == "" {
		view.RuntimeState = RuntimeStateMisconfigured
		view.CheckedAt = nil
		view.Error = "platform service is misconfigured"
		return
	}

	result := s.healthChecker.Check(ctx, row.ControlEndpoint)
	view.RuntimeState = result.State
	view.CheckedAt = &result.CheckedAt
	view.Error = result.Error
}
