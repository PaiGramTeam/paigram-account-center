package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	pb "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/account/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"

	"paigram/internal/grpc/interceptor"
	"paigram/internal/model"
	serviceaudit "paigram/internal/service/audit"
	"paigram/internal/service/botaccess"
	"paigram/internal/service/credentials"
)

const (
	botAccessScopeRead        = "bot.access.read"
	botAccessScopeIssueTicket = "bot.access.issue_ticket"
)

// Bot is the minimal identity record derived from the authenticated client ID.
// It remains structured because audit records consume bot metadata.
type Bot struct {
	Id string
}

// BotAccessService exposes bot binding operations over generated gRPC bindings.
type BotAccessService struct {
	pb.UnimplementedBotAccessServiceServer

	bindingAccessService *botaccess.BindingAccessService
	ticketService        *botaccess.TicketService
	db                   *gorm.DB
	runtimeRouteLookup   func(string) (model.PlatformService, error)
}

func NewBotAccessService(bindingAccessService *botaccess.BindingAccessService, ticketService *botaccess.TicketService, db *gorm.DB) *BotAccessService {
	return &BotAccessService{
		bindingAccessService: bindingAccessService,
		ticketService:        ticketService,
		db:                   db,
		runtimeRouteLookup: func(serviceKey string) (model.PlatformService, error) {
			var platform model.PlatformService
			err := db.Where("service_key = ? AND enabled = ?", serviceKey, true).First(&platform).Error
			return platform, err
		},
	}
}

func (s *BotAccessService) ResolveBotUser(ctx context.Context, req *pb.ResolveBotUserRequest) (*pb.ResolveBotUserResponse, error) {
	caller, err := botAccessCallerFromContext(ctx, botAccessScopeRead)
	if err != nil {
		return nil, err
	}
	if req.GetExternalUserId() == "" {
		return nil, status.Error(codes.InvalidArgument, "external_user_id is required")
	}

	identity, err := s.bindingAccessService.ResolveBotUser(caller.bot.Id, req.GetExternalUserId())
	if err != nil {
		return nil, mapBotAccessError("resolve bot user", err)
	}

	return &pb.ResolveBotUserResponse{
		UserId:           identity.UserID,
		BotId:            identity.BotID,
		ExternalUserId:   identity.ExternalUserID,
		ExternalUsername: nullStringValue(identity.ExternalUsername),
	}, nil
}

func (s *BotAccessService) ListAccessibleBindings(ctx context.Context, req *pb.ListAccessibleBindingsRequest) (*pb.ListAccessibleBindingsResponse, error) {
	caller, err := botAccessCallerFromContext(ctx, botAccessScopeRead)
	if err != nil {
		return nil, err
	}
	if req.GetExternalUserId() == "" {
		return nil, status.Error(codes.InvalidArgument, "external_user_id is required")
	}

	bindings, err := s.bindingAccessService.ListAccessibleBindingsForConsumer(caller.bot.Id, caller.consumer, req.GetExternalUserId(), req.GetPlatform())
	if err != nil {
		return nil, mapBotAccessError("list accessible bindings", err)
	}

	items := make([]*pb.PlatformAccountBinding, 0, len(bindings))
	for _, binding := range bindings {
		items = append(items, platformBindingToProto(binding))
	}

	return &pb.ListAccessibleBindingsResponse{Bindings: items}, nil
}

func (s *BotAccessService) GetPlatformRuntimeRoute(ctx context.Context, req *pb.GetPlatformRuntimeRouteRequest) (*pb.GetPlatformRuntimeRouteResponse, error) {
	if _, err := botAccessCallerFromContext(ctx, botAccessScopeRead); err != nil {
		return nil, err
	}
	serviceKey := strings.TrimSpace(req.GetPlatformServiceKey())
	if serviceKey == "" {
		return nil, status.Error(codes.InvalidArgument, "platform_service_key is required")
	}

	platform, err := s.runtimeRouteLookup(serviceKey)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "platform runtime route not found")
		}
		return nil, status.Error(codes.Internal, "resolve platform runtime route")
	}
	if strings.TrimSpace(platform.RuntimeEndpoint) == "" || strings.TrimSpace(platform.RuntimeServerName) == "" || strings.TrimSpace(platform.ServiceAudience) == "" {
		return nil, status.Error(codes.FailedPrecondition, "platform runtime route is incomplete")
	}
	supportedActions := []string{}
	if err := json.Unmarshal([]byte(platform.SupportedActionsJSON), &supportedActions); err != nil {
		return nil, status.Error(codes.Internal, "decode platform supported actions")
	}

	return &pb.GetPlatformRuntimeRouteResponse{
		PlatformKey:        platform.PlatformKey,
		PlatformServiceKey: platform.ServiceKey,
		RuntimeEndpoint:    platform.RuntimeEndpoint,
		RuntimeServerName:  platform.RuntimeServerName,
		ServiceAudience:    platform.ServiceAudience,
		SupportedActions:   supportedActions,
	}, nil
}

func (s *BotAccessService) IssueServiceTicket(ctx context.Context, req *pb.IssueServiceTicketRequest) (*pb.IssueServiceTicketResponse, error) {
	caller, err := botAccessCallerFromContext(ctx, botAccessScopeIssueTicket)
	if err != nil {
		return nil, err
	}
	if req.GetExternalUserId() == "" || req.GetBindingRef() == "" || req.GetRequestedAction() == "" {
		return nil, status.Error(codes.InvalidArgument, "external_user_id, binding_ref, and requested_action are required")
	}

	identity, binding, grant, err := s.bindingAccessService.GetGrantedBindingByRefForConsumer(caller.bot.Id, caller.consumer, req.GetExternalUserId(), req.GetBindingRef(), req.GetProfileRef())
	if err != nil {
		s.recordTicketAudit(ctx, caller.bot, nil, req, "ticket_reject", "failure", reasonCodeFromBotAccessErr(err), nil)
		return nil, mapBotAccessError("get granted binding", err)
	}
	audience, err := s.serviceAudienceForBinding(binding)
	if err != nil {
		s.recordTicketAudit(ctx, caller.bot, binding, req, "ticket_reject", "failure", "platform_service_unavailable", nil)
		return nil, mapBotAccessError("resolve platform service audience", err)
	}
	grantedActions := botaccess.GrantActions(*grant)
	if !slices.Contains(grantedActions, req.GetRequestedAction()) {
		err = botaccess.ErrScopeNotGranted
		s.recordTicketAudit(ctx, caller.bot, binding, req, "ticket_reject", "failure", reasonCodeFromBotAccessErr(err), nil)
		return nil, mapBotAccessError("validate requested scopes", err)
	}

	authorization, err := s.delegationAuthorization(identity, caller.consumer)
	if err != nil {
		s.recordTicketAudit(ctx, caller.bot, binding, req, "ticket_reject", "failure", "authorization_state_unavailable", nil)
		return nil, mapBotAccessError("resolve delegation authorization", err)
	}
	ticket, expiresAt, err := s.ticketService.Issue(caller.bot.Id, grant.Consumer, binding, req.GetRequestedAction(), audience, req.GetProfileRef(), grant.TicketVersion, authorization)
	if err != nil {
		s.recordTicketAudit(ctx, caller.bot, binding, req, "ticket_reject", "failure", reasonCodeFromBotAccessErr(err), map[string]any{"consumer": grant.Consumer})
		return nil, mapBotAccessError("issue service ticket", err)
	}
	s.recordTicketAudit(ctx, caller.bot, binding, req, "ticket_issue", "success", "", map[string]any{"consumer": grant.Consumer, "action": req.GetRequestedAction()})

	return &pb.IssueServiceTicketResponse{
		Ticket:    ticket,
		Audience:  audience,
		ExpiresAt: timestamppb.New(expiresAt),
		Binding:   platformBindingToProto(*binding),
	}, nil
}

func (s *BotAccessService) delegationAuthorization(identity *model.BotIdentity, consumer string) (botaccess.DelegationAuthorization, error) {
	if s == nil || s.db == nil || identity == nil {
		return botaccess.DelegationAuthorization{}, botaccess.ErrInvalidTicketConfig
	}
	var owner model.User
	if err := s.db.Select("user_ref", "owner_epoch").First(&owner, identity.UserID).Error; err != nil {
		return botaccess.DelegationAuthorization{}, err
	}
	var credential model.ServiceCredential
	if err := s.db.Select("consumer_epoch").Where("client_id = ?", consumer).First(&credential).Error; err != nil {
		return botaccess.DelegationAuthorization{}, err
	}
	return botaccess.DelegationAuthorization{
		OwnerUserRef:     owner.UserRef,
		EntryIdentityRef: identity.EntryIdentityRef,
		OwnerEpoch:       owner.OwnerEpoch,
		ConsumerEpoch:    credential.ConsumerEpoch,
		EntryEpoch:       identity.EntryEpoch,
	}, nil
}

func (s *BotAccessService) serviceAudienceForBinding(binding *model.PlatformAccountBinding) (string, error) {
	if s == nil || s.db == nil || binding == nil {
		return "", botaccess.ErrPlatformServiceNotEnabled
	}
	var platform model.PlatformService
	err := s.db.Where(
		"platform_key = ? AND service_key = ? AND enabled = ?",
		binding.Platform,
		binding.PlatformServiceKey,
		true,
	).First(&platform).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", botaccess.ErrPlatformServiceNotEnabled
		}
		return "", err
	}
	if strings.TrimSpace(platform.ServiceAudience) == "" {
		return "", botaccess.ErrPlatformServiceNotEnabled
	}
	return platform.ServiceAudience, nil
}

// botAccessCaller is the (bot, consumer) pair derived from the request's
// OAuth access token. The consumer is the credential client_id while bot_id
// identifies the logical Bot shared by one or more service credentials.
type botAccessCaller struct {
	bot      *Bot
	consumer string
}

// botAccessCallerFromContext extracts the validated AccessClaims, enforces
// the required scope, and projects the explicit (bot_id, client_id) mapping.
func botAccessCallerFromContext(ctx context.Context, requiredScope string) (*botAccessCaller, error) {
	claims, ok := interceptor.CredentialClaimsFromContext(ctx)
	if !ok || claims == nil {
		return nil, status.Error(codes.Unauthenticated, "credential claims missing")
	}
	if strings.TrimSpace(claims.ClientID) == "" || strings.TrimSpace(claims.BotID) == "" {
		return nil, status.Error(codes.Unauthenticated, "credential identity mapping missing")
	}
	if !credentials.HasScope(claims, requiredScope) {
		return nil, status.Error(codes.PermissionDenied, "required bot access scope missing")
	}
	return &botAccessCaller{
		bot:      &Bot{Id: claims.BotID},
		consumer: claims.ClientID,
	}, nil
}

// botFromContext returns the synthetic Bot value that bot route handlers
// share with this package. The context key matches what
// botFromCredentialClaims injects in bot_route_service.go.
func botFromContext(ctx context.Context) (*Bot, error) {
	claims, ok := interceptor.CredentialClaimsFromContext(ctx)
	if !ok || claims == nil {
		return nil, status.Error(codes.Unauthenticated, "credential claims missing")
	}
	if strings.TrimSpace(claims.BotID) == "" {
		return nil, status.Error(codes.Unauthenticated, "credential bot_id missing")
	}
	return &Bot{Id: claims.BotID}, nil
}

func platformBindingToProto(binding model.PlatformAccountBinding) *pb.PlatformAccountBinding {
	status := pb.PlatformAccountStatus_PLATFORM_ACCOUNT_STATUS_UNSPECIFIED
	switch binding.Status {
	case model.PlatformAccountBindingStatusActive:
		status = pb.PlatformAccountStatus_PLATFORM_ACCOUNT_STATUS_ACTIVE
	case model.PlatformAccountBindingStatusDeleted, model.PlatformAccountBindingStatusDeleting:
		status = pb.PlatformAccountStatus_PLATFORM_ACCOUNT_STATUS_REVOKED
	default:
		status = pb.PlatformAccountStatus_PLATFORM_ACCOUNT_STATUS_INACTIVE
	}

	return &pb.PlatformAccountBinding{
		BindingRef:         binding.BindingRef,
		Platform:           binding.Platform,
		PlatformServiceKey: binding.PlatformServiceKey,
		AccountKey:         nullStringValue(binding.ExternalAccountKey),
		DisplayName:        binding.DisplayName,
		Status:             status,
		Generation:         binding.Generation,
		CreatedAt:          timestamppb.New(binding.CreatedAt),
		UpdatedAt:          timestamppb.New(binding.UpdatedAt),
	}
}

func nullStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}

	return value.String
}

func mapBotAccessError(operation string, err error) error {
	switch {
	case errors.Is(err, botaccess.ErrBotIdentityNotFound):
		return status.Error(codes.NotFound, "bot identity not found")
	case errors.Is(err, botaccess.ErrPlatformAccountMissing):
		return status.Error(codes.NotFound, "platform account binding not found")
	case errors.Is(err, botaccess.ErrConsumerGrantNotFound):
		return status.Error(codes.PermissionDenied, "consumer grant required for binding")
	case errors.Is(err, botaccess.ErrConsumerGrantRevoked):
		return status.Error(codes.PermissionDenied, "consumer grant revoked for binding")
	case errors.Is(err, botaccess.ErrConsumerNotSupported):
		return status.Error(codes.InvalidArgument, "consumer is not supported")
	case errors.Is(err, botaccess.ErrScopeNotGranted):
		return status.Error(codes.PermissionDenied, "requested scope is not granted")
	case errors.Is(err, botaccess.ErrPlatformServiceNotEnabled):
		return status.Error(codes.InvalidArgument, "platform service is not enabled for platform")
	case errors.Is(err, botaccess.ErrInactiveBinding):
		return status.Error(codes.FailedPrecondition, "platform account binding is not active")
	case errors.Is(err, botaccess.ErrInvalidTicketConfig):
		return status.Error(codes.Unavailable, "invalid service ticket config")
	case errors.Is(err, botaccess.ErrSigningKeyUnavailable):
		return status.Error(codes.Unavailable, "service ticket signing key unavailable")
	default:
		return status.Errorf(codes.Internal, "%s: %v", operation, err)
	}
}

func (s *BotAccessService) recordTicketAudit(ctx context.Context, bot *Bot, binding *model.PlatformAccountBinding, req *pb.IssueServiceTicketRequest, action, result, reasonCode string, metadata map[string]any) {
	if s == nil || s.db == nil || bot == nil || req == nil {
		return
	}
	var bindingID *uint64
	var ownerUserID *uint64
	targetID := req.GetBindingRef()
	if binding != nil {
		bindingID = &binding.ID
		targetID = binding.BindingRef
		ownerUserID = &binding.OwnerUserID
	}
	writeMetadata := map[string]any{"bot_id": bot.Id, "external_user_id": req.GetExternalUserId()}
	for key, value := range metadata {
		writeMetadata[key] = value
	}
	_ = serviceaudit.Record(ctx, s.db, serviceaudit.WriteInput{
		Category:    "bot_access",
		ActorType:   "consumer",
		Action:      action,
		TargetType:  "binding",
		TargetID:    targetID,
		BindingID:   bindingID,
		OwnerUserID: ownerUserID,
		Result:      result,
		ReasonCode:  reasonCode,
		Metadata:    writeMetadata,
	})
}

func reasonCodeFromBotAccessErr(err error) string {
	switch {
	case errors.Is(err, botaccess.ErrBotIdentityNotFound):
		return "bot_identity_not_found"
	case errors.Is(err, botaccess.ErrPlatformAccountMissing):
		return "platform_account_missing"
	case errors.Is(err, botaccess.ErrConsumerGrantNotFound):
		return "consumer_grant_not_found"
	case errors.Is(err, botaccess.ErrConsumerGrantRevoked):
		return "consumer_grant_revoked"
	case errors.Is(err, botaccess.ErrConsumerNotSupported):
		return "consumer_not_supported"
	case errors.Is(err, botaccess.ErrScopeNotGranted):
		return "scope_not_granted"
	case errors.Is(err, botaccess.ErrInactiveBinding):
		return "inactive_binding"
	case errors.Is(err, botaccess.ErrInvalidTicketConfig):
		return "invalid_ticket_config"
	case errors.Is(err, botaccess.ErrSigningKeyUnavailable):
		return "signing_key_unavailable"
	default:
		return "internal_error"
	}
}
