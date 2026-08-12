package service

import (
	"context"
	"errors"

	mihomov2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/mihomo/v2"
	platformv2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v2"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/platformaction"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	"platform-mihomo-service/internal/biz"
	"platform-mihomo-service/internal/data"
	"platform-mihomo-service/internal/usecase"
)

type MihomoRuntimeService struct {
	mihomov2.UnimplementedMihomoRuntimeServiceServer

	ticketVerifier *data.TicketVerifier
	statusUC       *usecase.StatusUsecase
	profileUC      *usecase.ProfileUsecase
	authkeyUC      *usecase.AuthkeyUsecase
	managementUC   *usecase.ManagementUsecase
	devices        biz.DeviceRepository
}

func NewMihomoRuntimeService(
	ticketVerifier *data.TicketVerifier,
	statusUC *usecase.StatusUsecase,
	profileUC *usecase.ProfileUsecase,
	authkeyUC *usecase.AuthkeyUsecase,
	managementUC *usecase.ManagementUsecase,
	devices biz.DeviceRepository,
) *MihomoRuntimeService {
	return &MihomoRuntimeService{
		ticketVerifier: ticketVerifier,
		statusUC:       statusUC,
		profileUC:      profileUC,
		authkeyUC:      authkeyUC,
		managementUC:   managementUC,
		devices:        devices,
	}
}

func (s *MihomoRuntimeService) DescribePlatform(context.Context, *mihomov2.DescribePlatformRequest) (*mihomov2.DescribePlatformResponse, error) {
	schema, err := structpb.NewStruct(map[string]any{
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
	actions := append(platformaction.MihomoDelegationActions(), platformaction.MihomoControlActions()...)
	return &mihomov2.DescribePlatformResponse{
		PlatformKey:      "mihomo",
		DisplayName:      "Mihomo",
		ServiceAudience:  serviceTicketAudience,
		SupportedActions: actions,
		CredentialSchema: schema,
		ContractVersion:  "v2",
	}, nil
}

func (s *MihomoRuntimeService) GetStatus(ctx context.Context, req *mihomov2.GetStatusRequest) (*mihomov2.GetStatusResponse, error) {
	resource, _, err := s.authorizeResource(ctx, req.GetResource(), usecase.ActionStatusRead, true)
	if err != nil {
		return nil, err
	}
	output, err := s.statusUC.GetCredentialStatus(ctx, resource.GetAccountKey())
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	return &mihomov2.GetStatusResponse{Status: toCredentialStatus(output.Status), LastValidatedAt: toTimestamp(output.LastValidatedAt)}, nil
}

func (s *MihomoRuntimeService) ValidateCredential(ctx context.Context, req *mihomov2.ValidateCredentialRequest) (*mihomov2.ValidateCredentialResponse, error) {
	resource, _, err := s.authorizeResource(ctx, req.GetResource(), usecase.ActionCredentialValidate, true)
	if err != nil {
		return nil, err
	}
	output, err := s.statusUC.ValidateCredential(ctx, resource.GetAccountKey())
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	return &mihomov2.ValidateCredentialResponse{Status: toCredentialStatus(output.Status), ReasonCode: output.ErrorCode}, nil
}

func (s *MihomoRuntimeService) ListProfiles(ctx context.Context, req *mihomov2.ListProfilesRequest) (*mihomov2.ListProfilesResponse, error) {
	resource, guard, err := s.authorizeResource(ctx, req.GetResource(), usecase.ActionProfileRead, false)
	if err != nil {
		return nil, err
	}
	profiles, err := s.profileUC.ListProfilesWithScope(ctx, guard, resource.GetAccountKey())
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	summary, err := s.managementUC.GetCredentialSummary(ctx, resource.GetAccountKey())
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	return &mihomov2.ListProfilesResponse{Snapshot: toProfileSnapshot(profiles, summary.ProfileSnapshotComplete, summary.ProfileRevision, summary.ProfileObservedRevision)}, nil
}

func (s *MihomoRuntimeService) GetPrimaryProfile(ctx context.Context, req *mihomov2.GetPrimaryProfileRequest) (*mihomov2.GetPrimaryProfileResponse, error) {
	resource, guard, err := s.authorizeResource(ctx, req.GetResource(), usecase.ActionProfileRead, false)
	if err != nil {
		return nil, err
	}
	profile, err := s.profileUC.GetPrimaryProfileWithScope(ctx, guard, resource.GetAccountKey())
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	return &mihomov2.GetPrimaryProfileResponse{Profile: toProfileSummary(profile)}, nil
}

func (s *MihomoRuntimeService) GetAuthKey(ctx context.Context, req *mihomov2.GetAuthKeyRequest) (*mihomov2.GetAuthKeyResponse, error) {
	resource, guard, err := s.authorizeResource(ctx, req.GetResource(), usecase.ActionAuthKeyIssue, false)
	if err != nil {
		return nil, err
	}
	profile, err := s.profileUC.GetProfileByRef(ctx, guard, resource.GetAccountKey(), req.GetProfileRef())
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	output, err := s.authkeyUC.GetAuthKey(ctx, resource.GetAccountKey(), profile.PlayerID)
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	authorizeDelivery := func(txCtx context.Context) error {
		claims, err := verifyIncomingServiceTicket(txCtx, s.ticketVerifier)
		if err != nil {
			return err
		}
		if err := requireDelegationTicket(claims); err != nil {
			return err
		}
		guard, err := scopedGuard(claims, usecase.ActionAuthKeyIssue)
		if err != nil {
			return mapUsecaseError(err)
		}
		if claims.BindingRef != resource.GetBindingRef() || claims.AccountKey != resource.GetAccountKey() {
			return status.Error(codes.PermissionDenied, "resource does not match ticket")
		}
		if err := guard.RequireProfile(resource.GetBindingRef(), req.GetProfileRef()); err != nil {
			return mapUsecaseError(err)
		}
		return nil
	}
	if err := s.authkeyUC.ConfirmDeliverable(ctx, resource.GetBindingRef(), authorizeDelivery); err != nil {
		if invalidateErr := s.authkeyUC.InvalidateBinding(ctx, resource.GetBindingRef()); invalidateErr != nil && !errors.Is(invalidateErr, biz.ErrArtifactRevocationPending) {
			return nil, mapUsecaseError(invalidateErr)
		}
		if status.Code(err) != codes.Unknown {
			return nil, err
		}
		return nil, mapUsecaseError(err)
	}
	return &mihomov2.GetAuthKeyResponse{Authkey: output.AuthKey, ExpiresAt: toTimestamp(&output.ExpiresAt)}, nil
}

func (s *MihomoRuntimeService) GetDevice(ctx context.Context, req *mihomov2.GetDeviceRequest) (*mihomov2.GetDeviceResponse, error) {
	resource, _, err := s.authorizeResource(ctx, req.GetResource(), usecase.ActionDeviceRead, true)
	if err != nil {
		return nil, err
	}
	if req.GetDeviceRef() == "" {
		return nil, status.Error(codes.InvalidArgument, "device_ref is required")
	}
	device, err := s.devices.GetByDeviceRef(ctx, resource.GetBindingRef(), req.GetDeviceRef())
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	if device == nil || device.AccountKey != resource.GetAccountKey() {
		return nil, status.Error(codes.NotFound, "device not found")
	}
	return &mihomov2.GetDeviceResponse{Device: toDeviceSummary(device)}, nil
}

func (s *MihomoRuntimeService) authorizeResource(ctx context.Context, resource *platformv2.BindingResource, action string, bindingWide bool) (*platformv2.BindingResource, usecase.ScopeGuard, error) {
	if resource == nil || resource.GetBindingRef() == "" || resource.GetAccountKey() == "" {
		return nil, usecase.ScopeGuard{}, status.Error(codes.InvalidArgument, "binding resource is required")
	}
	claims, err := authorizeTicketAction(ctx, s.ticketVerifier, action, false)
	if err != nil {
		return nil, usecase.ScopeGuard{}, err
	}
	if claims.BindingRef != resource.GetBindingRef() || claims.AccountKey != resource.GetAccountKey() {
		return nil, usecase.ScopeGuard{}, status.Error(codes.PermissionDenied, "resource does not match ticket")
	}
	guard := toScopeGuardMust(claims)
	if bindingWide {
		if err := guard.RequireBindingWide(); err != nil {
			return nil, usecase.ScopeGuard{}, mapUsecaseError(err)
		}
	}
	return resource, guard, nil
}
