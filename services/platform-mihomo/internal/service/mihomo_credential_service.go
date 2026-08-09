package service

import (
	"context"

	mihomov1 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/mihomo/v1"
	v1 "platform-mihomo-service/api/mihomo/v1"
	"platform-mihomo-service/internal/usecase"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"platform-mihomo-service/internal/data"
)

type MihomoCredentialService struct {
	mihomov1.UnimplementedMihomoCredentialServiceServer

	ticketVerifier *data.TicketVerifier
	managementUC   *usecase.ManagementUsecase
}

func NewMihomoCredentialService(ticketVerifier *data.TicketVerifier, managementUC *usecase.ManagementUsecase) *MihomoCredentialService {
	return &MihomoCredentialService{ticketVerifier: ticketVerifier, managementUC: managementUC}
}

func (s *MihomoCredentialService) GetCredentialSummary(ctx context.Context, req *mihomov1.GetCredentialSummaryRequest) (*mihomov1.GetCredentialSummaryResponse, error) {
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

	return toSharedCredentialSummary(output), nil
}

func toSharedCredentialSummary(output *usecase.CredentialSummaryOutput) *mihomov1.GetCredentialSummaryResponse {
	profiles := make([]*mihomov1.ProfileSummary, 0, len(output.Profiles))
	for _, profile := range output.Profiles {
		profiles = append(profiles, &mihomov1.ProfileSummary{
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

	devices := make([]*mihomov1.DeviceSummary, 0, len(output.Devices))
	for _, device := range output.Devices {
		devices = append(devices, &mihomov1.DeviceSummary{
			DeviceId:   device.DeviceID,
			DeviceFp:   device.DeviceFP,
			DeviceName: derefString(device.DeviceName),
			IsValid:    device.IsValid,
			LastSeenAt: toTimestamp(device.LastSeenAt),
		})
	}

	return &mihomov1.GetCredentialSummaryResponse{
		PlatformAccountId: output.PlatformAccountID,
		Status:            toSharedCredentialStatus(output.Status),
		LastValidatedAt:   toTimestamp(output.LastValidatedAt),
		LastRefreshedAt:   toTimestamp(output.LastRefreshedAt),
		Devices:           devices,
		Profiles:          profiles,
	}
}

func toSharedCredentialStatus(statusValue v1.CredentialStatus) mihomov1.CredentialStatus {
	switch statusValue {
	case v1.CredentialStatus_CREDENTIAL_STATUS_ACTIVE:
		return mihomov1.CredentialStatus_CREDENTIAL_STATUS_ACTIVE
	case v1.CredentialStatus_CREDENTIAL_STATUS_EXPIRED:
		return mihomov1.CredentialStatus_CREDENTIAL_STATUS_EXPIRED
	case v1.CredentialStatus_CREDENTIAL_STATUS_INVALID:
		return mihomov1.CredentialStatus_CREDENTIAL_STATUS_INVALID
	case v1.CredentialStatus_CREDENTIAL_STATUS_CHALLENGE_REQUIRED:
		return mihomov1.CredentialStatus_CREDENTIAL_STATUS_CHALLENGE_REQUIRED
	default:
		return mihomov1.CredentialStatus_CREDENTIAL_STATUS_UNSPECIFIED
	}
}
