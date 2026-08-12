package service

import (
	"context"
	"encoding/json"

	platformv1 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

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
	profileUC        *usecase.ProfileUsecase
	authkeyUC        *usecase.AuthkeyUsecase
	managementUC     *usecase.ManagementUsecase
	invalidationRepo grantInvalidationStore
}

func (s *GenericPlatformService) WithConsumerUsecases(profileUC *usecase.ProfileUsecase, authkeyUC *usecase.AuthkeyUsecase) *GenericPlatformService {
	s.profileUC = profileUC
	s.authkeyUC = authkeyUC
	return s
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
		SupportedActions: []string{usecase.ActionStatusRead, usecase.ActionCredentialValidate, usecase.ActionProfileRead, usecase.ActionProfileWrite, usecase.ActionAuthKeyIssue, usecase.ActionCredentialRead, usecase.ActionCredentialBind, usecase.ActionCredentialUpdate, usecase.ActionCredentialRefresh, usecase.ActionCredentialDelete, usecase.ActionDeviceUpdate, consumerGrantInvalidateScope},
		CredentialSchema: credentialSchema,
		Version:          "v1",
	}, nil
}

func (s *GenericPlatformService) GetCredentialSummary(ctx context.Context, req *platformv1.GetCredentialSummaryRequest) (*platformv1.GetCredentialSummaryResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	claims, err := serviceTicketClaims(ctx, s.ticketVerifier)
	if err != nil {
		return nil, err
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

func (s *GenericPlatformService) GetCredentialStatus(ctx context.Context, req *platformv1.GetCredentialStatusRequest) (*platformv1.GetCredentialStatusResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if _, err := s.authorizePlatformAccount(ctx, req.GetPlatformAccountId(), usecase.ActionStatusRead, true); err != nil {
		return nil, err
	}
	output, err := s.statusUC.GetCredentialStatus(ctx, req.GetPlatformAccountId())
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	return &platformv1.GetCredentialStatusResponse{
		Status:          toGenericCredentialStatus(output.Status),
		LastValidatedAt: toTimestamp(output.LastValidatedAt),
	}, nil
}

func (s *GenericPlatformService) ValidateCredential(ctx context.Context, req *platformv1.ValidateCredentialRequest) (*platformv1.ValidateCredentialResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if _, err := s.authorizePlatformAccount(ctx, req.GetPlatformAccountId(), usecase.ActionCredentialValidate, true); err != nil {
		return nil, err
	}
	output, err := s.statusUC.ValidateCredential(ctx, req.GetPlatformAccountId())
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	return &platformv1.ValidateCredentialResponse{
		Status:    toGenericCredentialStatus(output.Status),
		ErrorCode: output.ErrorCode,
	}, nil
}

func (s *GenericPlatformService) ListProfiles(ctx context.Context, req *platformv1.ListProfilesRequest) (*platformv1.ListProfilesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.profileUC == nil {
		return nil, status.Error(codes.FailedPrecondition, "profile service is not configured")
	}
	guard, err := s.authorizePlatformAccount(ctx, req.GetPlatformAccountId(), usecase.ActionProfileRead, false)
	if err != nil {
		return nil, err
	}
	profiles, err := s.profileUC.ListProfilesWithScope(ctx, guard, req.GetPlatformAccountId())
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	items := make([]*platformv1.ProfileSummary, 0, len(profiles))
	for _, profile := range profiles {
		items = append(items, toGenericProfileSummary(profile))
	}
	return &platformv1.ListProfilesResponse{Profiles: items}, nil
}

func (s *GenericPlatformService) GetPrimaryProfile(ctx context.Context, req *platformv1.GetPrimaryProfileRequest) (*platformv1.GetPrimaryProfileResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.profileUC == nil {
		return nil, status.Error(codes.FailedPrecondition, "profile service is not configured")
	}
	guard, err := s.authorizePlatformAccount(ctx, req.GetPlatformAccountId(), usecase.ActionProfileRead, false)
	if err != nil {
		return nil, err
	}
	profile, err := s.profileUC.GetPrimaryProfileWithScope(ctx, guard, req.GetPlatformAccountId())
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	return &platformv1.GetPrimaryProfileResponse{Profile: toGenericProfileSummary(profile)}, nil
}

func (s *GenericPlatformService) ConfirmPrimaryProfile(ctx context.Context, req *platformv1.ConfirmPrimaryProfileRequest) (*platformv1.ConfirmPrimaryProfileResponse, error) {
	if req == nil || req.GetPlayerId() == "" {
		return nil, status.Error(codes.InvalidArgument, "request and player_id are required")
	}
	if s.profileUC == nil {
		return nil, status.Error(codes.FailedPrecondition, "profile service is not configured")
	}
	guard, err := s.authorizePlatformAccount(ctx, req.GetPlatformAccountId(), usecase.ActionProfileWrite, true)
	if err != nil {
		return nil, err
	}
	profile, err := s.profileUC.ConfirmPrimaryProfileWithScope(ctx, guard, req.GetPlatformAccountId(), req.GetPlayerId())
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	return &platformv1.ConfirmPrimaryProfileResponse{Profile: toGenericProfileSummary(profile)}, nil
}

func (s *GenericPlatformService) GetAuthKey(ctx context.Context, req *platformv1.GetAuthKeyRequest) (*platformv1.GetAuthKeyResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.profileUC == nil || s.authkeyUC == nil {
		return nil, status.Error(codes.FailedPrecondition, "authkey service is not configured")
	}
	guard, err := s.authorizePlatformAccount(ctx, req.GetPlatformAccountId(), usecase.ActionAuthKeyIssue, false)
	if err != nil {
		return nil, err
	}
	if err := s.profileUC.RequireProfileAccessByPlayerID(ctx, guard, req.GetPlatformAccountId(), req.GetPlayerId()); err != nil {
		return nil, mapUsecaseError(err)
	}
	output, err := s.authkeyUC.GetAuthKey(ctx, req.GetPlatformAccountId(), req.GetPlayerId())
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	return &platformv1.GetAuthKeyResponse{Authkey: output.AuthKey, ExpiresAt: toTimestamp(&output.ExpiresAt)}, nil
}

func (s *GenericPlatformService) UpsertDevice(ctx context.Context, req *platformv1.UpsertDeviceRequest) (*platformv1.UpsertDeviceResponse, error) {
	if req == nil || req.GetDevice() == nil {
		return nil, status.Error(codes.InvalidArgument, "request and device are required")
	}
	if _, err := s.authorizePlatformAccount(ctx, req.GetPlatformAccountId(), usecase.ActionDeviceUpdate, true); err != nil {
		return nil, err
	}
	device := req.GetDevice()
	if err := s.bindUC.UpsertDevice(ctx, req.GetPlatformAccountId(), device.GetDeviceId(), device.GetDeviceFp(), device.GetDeviceName()); err != nil {
		return nil, mapUsecaseError(err)
	}
	return &platformv1.UpsertDeviceResponse{Success: true}, nil
}

func (s *GenericPlatformService) authorizePlatformAccount(ctx context.Context, platformAccountID, action string, bindingWide bool) (usecase.ScopeGuard, error) {
	if platformAccountID == "" {
		return usecase.ScopeGuard{}, status.Error(codes.InvalidArgument, "platform_account_id is required")
	}
	claims, err := serviceTicketClaims(ctx, s.ticketVerifier)
	if err != nil {
		return usecase.ScopeGuard{}, err
	}
	guard, err := scopedGuardForPlatformAccount(claims, platformAccountID, action)
	if err != nil {
		return usecase.ScopeGuard{}, mapUsecaseError(err)
	}
	if bindingWide {
		if err := guard.RequireBindingWide(); err != nil {
			return usecase.ScopeGuard{}, mapUsecaseError(err)
		}
	}
	return guard, nil
}

func (s *GenericPlatformService) RefreshCredential(ctx context.Context, req *platformv1.RefreshCredentialRequest) (*platformv1.RefreshCredentialResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	_, _, platformAccountID, err := s.authorizeExistingCredentialMutation(ctx, req.GetPlatformAccountId(), usecase.ActionCredentialRefresh)
	if err != nil {
		return nil, err
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

	_, guard, platformAccountID, err := s.authorizeExistingCredentialMutation(ctx, req.GetPlatformAccountId(), usecase.ActionCredentialDelete)
	if err != nil {
		return nil, err
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

	claims, err := serviceTicketClaims(ctx, s.ticketVerifier)
	if err != nil {
		return nil, err
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
		profiles = append(profiles, toGenericProfileSummary(profile))
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

func toGenericProfileSummary(profile *usecase.ProfileSummary) *platformv1.ProfileSummary {
	if profile == nil {
		return nil
	}
	return &platformv1.ProfileSummary{
		Id:                profile.ID,
		PlatformAccountId: profile.PlatformAccountID,
		GameBiz:           profile.GameBiz,
		Region:            profile.Region,
		PlayerId:          profile.PlayerID,
		Nickname:          profile.Nickname,
		Level:             profile.Level,
		IsDefault:         profile.IsDefault,
	}
}

func toGenericCredentialStatus(statusValue usecase.CredentialStatus) platformv1.CredentialStatus {
	switch statusValue {
	case usecase.CredentialStatusActive:
		return platformv1.CredentialStatus_CREDENTIAL_STATUS_ACTIVE
	case usecase.CredentialStatusExpired:
		return platformv1.CredentialStatus_CREDENTIAL_STATUS_EXPIRED
	case usecase.CredentialStatusInvalid:
		return platformv1.CredentialStatus_CREDENTIAL_STATUS_INVALID
	case usecase.CredentialStatusChallengeRequired:
		return platformv1.CredentialStatus_CREDENTIAL_STATUS_CHALLENGE_REQUIRED
	default:
		return platformv1.CredentialStatus_CREDENTIAL_STATUS_UNSPECIFIED
	}
}
