package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	platformv1 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v1"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/platformaction"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	grpcstatus "google.golang.org/grpc/status"
	"gorm.io/gorm"

	"paigram/internal/config"
	"paigram/internal/grpc/clientauth"
	"paigram/internal/model"
	"paigram/internal/service/botaccess"
	"paigram/internal/service/platformbinding"
	"paigram/internal/serviceticket"
)

var (
	ErrInvalidTicketConfig               = errors.New("invalid service ticket config")
	ErrPlatformSummaryProxyUnavailable   = platformbinding.ErrPlatformSummaryProxyUnavailable
	ErrPlatformServiceUnavailable        = platformbinding.ErrPlatformServiceUnavailable
	ErrConsumerGrantInvalidationRejected = errors.New("consumer grant invalidation rejected by platform service")
)

// ServiceTicketClaims carries actor-scoped platform access metadata.
type ServiceTicketClaims = botaccess.ServiceTicketClaims

// PlatformListView is the browser-facing platform registry list model.
type PlatformListView struct {
	Platform         string   `json:"platform"`
	DisplayName      string   `json:"display_name"`
	SupportedActions []string `json:"supported_actions"`
}

// PlatformSchemaView is the browser-facing platform schema model.
type PlatformSchemaView struct {
	Platform         string         `json:"platform"`
	DisplayName      string         `json:"display_name"`
	SupportedActions []string       `json:"supported_actions"`
	CredentialSchema map[string]any `json:"credential_schema"`
}

type platformSummaryProxy interface {
	GetCredentialSummary(ctx context.Context, endpoint, ticket, platformAccountID string) (map[string]any, error)
}

// PlatformService provides platform registry lookups.
type PlatformService struct {
	db                  *gorm.DB
	ticketSigner        *serviceticket.Signer
	dial                dialFunc
	genericSummaryProxy platformSummaryProxy
	healthChecker       platformHealthChecker
}

func buildPlatformServiceTicketClaims(actorType, actorID string, ownerUserID, platformAccountRefID uint64, platform, platformAccountID string, scopes []string) ServiceTicketClaims {
	return ServiceTicketClaims{
		ActorType:         actorType,
		ActorID:           actorID,
		OwnerUserID:       ownerUserID,
		UserID:            ownerUserID,
		Platform:          platform,
		BindingID:         platformAccountRefID,
		PlatformAccountID: platformAccountID,
		Scopes:            scopes,
	}
}

func buildBindingScopedTicketClaims(actorType, actorID string, ownerUserID, bindingID uint64, platform, platformServiceKey, platformAccountID string, scopes []string) ServiceTicketClaims {
	claims := buildPlatformServiceTicketClaims(actorType, actorID, ownerUserID, bindingID, platform, platformAccountID, scopes)
	claims.PlatformServiceKey = platformServiceKey
	return claims
}

func isSupportedInternalActorType(actorType string) bool {
	switch actorType {
	case "user", "admin", "consumer":
		return true
	default:
		return false
	}
}

// ConfigureAuth loads service ticket signing settings from auth config.
func (s *PlatformService) ConfigureAuth(authCfg config.AuthConfig) error {
	signer, err := serviceticket.NewSigner(serviceticket.Config{
		Issuer:        authCfg.ServiceTicketIssuer,
		KeyID:         authCfg.ServiceTicketKeyID,
		TTL:           time.Duration(authCfg.ServiceTicketTTLSeconds) * time.Second,
		PrivateKeyPEM: authCfg.ServiceTicketPrivateKeyPEM,
	})
	if err != nil {
		return ErrInvalidTicketConfig
	}
	s.ticketSigner = signer
	return nil
}

// ListEnabledPlatforms returns all enabled platform registry entries.
func (s *PlatformService) ListEnabledPlatforms() ([]model.PlatformService, error) {
	var platforms []model.PlatformService
	if err := s.db.Where("enabled = ?", true).Order("platform_key ASC").Find(&platforms).Error; err != nil {
		return nil, err
	}

	return platforms, nil
}

// GetEnabledPlatform returns an enabled platform registry entry by key.
func (s *PlatformService) GetEnabledPlatform(platformKey string) (*model.PlatformService, error) {
	var platform model.PlatformService
	if err := s.db.Where("platform_key = ? AND enabled = ?", platformKey, true).First(&platform).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}

		return nil, err
	}

	return &platform, nil
}

// ListEnabledPlatformViews returns enabled platform entries decoded for handler responses.
func (s *PlatformService) ListEnabledPlatformViews() ([]PlatformListView, error) {
	platforms, err := s.ListEnabledPlatforms()
	if err != nil {
		return nil, err
	}

	views := make([]PlatformListView, 0, len(platforms))
	for _, platform := range platforms {
		supportedActions, err := parseStringListJSON(platform.SupportedActionsJSON)
		if err != nil {
			return nil, err
		}

		views = append(views, PlatformListView{
			Platform:         platform.PlatformKey,
			DisplayName:      platform.DisplayName,
			SupportedActions: supportedActions,
		})
	}

	return views, nil
}

// GetPlatformSchemaView returns a decoded schema view for an enabled platform.
func (s *PlatformService) GetPlatformSchemaView(platformKey string) (*PlatformSchemaView, error) {
	platform, err := s.GetEnabledPlatform(platformKey)
	if err != nil {
		return nil, err
	}

	supportedActions, err := parseStringListJSON(platform.SupportedActionsJSON)
	if err != nil {
		return nil, err
	}

	credentialSchema, err := parseObjectJSON(platform.CredentialSchemaJSON)
	if err != nil {
		return nil, err
	}

	return &PlatformSchemaView{
		Platform:         platform.PlatformKey,
		DisplayName:      platform.DisplayName,
		SupportedActions: supportedActions,
		CredentialSchema: credentialSchema,
	}, nil
}

// IssueLegacyRefScopedTicket signs a short-lived service ticket for a legacy platform account ref.
// Migration-only: do not use for new runtime platform binding flows.
func (s *PlatformService) IssueLegacyRefScopedTicket(actorType, actorID string, ownerUserID uint64, ref *model.PlatformAccountRef, scopes []string, audience string) (string, time.Time, error) {
	if s.ticketSigner == nil {
		return "", time.Time{}, ErrInvalidTicketConfig
	}
	if ref == nil || ref.Status != model.PlatformAccountRefStatusActive {
		return "", time.Time{}, gorm.ErrRecordNotFound
	}
	if actorType == "" || actorID == "" || audience == "" || !isSupportedInternalActorType(actorType) {
		return "", time.Time{}, ErrInvalidTicketConfig
	}

	claims := buildBindingScopedTicketClaims(actorType, actorID, ownerUserID, ref.ID, ref.Platform, ref.PlatformServiceKey, ref.PlatformAccountID, scopes)
	claims.AllowedActions = scopes
	return s.ticketSigner.Issue(ticketTypeForActor(actorType), ticketSubject(actorType, actorID, ownerUserID), audience, claims)
}

func (s *PlatformService) IssueBindingScopedTicket(actorType, actorID string, binding *model.PlatformAccountBinding, scopes []string) (string, time.Time, error) {
	if s.ticketSigner == nil {
		return "", time.Time{}, ErrInvalidTicketConfig
	}
	if binding == nil || actorType == "" || actorID == "" || !isSupportedInternalActorType(actorType) {
		return "", time.Time{}, ErrInvalidTicketConfig
	}

	platformRow, err := s.GetEnabledPlatform(binding.Platform)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", time.Time{}, ErrPlatformServiceUnavailable
		}
		return "", time.Time{}, err
	}

	claims := buildBindingScopedTicketClaims(actorType, actorID, binding.OwnerUserID, binding.ID, binding.Platform, binding.PlatformServiceKey, nullableBindingExternalAccountKey(binding.ExternalAccountKey), scopes)
	claims.AllowedActions = scopes
	return s.ticketSigner.Issue(ticketTypeForActor(actorType), ticketSubject(actorType, actorID, binding.OwnerUserID), platformRow.ServiceAudience, claims)
}

func (s *PlatformService) InvalidateConsumerGrant(ctx context.Context, input platformbinding.GrantInvalidationInput) error {
	if s.ticketSigner == nil {
		return ErrInvalidTicketConfig
	}
	if input.BindingID == 0 || input.Platform == "" || input.PlatformServiceKey == "" || input.Consumer == "" || input.MinimumGrantVersion == 0 {
		return ErrInvalidTicketConfig
	}

	platformRow, err := s.getEnabledPlatformService(input.Platform, input.PlatformServiceKey)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPlatformServiceUnavailable
		}
		return err
	}

	actorType := input.ActorType
	actorID := input.ActorID
	if actorType == "" || actorID == "" || actorType == "consumer" {
		actorType = "user"
		actorID = "system:grant-revoke"
	}
	if actorType != "user" && actorType != "admin" {
		return ErrInvalidTicketConfig
	}

	claims := buildBindingScopedTicketClaims(actorType, actorID, input.OwnerUserID, input.BindingID, input.Platform, input.PlatformServiceKey, "", []string{platformaction.MihomoConsumerGrantInvalidate})
	claims.AllowedActions = claims.Scopes
	signed, _, err := s.ticketSigner.Issue(serviceticket.TypeControl, ticketSubject(actorType, actorID, input.OwnerUserID), platformRow.ServiceAudience, claims)
	if err != nil {
		return err
	}

	dial := s.dial
	if dial == nil {
		dial = func(ctx context.Context, endpoint string) (*grpc.ClientConn, error) {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			return grpc.DialContext(ctx, endpoint,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithBlock(),
			)
		}
	}

	conn, err := dial(ctx, platformRow.Endpoint)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPlatformServiceUnavailable, err)
	}
	defer conn.Close()

	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	callCtx = clientauth.WithServiceTicket(callCtx, signed)

	resp, err := platformv1.NewPlatformServiceClient(conn).InvalidateConsumerGrant(callCtx, &platformv1.InvalidateConsumerGrantRequest{
		BindingId:           input.BindingID,
		Consumer:            input.Consumer,
		MinimumGrantVersion: input.MinimumGrantVersion,
	})
	if err != nil && isPlatformUnavailableRPCError(err) {
		return fmt.Errorf("%w: %v", ErrPlatformServiceUnavailable, err)
	}
	if err != nil {
		return err
	}
	if resp == nil || !resp.GetSuccess() {
		return ErrConsumerGrantInvalidationRejected
	}
	return nil
}

func ticketTypeForActor(actorType string) string {
	if actorType == "consumer" {
		return serviceticket.TypeDelegation
	}
	return serviceticket.TypeControl
}

func ticketSubject(actorType, actorID string, ownerUserID uint64) string {
	if actorType == "consumer" {
		return "consumer:" + actorID
	}
	if actorType == "system" {
		return "system:account-center"
	}
	return fmt.Sprintf("user:%d", ownerUserID)
}

func (s *PlatformService) getEnabledPlatformService(platformKey, serviceKey string) (*model.PlatformService, error) {
	var platform model.PlatformService
	if err := s.db.Where("platform_key = ? AND service_key = ? AND enabled = ?", platformKey, serviceKey, true).First(&platform).Error; err != nil {
		return nil, err
	}
	return &platform, nil
}

func isPlatformUnavailableRPCError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if st, ok := grpcstatus.FromError(err); ok {
		return st.Code() == codes.Unavailable || st.Code() == codes.DeadlineExceeded
	}
	return false
}

func (s *PlatformService) SetGenericSummaryProxy(proxy platformSummaryProxy) {
	s.genericSummaryProxy = proxy
}

func (s *PlatformService) SetHealthChecker(checker platformHealthChecker) {
	s.healthChecker = checker
}

func (s *PlatformService) GetPlatformAccountSummary(ctx context.Context, actorType, actorID string, ownerUserID, platformAccountRefID uint64, scopes []string) (map[string]any, error) {
	if bindingSummary, ok, err := s.getBindingSummary(ctx, ownerUserID, platformAccountRefID); err != nil {
		return nil, err
	} else if ok {
		return bindingSummary, nil
	}

	if s.genericSummaryProxy == nil {
		return nil, gorm.ErrRecordNotFound
	}

	var ref model.PlatformAccountRef
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", platformAccountRefID, ownerUserID).First(&ref).Error; err != nil {
		return nil, err
	}

	platform, err := s.GetEnabledPlatform(ref.Platform)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPlatformServiceUnavailable
		}
		return nil, err
	}

	ticket, _, err := s.IssueLegacyRefScopedTicket(actorType, actorID, ownerUserID, &ref, scopes, platform.ServiceAudience)
	if err != nil {
		return nil, err
	}

	return s.genericSummaryProxy.GetCredentialSummary(ctx, platform.Endpoint, ticket, ref.PlatformAccountID)
}

func (s *PlatformService) GetBindingRuntimeSummary(ctx context.Context, actorType, actorID string, binding *model.PlatformAccountBinding, scopes []string) (map[string]any, error) {
	if binding == nil {
		return nil, gorm.ErrRecordNotFound
	}
	if !binding.ExternalAccountKey.Valid {
		return nil, gorm.ErrRecordNotFound
	}
	if s.genericSummaryProxy == nil {
		return nil, ErrPlatformSummaryProxyUnavailable
	}

	platformRow, err := s.GetEnabledPlatform(binding.Platform)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPlatformServiceUnavailable
		}
		return nil, err
	}

	ticket, _, err := s.IssueBindingScopedTicket(actorType, actorID, binding, scopes)
	if err != nil {
		return nil, err
	}

	platformAccountID := nullableBindingExternalAccountKey(binding.ExternalAccountKey)
	return s.genericSummaryProxy.GetCredentialSummary(ctx, platformRow.Endpoint, ticket, platformAccountID)
}

func (s *PlatformService) ConfirmBindingPrimaryProfile(ctx context.Context, actorType, actorID string, binding *model.PlatformAccountBinding, playerID string) error {
	if binding == nil || playerID == "" || !binding.ExternalAccountKey.Valid || binding.ExternalAccountKey.String == "" {
		return gorm.ErrRecordNotFound
	}
	if binding.Platform != "mihomo" {
		return ErrPlatformServiceUnavailable
	}

	platformRow, err := s.GetEnabledPlatform(binding.Platform)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPlatformServiceUnavailable
		}
		return err
	}

	ticket, _, err := s.IssueBindingScopedTicket(actorType, actorID, binding, []string{platformaction.MihomoProfileWrite})
	if err != nil {
		return err
	}

	dial := s.dial
	if dial == nil {
		dial = func(ctx context.Context, endpoint string) (*grpc.ClientConn, error) {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			return grpc.DialContext(ctx, endpoint,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithBlock(),
			)
		}
	}

	conn, err := dial(ctx, platformRow.Endpoint)
	if err != nil {
		return err
	}
	defer conn.Close()

	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	callCtx = clientauth.WithServiceTicket(callCtx, ticket)

	_, err = platformv1.NewPlatformServiceClient(conn).ConfirmPrimaryProfile(callCtx, &platformv1.ConfirmPrimaryProfileRequest{
		PlatformAccountId: nullableBindingExternalAccountKey(binding.ExternalAccountKey),
		PlayerId:          playerID,
	})
	return err
}

func (s *PlatformService) getBindingSummary(ctx context.Context, ownerUserID, bindingID uint64) (map[string]any, bool, error) {
	var binding model.PlatformAccountBinding
	err := s.db.WithContext(ctx).
		Preload("Profiles", func(db *gorm.DB) *gorm.DB {
			return db.Order("is_primary DESC").Order("id ASC")
		}).
		Where("id = ? AND owner_user_id = ?", bindingID, ownerUserID).
		First(&binding).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}

	profiles := make([]map[string]any, 0, len(binding.Profiles))
	for _, profile := range binding.Profiles {
		profiles = append(profiles, map[string]any{
			"id":                   profile.ID,
			"platform_profile_key": profile.PlatformProfileKey,
			"game_biz":             profile.GameBiz,
			"region":               profile.Region,
			"player_uid":           profile.PlayerUID,
			"nickname":             profile.Nickname,
			"level":                nullableBindingSummaryInt(profile.Level),
			"is_primary":           profile.IsPrimary,
			"source_updated_at":    nullableBindingSummaryTime(profile.SourceUpdatedAt),
		})
	}

	return map[string]any{
		"binding_id":            binding.ID,
		"platform":              binding.Platform,
		"external_account_key":  nullableBindingSummaryString(binding.ExternalAccountKey),
		"platform_service_key":  binding.PlatformServiceKey,
		"display_name":          binding.DisplayName,
		"status":                binding.Status,
		"status_reason_code":    binding.StatusReasonCode,
		"status_reason_message": binding.StatusReasonMessage,
		"primary_profile_id":    nullableBindingSummaryInt(binding.PrimaryProfileID),
		"last_validated_at":     nullableBindingSummaryTime(binding.LastValidatedAt),
		"last_refreshed_at":     nullableBindingSummaryTime(binding.LastSyncedAt),
		"last_synced_at":        nullableBindingSummaryTime(binding.LastSyncedAt),
		"profiles":              profiles,
	}, true, nil
}

func nullableBindingSummaryInt(value sql.NullInt64) any {
	if !value.Valid {
		return nil
	}
	return value.Int64
}

func nullableBindingSummaryTime(value sql.NullTime) any {
	if !value.Valid {
		return nil
	}
	return value.Time
}

func nullableBindingSummaryString(value sql.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}

func nullableBindingExternalAccountKey(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func parseStringListJSON(raw string) ([]string, error) {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	return values, nil
}

func parseObjectJSON(raw string) (map[string]any, error) {
	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, err
	}
	return value, nil
}
