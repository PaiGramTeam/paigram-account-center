package service

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1 "platform-mihomo-service/api/mihomo/v1"
	"platform-mihomo-service/internal/biz"
	"platform-mihomo-service/internal/data"
	"platform-mihomo-service/internal/usecase"
)

const serviceTicketAudience = "platform-mihomo-service"

type MihomoAccountService struct {
	v1.UnimplementedMihomoAccountServiceServer

	ticketVerifier *data.TicketVerifier
	bindUC         *usecase.BindUsecase
	statusUC       *usecase.StatusUsecase
	profileUC      *usecase.ProfileUsecase
	authkeyUC      *usecase.AuthkeyUsecase
	managementUC   *usecase.ManagementUsecase
}

func NewMihomoAccountService(
	ticketVerifier *data.TicketVerifier,
	bindUC *usecase.BindUsecase,
	statusUC *usecase.StatusUsecase,
	profileUC *usecase.ProfileUsecase,
	authkeyUC *usecase.AuthkeyUsecase,
	managementUC *usecase.ManagementUsecase,
) *MihomoAccountService {
	return &MihomoAccountService{
		ticketVerifier: ticketVerifier,
		bindUC:         bindUC,
		statusUC:       statusUC,
		profileUC:      profileUC,
		authkeyUC:      authkeyUC,
		managementUC:   managementUC,
	}
}

func (s *MihomoAccountService) BindCredential(ctx context.Context, req *v1.BindCredentialRequest) (*v1.BindCredentialResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	claims, err := s.verifyServiceTicket(ctx, req.GetServiceTicket())
	if err != nil {
		return nil, err
	}
	guard, err := scopedGuard(claims, usecase.ActionCredentialBind)
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	if err := guard.RequireBindingWide(); err != nil {
		return nil, mapUsecaseError(err)
	}

	output, err := s.bindUC.BindCredential(ctx, toBindCredentialInput(req, claims))
	if err != nil {
		return nil, mapUsecaseError(err)
	}

	return toBindCredentialResponse(output), nil
}

func (s *MihomoAccountService) GetCredentialStatus(ctx context.Context, req *v1.GetCredentialStatusRequest) (*v1.GetCredentialStatusResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	claims, err := s.verifyServiceTicket(ctx, req.GetServiceTicket())
	if err != nil {
		return nil, err
	}
	guard, err := scopedGuardForPlatformAccount(claims, req.GetPlatformAccountId(), usecase.ActionStatusRead)
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	if err := guard.RequireBindingWide(); err != nil {
		return nil, mapUsecaseError(err)
	}

	output, err := s.statusUC.GetCredentialStatus(ctx, req.GetPlatformAccountId())
	if err != nil {
		return nil, mapUsecaseError(err)
	}

	return &v1.GetCredentialStatusResponse{
		Status:          output.Status,
		LastValidatedAt: toTimestamp(output.LastValidatedAt),
	}, nil
}

func (s *MihomoAccountService) ValidateCredential(ctx context.Context, req *v1.ValidateCredentialRequest) (*v1.ValidateCredentialResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	claims, err := s.verifyServiceTicket(ctx, req.GetServiceTicket())
	if err != nil {
		return nil, err
	}
	guard, err := scopedGuardForPlatformAccount(claims, req.GetPlatformAccountId(), usecase.ActionStatusRead)
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	if err := guard.RequireBindingWide(); err != nil {
		return nil, mapUsecaseError(err)
	}

	output, err := s.statusUC.ValidateCredential(ctx, req.GetPlatformAccountId())
	if err != nil {
		return nil, mapUsecaseError(err)
	}

	return &v1.ValidateCredentialResponse{Status: output.Status, ErrorCode: output.ErrorCode}, nil
}

func (s *MihomoAccountService) RefreshCredential(ctx context.Context, req *v1.RefreshCredentialRequest) (*v1.RefreshCredentialResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	claims, err := s.verifyServiceTicket(ctx, req.GetServiceTicket())
	if err != nil {
		return nil, err
	}
	guard, err := scopedGuardForPlatformAccount(claims, req.GetPlatformAccountId(), usecase.ActionCredentialRefresh)
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	if err := guard.RequireBindingWide(); err != nil {
		return nil, mapUsecaseError(err)
	}

	output, err := s.statusUC.RefreshCredential(ctx, req.GetPlatformAccountId())
	if err != nil {
		return nil, mapUsecaseError(err)
	}

	return &v1.RefreshCredentialResponse{Status: output.Status, RefreshedAt: toTimestamp(output.RefreshedAt)}, nil
}

func (s *MihomoAccountService) ListProfiles(ctx context.Context, req *v1.ListProfilesRequest) (*v1.ListProfilesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	claims, err := s.verifyServiceTicket(ctx, req.GetServiceTicket())
	if err != nil {
		return nil, err
	}

	guard, err := scopedGuardForPlatformAccount(claims, req.GetPlatformAccountId(), usecase.ActionProfileRead)
	if err != nil {
		return nil, mapUsecaseError(err)
	}

	profiles, err := s.profileUC.ListProfilesWithScope(ctx, guard, req.GetPlatformAccountId())
	if err != nil {
		return nil, mapUsecaseError(err)
	}

	return &v1.ListProfilesResponse{Profiles: profiles}, nil
}

func (s *MihomoAccountService) GetPrimaryProfile(ctx context.Context, req *v1.GetPrimaryProfileRequest) (*v1.GetPrimaryProfileResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	claims, err := s.verifyServiceTicket(ctx, req.GetServiceTicket())
	if err != nil {
		return nil, err
	}

	guard, err := scopedGuardForPlatformAccount(claims, req.GetPlatformAccountId(), usecase.ActionProfileRead)
	if err != nil {
		return nil, mapUsecaseError(err)
	}

	profile, err := s.profileUC.GetPrimaryProfileWithScope(ctx, guard, req.GetPlatformAccountId())
	if err != nil {
		return nil, mapUsecaseError(err)
	}

	return &v1.GetPrimaryProfileResponse{Profile: profile}, nil
}

func (s *MihomoAccountService) GetAuthKey(ctx context.Context, req *v1.GetAuthKeyRequest) (*v1.GetAuthKeyResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	claims, err := s.verifyServiceTicket(ctx, req.GetServiceTicket())
	if err != nil {
		return nil, err
	}

	guard, err := scopedGuardForPlatformAccount(claims, req.GetPlatformAccountId(), usecase.ActionAuthKeyIssue)
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	if err := s.profileUC.RequireProfileAccessByPlayerID(ctx, guard, req.GetPlatformAccountId(), req.GetPlayerId()); err != nil {
		return nil, mapUsecaseError(err)
	}
	output, err := s.authkeyUC.GetAuthKey(ctx, req.GetPlatformAccountId(), req.GetPlayerId())
	if err != nil {
		return nil, mapUsecaseError(err)
	}

	return &v1.GetAuthKeyResponse{Authkey: output.AuthKey, ExpiresAt: toTimestamp(&output.ExpiresAt)}, nil
}

func (s *MihomoAccountService) GetCredentialSummary(ctx context.Context, req *v1.GetCredentialSummaryRequest) (*v1.GetCredentialSummaryResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	claims, err := s.verifyServiceTicket(ctx, req.GetServiceTicket())
	if err != nil {
		return nil, err
	}

	guard, err := scopedGuard(claims, usecase.ActionCredentialRead)
	if err != nil {
		return nil, mapUsecaseError(err)
	}

	output, err := s.managementUC.GetCredentialSummaryWithScope(ctx, guard, req.GetPlatformAccountId())
	if err != nil {
		return nil, mapUsecaseError(err)
	}

	return toCredentialSummaryResponse(output), nil
}

func (s *MihomoAccountService) UpdateCredential(ctx context.Context, req *v1.UpdateCredentialRequest) (*v1.UpdateCredentialResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	claims, err := s.verifyServiceTicket(ctx, req.GetServiceTicket())
	if err != nil {
		return nil, err
	}
	guard, err := scopedGuard(claims, usecase.ActionCredentialUpdate)
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	if req.GetDevice() == nil {
		return nil, status.Error(codes.InvalidArgument, "device is required")
	}
	device := req.GetDevice()

	input := usecase.UpdateCredentialInput{
		PlatformAccountID: req.GetPlatformAccountId(),
		BindCredentialInput: usecase.BindCredentialInput{
			BindingID:        claims.BindingID,
			CookieBundleJSON: req.GetCookieBundleJson(),
			RegionHint:       req.GetRegionHint(),
		},
	}
	if device != nil {
		input.DeviceID = device.GetDeviceId()
		input.DeviceFP = device.GetDeviceFp()
		input.DeviceName = device.GetDeviceName()
	}

	output, err := s.managementUC.UpdateCredentialWithScope(ctx, guard, input)
	if err != nil {
		return nil, mapUsecaseError(err)
	}

	return &v1.UpdateCredentialResponse{Summary: toCredentialSummary(output)}, nil
}

func (s *MihomoAccountService) DeleteCredential(ctx context.Context, req *v1.DeleteCredentialRequest) (*v1.DeleteCredentialResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	claims, err := s.verifyServiceTicket(ctx, req.GetServiceTicket())
	if err != nil {
		return nil, err
	}
	guard, err := scopedGuard(claims, usecase.ActionCredentialDelete)
	if err != nil {
		return nil, mapUsecaseError(err)
	}

	if err := s.managementUC.DeleteCredentialWithScope(ctx, guard, req.GetPlatformAccountId()); err != nil {
		return nil, mapUsecaseError(err)
	}

	return &v1.DeleteCredentialResponse{Success: true}, nil
}

func (s *MihomoAccountService) UpsertDevice(ctx context.Context, req *v1.UpsertDeviceRequest) (*v1.UpsertDeviceResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	claims, err := s.verifyServiceTicket(ctx, req.GetServiceTicket())
	if err != nil {
		return nil, err
	}
	guard, err := scopedGuardForPlatformAccount(claims, req.GetPlatformAccountId(), usecase.ActionDeviceUpdate)
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	if err := guard.RequireBindingWide(); err != nil {
		return nil, mapUsecaseError(err)
	}
	device := req.GetDevice()
	if device == nil {
		return nil, status.Error(codes.InvalidArgument, "device is required")
	}
	if err := s.bindUC.UpsertDevice(ctx, req.GetPlatformAccountId(), device.GetDeviceId(), device.GetDeviceFp(), device.GetDeviceName()); err != nil {
		return nil, mapUsecaseError(err)
	}
	return &v1.UpsertDeviceResponse{Success: true}, nil
}

func (s *MihomoAccountService) ConfirmPrimaryProfile(ctx context.Context, req *v1.ConfirmPrimaryProfileRequest) (*v1.ConfirmPrimaryProfileResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	claims, err := s.verifyServiceTicket(ctx, req.GetServiceTicket())
	if err != nil {
		return nil, err
	}
	guard, err := scopedGuardForPlatformAccount(claims, req.GetPlatformAccountId(), usecase.ActionProfileWrite)
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	if err := guard.RequireBindingWide(); err != nil {
		return nil, mapUsecaseError(err)
	}
	profile, err := s.profileUC.ConfirmPrimaryProfileWithScope(ctx, guard, req.GetPlatformAccountId(), req.GetPlayerId())
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	return &v1.ConfirmPrimaryProfileResponse{Profile: profile}, nil
}

func (s *MihomoAccountService) verifyServiceTicket(ctx context.Context, raw string) (*biz.ServiceTicketClaims, error) {
	claims, err := s.ticketVerifier.VerifyContext(ctx, raw, serviceTicketAudience)
	if err != nil {
		return nil, mapTicketVerificationError(err)
	}

	return claims, nil
}

func mapTicketVerificationError(err error) error {
	if errors.Is(err, data.ErrGrantVersionRevoked) {
		return status.Error(codes.PermissionDenied, err.Error())
	}
	return status.Error(codes.Unauthenticated, "invalid service ticket")
}

func mapUsecaseError(err error) error {
	if errors.Is(err, usecase.ErrCredentialNotFound) {
		return status.Error(codes.NotFound, err.Error())
	}
	if errors.Is(err, usecase.ErrProfileNotFound) {
		return status.Error(codes.NotFound, err.Error())
	}
	if errors.Is(err, usecase.ErrPlatformAccountMismatch) {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	if errors.Is(err, usecase.ErrActionScopeDenied) || errors.Is(err, usecase.ErrBindingScopeDenied) || errors.Is(err, usecase.ErrProfileScopeDenied) {
		return status.Error(codes.PermissionDenied, err.Error())
	}

	return status.Error(codes.Internal, err.Error())
}

func scopedGuardForPlatformAccount(claims *biz.ServiceTicketClaims, platformAccountID string, requiredActions ...string) (usecase.ScopeGuard, error) {
	guard, err := toScopeGuard(claims)
	if err != nil {
		return usecase.ScopeGuard{}, err
	}
	for _, action := range requiredActions {
		if err := guard.RequireAction(action); err != nil {
			return usecase.ScopeGuard{}, err
		}
	}
	if err := guard.RequirePlatformAccountID(platformAccountID); err != nil {
		return usecase.ScopeGuard{}, err
	}
	return guard, nil
}

func scopedGuard(claims *biz.ServiceTicketClaims, requiredActions ...string) (usecase.ScopeGuard, error) {
	guard, err := toScopeGuard(claims)
	if err != nil {
		return usecase.ScopeGuard{}, err
	}
	for _, action := range requiredActions {
		if err := guard.RequireAction(action); err != nil {
			return usecase.ScopeGuard{}, err
		}
	}
	return guard, nil
}
