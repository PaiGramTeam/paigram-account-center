package service

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "platform-mihomo-service/api/mihomo/v1"
	"platform-mihomo-service/internal/biz"
	"platform-mihomo-service/internal/usecase"
)

func toBindCredentialInput(req *v1.BindCredentialRequest, claims *biz.ServiceTicketClaims) usecase.BindCredentialInput {
	input := usecase.BindCredentialInput{
		BindingID:        claims.BindingID,
		CookieBundleJSON: req.GetCookieBundleJson(),
		RegionHint:       req.GetRegionHint(),
	}

	if device := req.GetDevice(); device != nil {
		input.DeviceID = device.GetDeviceId()
		input.DeviceFP = device.GetDeviceFp()
		input.DeviceName = device.GetDeviceName()
	}

	return input
}

func toScopeGuard(claims *biz.ServiceTicketClaims) (usecase.ScopeGuard, error) {
	if claims.Platform != "mihomo" {
		return usecase.ScopeGuard{}, usecase.ErrBindingScopeDenied
	}

	allowedActions := make(map[string]struct{}, len(claims.Scopes))
	for _, scope := range claims.Scopes {
		allowedActions[scope] = struct{}{}
	}

	return usecase.ScopeGuard{
		AllowedActions: allowedActions,
		BindingID:      claims.BindingID,
		ProfileID:      claims.ProfileID,
	}, nil
}

func toBindCredentialResponse(output *usecase.BindCredentialOutput) *v1.BindCredentialResponse {
	profiles := make([]*v1.ProfileSummary, 0, len(output.Profiles))
	for i := range output.Profiles {
		profile := &output.Profiles[i]
		profiles = append(profiles, &v1.ProfileSummary{
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

	return &v1.BindCredentialResponse{
		PlatformAccountId: output.PlatformAccountID,
		Status:            output.Status,
		Profiles:          profiles,
	}
}

func toCredentialSummaryResponse(output *usecase.CredentialSummaryOutput) *v1.GetCredentialSummaryResponse {
	return &v1.GetCredentialSummaryResponse{Summary: toCredentialSummary(output)}
}

func toCredentialSummary(output *usecase.CredentialSummaryOutput) *v1.CredentialSummary {
	if output == nil {
		return nil
	}

	profiles := make([]*v1.ProfileSummary, 0, len(output.Profiles))
	for _, profile := range output.Profiles {
		profiles = append(profiles, profile)
	}

	return &v1.CredentialSummary{
		PlatformAccountId: output.PlatformAccountID,
		Status:            output.Status,
		LastValidatedAt:   toTimestamp(output.LastValidatedAt),
		LastRefreshedAt:   toTimestamp(output.LastRefreshedAt),
		Devices:           toDeviceSummaries(output.Devices),
		Profiles:          profiles,
	}
}

func toDeviceSummaries(devices []*biz.Device) []*v1.DeviceSummary {
	result := make([]*v1.DeviceSummary, 0, len(devices))
	for _, device := range devices {
		result = append(result, &v1.DeviceSummary{
			DeviceId:   device.DeviceID,
			DeviceFp:   device.DeviceFP,
			DeviceName: derefString(device.DeviceName),
			IsValid:    device.IsValid,
			LastSeenAt: toTimestamp(device.LastSeenAt),
		})
	}
	return result
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func toTimestamp(ts *time.Time) *timestamppb.Timestamp {
	if ts == nil {
		return nil
	}

	return timestamppb.New(*ts)
}
