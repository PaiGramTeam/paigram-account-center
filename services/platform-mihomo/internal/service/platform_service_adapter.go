package service

import (
	"context"
	"encoding/json"

	platformv1 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	mihomoapiv1 "platform-mihomo-service/api/mihomo/v1"
	"platform-mihomo-service/internal/data"
	"platform-mihomo-service/internal/usecase"
)

const consumerGrantInvalidateScope = "mihomo.consumer_grant.invalidate"

type grantInvalidationStore interface {
	Upsert(ctx context.Context, bindingID uint64, consumer string, minimumVersion uint64) error
}

type GenericPlatformService struct {
	platformv1.UnimplementedPlatformServiceServer

	ticketVerifier   *data.TicketVerifier
	bindUC           *usecase.BindUsecase
	statusUC         *usecase.StatusUsecase
	managementUC     *usecase.ManagementUsecase
	invalidationRepo grantInvalidationStore
}

func NewGenericPlatformService(ticketVerifier *data.TicketVerifier, bindUC *usecase.BindUsecase, statusUC *usecase.StatusUsecase, managementUC *usecase.ManagementUsecase, invalidationRepo grantInvalidationStore) *GenericPlatformService {
	return &GenericPlatformService{ticketVerifier: ticketVerifier, bindUC: bindUC, statusUC: statusUC, managementUC: managementUC, invalidationRepo: invalidationRepo}
}

func (s *GenericPlatformService) DescribePlatform(context.Context, *platformv1.DescribePlatformRequest) (*platformv1.DescribePlatformResponse, error) {
	credentialSchema, err := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"cookie_bundle": map[string]any{"type": "string"},
			"device_id":     map[string]any{"type": "string"},
			"device_fp":     map[string]any{"type": "string"},
			"device_name":   map[string]any{"type": "string"},
		},
		"required": []any{"cookie_bundle", "device_id", "device_fp"},
	})
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to build credential schema")
	}

	return &platformv1.DescribePlatformResponse{
		PlatformKey:      "mihomo",
		DisplayName:      "Mihomo",
		ServiceAudience:  serviceTicketAudience,
		SupportedActions: []string{usecase.ActionStatusRead, usecase.ActionProfileRead, usecase.ActionProfileWrite, usecase.ActionAuthKeyIssue, usecase.ActionCredentialRead, usecase.ActionCredentialBind, usecase.ActionCredentialUpdate, usecase.ActionCredentialRefresh, usecase.ActionCredentialDelete, usecase.ActionDeviceUpdate, consumerGrantInvalidateScope},
		CredentialSchema: credentialSchema,
		Version:          "v1",
	}, nil
}

func (s *GenericPlatformService) GetCredentialSummary(ctx context.Context, req *platformv1.GetCredentialSummaryRequest) (*platformv1.GetCredentialSummaryResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	claims, err := s.ticketVerifier.VerifyContext(ctx, req.GetServiceTicket(), serviceTicketAudience)
	if err != nil {
		return nil, mapTicketVerificationError(err)
	}
	guard, err := scopedGuardForPlatformAccount(claims, req.GetPlatformAccountId(), usecase.ActionCredentialRead)
	if err != nil {
		return nil, mapUsecaseError(err)
	}

	output, err := s.managementUC.GetCredentialSummaryWithScope(ctx, guard, req.GetPlatformAccountId())
	if err != nil {
		return nil, mapUsecaseError(err)
	}

	return toGenericCredentialSummary(output), nil
}

func (s *GenericPlatformService) PutCredential(ctx context.Context, req *platformv1.PutCredentialRequest) (*platformv1.PutCredentialResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	claims, err := s.ticketVerifier.VerifyContext(ctx, req.GetServiceTicket(), serviceTicketAudience)
	if err != nil {
		return nil, mapTicketVerificationError(err)
	}
	guard, err := scopedGuard(claims)
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	if err := guard.RequireBindingWide(); err != nil {
		return nil, mapUsecaseError(err)
	}

	payload, err := decodeGenericCredentialPayload(req.GetCredentialPayloadJson())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	bindInput := usecase.BindCredentialInput{
		BindingID:        claims.BindingID,
		CookieBundleJSON: payload.CookieBundle,
		DeviceID:         payload.DeviceID,
		DeviceFP:         payload.DeviceFP,
		DeviceName:       payload.DeviceName,
		RegionHint:       payload.RegionHint,
	}

	platformAccountID := req.GetPlatformAccountId()
	if platformAccountID != "" {
		if err := guard.RequireAction(usecase.ActionCredentialUpdate); err != nil {
			return nil, mapUsecaseError(err)
		}
		if err := guard.RequirePlatformAccountID(platformAccountID); err != nil {
			return nil, mapUsecaseError(err)
		}
		summary, err := s.managementUC.UpdateCredentialWithScope(ctx, guard, usecase.UpdateCredentialInput{
			PlatformAccountID:   platformAccountID,
			BindCredentialInput: bindInput,
		})
		if err != nil {
			return nil, mapUsecaseError(err)
		}
		return &platformv1.PutCredentialResponse{Summary: toGenericCredentialSummary(summary)}, nil
	}
	if err := guard.RequireAction(usecase.ActionCredentialBind); err != nil {
		return nil, mapUsecaseError(err)
	}

	bound, err := s.bindUC.BindCredential(ctx, bindInput)
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	summary, err := s.managementUC.GetCredentialSummaryWithScope(ctx, guard, bound.PlatformAccountID)
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	return &platformv1.PutCredentialResponse{Summary: toGenericCredentialSummary(summary)}, nil
}

func (s *GenericPlatformService) RefreshCredential(ctx context.Context, req *platformv1.RefreshCredentialRequest) (*platformv1.RefreshCredentialResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	claims, err := s.ticketVerifier.VerifyContext(ctx, req.GetServiceTicket(), serviceTicketAudience)
	if err != nil {
		return nil, mapTicketVerificationError(err)
	}
	guard, err := scopedGuard(claims, usecase.ActionCredentialRefresh)
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	platformAccountID := req.GetPlatformAccountId()
	if platformAccountID == "" {
		platformAccountID = claims.PlatformAccountID
	}
	if platformAccountID == "" {
		return nil, status.Error(codes.InvalidArgument, "platform_account_id is required")
	}
	if err := guard.RequirePlatformAccountID(platformAccountID); err != nil {
		return nil, mapUsecaseError(err)
	}
	if err := guard.RequireBindingWide(); err != nil {
		return nil, mapUsecaseError(err)
	}

	output, err := s.statusUC.RefreshCredential(ctx, platformAccountID)
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	return &platformv1.RefreshCredentialResponse{Status: toGenericCredentialStatus(output.Status), RefreshedAt: toTimestamp(output.RefreshedAt)}, nil
}

func (s *GenericPlatformService) DeleteCredential(ctx context.Context, req *platformv1.DeleteCredentialRequest) (*platformv1.DeleteCredentialResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	claims, err := s.ticketVerifier.VerifyContext(ctx, req.GetServiceTicket(), serviceTicketAudience)
	if err != nil {
		return nil, mapTicketVerificationError(err)
	}
	guard, err := scopedGuard(claims, usecase.ActionCredentialDelete)
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	platformAccountID := req.GetPlatformAccountId()
	if platformAccountID == "" {
		platformAccountID = claims.PlatformAccountID
	}
	if platformAccountID == "" {
		return nil, status.Error(codes.InvalidArgument, "platform_account_id is required")
	}
	if err := guard.RequirePlatformAccountID(platformAccountID); err != nil {
		return nil, mapUsecaseError(err)
	}
	if err := guard.RequireBindingWide(); err != nil {
		return nil, mapUsecaseError(err)
	}
	if err := s.managementUC.DeleteCredentialWithScope(ctx, guard, platformAccountID); err != nil {
		return nil, mapUsecaseError(err)
	}
	return &platformv1.DeleteCredentialResponse{Success: true}, nil
}

func (s *GenericPlatformService) InvalidateConsumerGrant(ctx context.Context, req *platformv1.InvalidateConsumerGrantRequest) (*platformv1.InvalidateConsumerGrantResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if req.GetBindingId() == 0 {
		return nil, status.Error(codes.InvalidArgument, "binding_id is required")
	}
	if req.GetConsumer() == "" {
		return nil, status.Error(codes.InvalidArgument, "consumer is required")
	}
	if req.GetMinimumGrantVersion() == 0 {
		return nil, status.Error(codes.InvalidArgument, "minimum_grant_version is required")
	}

	claims, err := s.ticketVerifier.VerifyContext(ctx, req.GetServiceTicket(), serviceTicketAudience)
	if err != nil {
		return nil, mapTicketVerificationError(err)
	}
	if claims.ActorType != "admin" && claims.ActorType != "user" {
		return nil, status.Error(codes.PermissionDenied, "only admin or user tickets can invalidate grants")
	}
	if claims.BindingID != req.GetBindingId() {
		return nil, status.Error(codes.PermissionDenied, "ticket binding_id does not match request")
	}
	guard, err := scopedGuard(claims, consumerGrantInvalidateScope)
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	if err := guard.RequireBindingWide(); err != nil {
		return nil, mapUsecaseError(err)
	}
	if s.invalidationRepo == nil {
		return nil, status.Error(codes.Internal, "grant invalidation repo is not configured")
	}
	if err := s.invalidationRepo.Upsert(ctx, req.GetBindingId(), req.GetConsumer(), req.GetMinimumGrantVersion()); err != nil {
		return nil, status.Error(codes.Internal, "failed to invalidate consumer grant")
	}

	return &platformv1.InvalidateConsumerGrantResponse{Success: true}, nil
}

type genericCredentialPayload struct {
	CookieBundle string `json:"cookie_bundle"`
	DeviceID     string `json:"device_id"`
	DeviceFP     string `json:"device_fp"`
	DeviceName   string `json:"device_name"`
	RegionHint   string `json:"region_hint"`
}

func decodeGenericCredentialPayload(raw string) (*genericCredentialPayload, error) {
	var payload genericCredentialPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func toGenericCredentialSummary(output *usecase.CredentialSummaryOutput) *platformv1.GetCredentialSummaryResponse {
	profiles := make([]*platformv1.ProfileSummary, 0, len(output.Profiles))
	for _, profile := range output.Profiles {
		profiles = append(profiles, &platformv1.ProfileSummary{
			Id:                profile.Id,
			PlatformAccountId: profile.PlatformAccountId,
			GameBiz:           profile.GameBiz,
			Region:            profile.Region,
			PlayerId:          profile.PlayerId,
			Nickname:          profile.Nickname,
			Level:             profile.Level,
			IsDefault:         profile.IsDefault,
		})
	}

	devices := make([]*platformv1.DeviceSummary, 0, len(output.Devices))
	for _, device := range output.Devices {
		devices = append(devices, &platformv1.DeviceSummary{
			DeviceId:   device.DeviceID,
			DeviceFp:   device.DeviceFP,
			DeviceName: derefString(device.DeviceName),
			IsValid:    device.IsValid,
			LastSeenAt: toTimestamp(device.LastSeenAt),
		})
	}

	return &platformv1.GetCredentialSummaryResponse{
		PlatformAccountId: output.PlatformAccountID,
		Status:            toGenericCredentialStatus(output.Status),
		LastValidatedAt:   toTimestamp(output.LastValidatedAt),
		LastRefreshedAt:   toTimestamp(output.LastRefreshedAt),
		Devices:           devices,
		Profiles:          profiles,
	}
}

func toGenericCredentialStatus(statusValue mihomoapiv1.CredentialStatus) platformv1.CredentialStatus {
	switch statusValue {
	case mihomoapiv1.CredentialStatus_CREDENTIAL_STATUS_ACTIVE:
		return platformv1.CredentialStatus_CREDENTIAL_STATUS_ACTIVE
	case mihomoapiv1.CredentialStatus_CREDENTIAL_STATUS_EXPIRED:
		return platformv1.CredentialStatus_CREDENTIAL_STATUS_EXPIRED
	case mihomoapiv1.CredentialStatus_CREDENTIAL_STATUS_INVALID:
		return platformv1.CredentialStatus_CREDENTIAL_STATUS_INVALID
	case mihomoapiv1.CredentialStatus_CREDENTIAL_STATUS_CHALLENGE_REQUIRED:
		return platformv1.CredentialStatus_CREDENTIAL_STATUS_CHALLENGE_REQUIRED
	default:
		return platformv1.CredentialStatus_CREDENTIAL_STATUS_UNSPECIFIED
	}
}
