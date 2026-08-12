package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	platformv2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v2"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/operationid"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/platformaction"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"

	"paigram/internal/config"
	"paigram/internal/grpc/clientauth"
	"paigram/internal/model"
	"paigram/internal/platformtransport"
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
	Platform          string   `json:"platform"`
	DisplayName       string   `json:"display_name"`
	RuntimeEndpoint   string   `json:"runtime_endpoint"`
	RuntimeServerName string   `json:"runtime_server_name"`
	ServiceAudience   string   `json:"service_audience"`
	SupportedActions  []string `json:"supported_actions"`
}

// PlatformSchemaView is the browser-facing platform schema model.
type PlatformSchemaView struct {
	Platform         string         `json:"platform"`
	DisplayName      string         `json:"display_name"`
	SupportedActions []string       `json:"supported_actions"`
	CredentialSchema map[string]any `json:"credential_schema"`
}

type platformSummaryProxy interface {
	GetCredentialSummary(ctx context.Context, endpoint, ticket, bindingRef, accountKey string) (map[string]any, error)
}

// PlatformService provides platform registry lookups.
type PlatformService struct {
	db                  *gorm.DB
	ticketSigner        serviceticket.Signer
	dial                dialFunc
	genericSummaryProxy platformSummaryProxy
	healthChecker       platformHealthChecker
}

func buildPlatformServiceTicketClaims(actorType, actorID, bindingRef, platform, accountKey string, scopes []string) ServiceTicketClaims {
	return ServiceTicketClaims{
		ActorType:  actorType,
		ActorID:    actorID,
		Platform:   platform,
		BindingRef: bindingRef,
		AccountKey: accountKey,
		Scopes:     scopes,
	}
}

func buildBindingScopedTicketClaims(actorType, actorID, bindingRef, platform, platformServiceKey, accountKey string, scopes []string) ServiceTicketClaims {
	claims := buildPlatformServiceTicketClaims(actorType, actorID, bindingRef, platform, accountKey, scopes)
	claims.PlatformServiceKey = platformServiceKey
	return claims
}

func isSupportedInternalActorType(actorType string) bool {
	switch actorType {
	case "user", "admin", "consumer", "system":
		return true
	default:
		return false
	}
}

// ConfigureAuth loads service ticket signing settings from auth config.
func (s *PlatformService) ConfigureAuth(authCfg config.AuthConfig) error {
	signer, err := serviceticket.NewFileSigner(
		authCfg.ServiceTicketIssuer,
		time.Duration(authCfg.ServiceTicketTTLSeconds)*time.Second,
		authCfg.ServiceTicketSigningKeyFile,
	)
	if err != nil {
		return ErrInvalidTicketConfig
	}
	return s.ConfigureTicketSigner(signer)

}

func (s *PlatformService) ConfigureTicketSigner(signer serviceticket.Signer) error {
	if signer == nil {
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
			Platform:          platform.PlatformKey,
			DisplayName:       platform.DisplayName,
			RuntimeEndpoint:   platform.RuntimeEndpoint,
			RuntimeServerName: platform.RuntimeServerName,
			ServiceAudience:   platform.ServiceAudience,
			SupportedActions:  supportedActions,
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

func (s *PlatformService) IssueBindingScopedTicket(actorType, actorID string, binding *model.PlatformAccountBinding, scopes []string) (string, time.Time, error) {
	return s.issueBindingScopedTicket(actorType, actorID, binding, "", "", scopes)
}

func (s *PlatformService) IssueBindingScopedOperationTicket(actorType, actorID string, binding *model.PlatformAccountBinding, operationID string, scopes []string) (string, time.Time, error) {
	if operationID == "" {
		return "", time.Time{}, ErrInvalidTicketConfig
	}
	return s.issueBindingScopedTicket(actorType, actorID, binding, "", operationID, scopes)
}

func (s *PlatformService) IssueProfileScopedOperationTicket(actorType, actorID string, binding *model.PlatformAccountBinding, profileRef, operationID string, scopes []string) (string, time.Time, error) {
	if profileRef == "" || operationID == "" {
		return "", time.Time{}, ErrInvalidTicketConfig
	}
	return s.issueBindingScopedTicket(actorType, actorID, binding, profileRef, operationID, scopes)
}

func (s *PlatformService) issueBindingScopedTicket(actorType, actorID string, binding *model.PlatformAccountBinding, profileRef, operationID string, scopes []string) (string, time.Time, error) {
	if s.ticketSigner == nil {
		return "", time.Time{}, ErrInvalidTicketConfig
	}
	if binding == nil || binding.BindingRef == "" || binding.PlatformServiceKey == "" || binding.OwnerUserID == 0 || len(scopes) != 1 || scopes[0] == "" || actorType == "" || actorID == "" || !isSupportedInternalActorType(actorType) {
		return "", time.Time{}, ErrInvalidTicketConfig
	}

	platformRow, err := s.GetEnabledPlatform(binding.Platform)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", time.Time{}, ErrPlatformServiceUnavailable
		}
		return "", time.Time{}, err
	}

	claims := buildBindingScopedTicketClaims(actorType, actorID, binding.BindingRef, binding.Platform, binding.PlatformServiceKey, nullableBindingExternalAccountKey(binding.ExternalAccountKey), scopes)
	var owner model.User
	if err := s.db.Select("user_ref", "owner_epoch").First(&owner, binding.OwnerUserID).Error; err != nil {
		return "", time.Time{}, err
	}
	claims.OwnerUserRef = owner.UserRef
	claims.OwnerEpoch = owner.OwnerEpoch
	claims.CredentialGeneration = binding.Generation
	claims.OperationID = operationID
	claims.ProfileRef = profileRef
	claims.AllowedActions = scopes
	return s.ticketSigner.Issue(ticketTypeForActor(actorType), ticketSubject(actorType, actorID, owner.UserRef), platformRow.ServiceAudience, claims)
}

func (s *PlatformService) InvalidateConsumerGrant(ctx context.Context, input platformbinding.GrantInvalidationInput) error {
	if s.ticketSigner == nil {
		return ErrInvalidTicketConfig
	}
	if input.BindingRef == "" || input.Platform == "" || input.PlatformServiceKey == "" || input.Consumer == "" || input.MinimumGrantVersion == 0 {
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

	operationFingerprint := operationid.AuthorizationFenceFingerprint(
		platformv2.OperationKind_OPERATION_KIND_APPLY_AUTHORIZATION_FENCE.String(),
		input.BindingRef,
		input.Consumer,
		input.Generation,
		input.MinimumGrantVersion,
		0,
		0,
		0,
	)
	operationID := operationid.DeterministicID(operationFingerprint)
	operation := &platformv2.OperationRef{
		OperationId:        operationID,
		Kind:               platformv2.OperationKind_OPERATION_KIND_APPLY_AUTHORIZATION_FENCE,
		BindingRef:         input.BindingRef,
		PreGeneration:      input.Generation,
		TargetGeneration:   input.Generation,
		RequestFingerprint: operationFingerprint,
	}
	var owner model.User
	if err := s.db.Select("user_ref", "owner_epoch").First(&owner, input.OwnerUserID).Error; err != nil {
		return err
	}
	claims := buildBindingScopedTicketClaims(actorType, actorID, input.BindingRef, input.Platform, input.PlatformServiceKey, "", []string{platformaction.MihomoAuthorizationFenceApply})
	claims.OwnerUserRef = owner.UserRef
	claims.OwnerEpoch = owner.OwnerEpoch
	claims.CredentialGeneration = input.Generation
	claims.OperationID = operationID
	claims.AllowedActions = claims.Scopes
	signed, _, err := s.ticketSigner.Issue(serviceticket.TypeControl, ticketSubject(actorType, actorID, owner.UserRef), platformRow.ServiceAudience, claims)
	if err != nil {
		return err
	}

	dial := s.dial
	if dial == nil {
		return platformtransport.ErrControlTransportNotConfigured
	}

	conn, err := dial(ctx, platformRow.ControlEndpoint)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPlatformServiceUnavailable, err)
	}
	defer conn.Close()

	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	callCtx = clientauth.WithServiceTicket(callCtx, signed)

	resp, err := platformv2.NewPlatformControlServiceClient(conn).ApplyAuthorizationFence(callCtx, &platformv2.ApplyAuthorizationFenceRequest{
		Operation:           operation,
		ConsumerPrincipal:   input.Consumer,
		MinimumGrantVersion: input.MinimumGrantVersion,
	})
	if err != nil && isPlatformUnavailableRPCError(err) {
		return fmt.Errorf("%w: %v", ErrPlatformServiceUnavailable, err)
	}
	if err != nil {
		return err
	}
	if resp == nil || resp.GetResult().GetState() != platformv2.OperationState_OPERATION_STATE_SUCCEEDED || !proto.Equal(resp.GetResult().GetOperation(), operation) {
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

func ticketSubject(actorType, actorID, ownerUserRef string) string {
	if actorType == "consumer" {
		return "consumer:" + actorID
	}
	if actorType == "system" {
		return "system:account-center"
	}
	return "user:" + ownerUserRef
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

func (s *PlatformService) ConfigureTransport(dial func(context.Context, string) (*grpc.ClientConn, error)) error {
	if dial == nil {
		return platformtransport.ErrControlTransportNotConfigured
	}
	s.dial = dialFunc(dial)
	s.genericSummaryProxy = NewGRPCGenericSummaryProxy(dial)
	s.healthChecker = newGRPCHealthChecker(2*time.Second, dial)
	return nil
}

func (s *PlatformService) ControlDialer() func(context.Context, string) (*grpc.ClientConn, error) {
	if s == nil || s.dial == nil {
		return nil
	}
	return s.dial
}

func (s *PlatformService) SetHealthChecker(checker platformHealthChecker) {
	s.healthChecker = checker
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
	return s.genericSummaryProxy.GetCredentialSummary(ctx, platformRow.ControlEndpoint, ticket, binding.BindingRef, platformAccountID)
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
