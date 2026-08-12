package usecase

import "platform-mihomo-service/internal/biz"

type CredentialStatus string

const (
	CredentialStatusUnspecified       CredentialStatus = "unspecified"
	CredentialStatusActive            CredentialStatus = "active"
	CredentialStatusExpired           CredentialStatus = "expired"
	CredentialStatusInvalid           CredentialStatus = "invalid"
	CredentialStatusChallengeRequired CredentialStatus = "challenge_required"
)

type ProfileSummary struct {
	ID         uint64
	AccountKey string
	ProfileRef string
	GameBiz    string
	Region     string
	PlayerID   string
	Nickname   string
	Level      int32
	IsDefault  bool
}

func credentialStatusFromStorage(status string) CredentialStatus {
	switch CredentialStatus(status) {
	case CredentialStatusActive, CredentialStatusExpired, CredentialStatusInvalid, CredentialStatusChallengeRequired:
		return CredentialStatus(status)
	default:
		return CredentialStatusUnspecified
	}
}

func toProfileSummary(profile *biz.Profile) *ProfileSummary {
	if profile == nil {
		return nil
	}
	return &ProfileSummary{
		ID:         profile.ID,
		AccountKey: profile.AccountKey,
		ProfileRef: profile.ProfileRef,
		GameBiz:    profile.GameBiz,
		Region:     profile.Region,
		PlayerID:   profile.PlayerID,
		Nickname:   profile.Nickname,
		Level:      int32(profile.Level),
		IsDefault:  profile.IsDefault,
	}
}
