package service

import (
	"encoding/json"
	"time"

	platformv2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v2"

	"platform-mihomo-service/internal/biz"
	"platform-mihomo-service/internal/usecase"
)

type storedProfileSnapshot struct {
	Profiles         []*platformv2.ProfileSummary `json:"profiles"`
	Complete         bool                         `json:"complete"`
	Revision         uint64                       `json:"revision"`
	ObservedRevision uint64                       `json:"observed_revision"`
	LastValidatedAt  *time.Time                   `json:"last_validated_at,omitempty"`
	LastRefreshedAt  *time.Time                   `json:"last_refreshed_at,omitempty"`
}

func storedSnapshotFromSummary(summary *usecase.CredentialSummaryOutput) storedProfileSnapshot {
	return storedProfileSnapshot{
		Profiles:         profileSummaries(summary.Profiles),
		Complete:         summary.ProfileSnapshotComplete,
		Revision:         summary.ProfileRevision,
		ObservedRevision: summary.ProfileObservedRevision,
		LastValidatedAt:  summary.LastValidatedAt,
		LastRefreshedAt:  summary.LastRefreshedAt,
	}
}

func toOperationResult(result *biz.OperationResult) *platformv2.OperationResult {
	if result == nil {
		return nil
	}
	snapshot := &platformv2.ProfileSnapshot{}
	var stored storedProfileSnapshot
	if json.Unmarshal([]byte(result.SnapshotJSON), &stored) == nil {
		snapshot = &platformv2.ProfileSnapshot{
			Profiles:         stored.Profiles,
			Complete:         stored.Complete,
			Revision:         stored.Revision,
			ObservedRevision: stored.ObservedRevision,
		}
	}
	return &platformv2.OperationResult{
		Operation: &platformv2.OperationRef{
			OperationId:        result.Operation.OperationID,
			Kind:               operationKind(result.Operation.Kind),
			BindingRef:         result.Operation.BindingRef,
			PreGeneration:      result.Operation.PreGeneration,
			TargetGeneration:   result.Operation.TargetGeneration,
			RequestFingerprint: result.Operation.RequestFingerprint,
		},
		State:            operationState(result.State),
		ReasonCode:       result.ReasonCode,
		AccountKey:       result.AccountKey,
		CredentialStatus: toCredentialStatus(usecase.CredentialStatus(result.Status)),
		ProfileSnapshot:  snapshot,
		UpdatedAt:        toTimestamp(&result.UpdatedAt),
		LastValidatedAt:  toTimestamp(stored.LastValidatedAt),
		LastRefreshedAt:  toTimestamp(stored.LastRefreshedAt),
	}
}

func toBindingState(bindingRef string, summary *usecase.CredentialSummaryOutput) *platformv2.BindingState {
	if summary == nil {
		return nil
	}
	return &platformv2.BindingState{
		Exists:               true,
		BindingRef:           bindingRef,
		AccountKey:           summary.AccountKey,
		CredentialGeneration: summary.Generation,
		CredentialStatus:     toCredentialStatus(summary.Status),
		ProfileSnapshot:      toProfileSnapshot(summary.Profiles, summary.ProfileSnapshotComplete, summary.ProfileRevision, summary.ProfileObservedRevision),
		LastValidatedAt:      toTimestamp(summary.LastValidatedAt),
		LastRefreshedAt:      toTimestamp(summary.LastRefreshedAt),
	}
}

func toProfileSnapshot(profiles []*usecase.ProfileSummary, complete bool, revision, observedRevision uint64) *platformv2.ProfileSnapshot {
	return &platformv2.ProfileSnapshot{
		Profiles:         profileSummaries(profiles),
		Complete:         complete,
		Revision:         revision,
		ObservedRevision: observedRevision,
	}
}

func profileSummaries(profiles []*usecase.ProfileSummary) []*platformv2.ProfileSummary {
	items := make([]*platformv2.ProfileSummary, 0, len(profiles))
	for _, profile := range profiles {
		items = append(items, toProfileSummary(profile))
	}
	return items
}

func toProfileSummary(profile *usecase.ProfileSummary) *platformv2.ProfileSummary {
	if profile == nil {
		return nil
	}
	return &platformv2.ProfileSummary{
		ProfileRef: profile.ProfileRef,
		AccountKey: profile.AccountKey,
		GameBiz:    profile.GameBiz,
		Region:     profile.Region,
		PlayerId:   profile.PlayerID,
		Nickname:   profile.Nickname,
		Level:      profile.Level,
		IsDefault:  profile.IsDefault,
	}
}

func toDeviceSummary(device *biz.Device) *platformv2.DeviceSummary {
	if device == nil {
		return nil
	}
	return &platformv2.DeviceSummary{
		DeviceRef:  device.DeviceRef,
		DeviceName: derefString(device.DeviceName),
		IsValid:    device.IsValid,
		LastSeenAt: toTimestamp(device.LastSeenAt),
	}
}

func toCredentialStatus(status usecase.CredentialStatus) platformv2.CredentialStatus {
	switch status {
	case usecase.CredentialStatusActive:
		return platformv2.CredentialStatus_CREDENTIAL_STATUS_ACTIVE
	case usecase.CredentialStatusExpired:
		return platformv2.CredentialStatus_CREDENTIAL_STATUS_EXPIRED
	case usecase.CredentialStatusInvalid:
		return platformv2.CredentialStatus_CREDENTIAL_STATUS_INVALID
	case usecase.CredentialStatusChallengeRequired:
		return platformv2.CredentialStatus_CREDENTIAL_STATUS_CHALLENGE_REQUIRED
	default:
		return platformv2.CredentialStatus_CREDENTIAL_STATUS_UNSPECIFIED
	}
}

func operationKind(value string) platformv2.OperationKind {
	if enumValue, ok := platformv2.OperationKind_value[value]; ok {
		return platformv2.OperationKind(enumValue)
	}
	return platformv2.OperationKind_OPERATION_KIND_UNSPECIFIED
}

func operationState(value string) platformv2.OperationState {
	switch value {
	case "pending":
		return platformv2.OperationState_OPERATION_STATE_PENDING
	case "succeeded":
		return platformv2.OperationState_OPERATION_STATE_SUCCEEDED
	case "failed":
		return platformv2.OperationState_OPERATION_STATE_FAILED
	case "not_received":
		return platformv2.OperationState_OPERATION_STATE_NOT_RECEIVED
	case "failed_input_required":
		return platformv2.OperationState_OPERATION_STATE_FAILED_INPUT_REQUIRED
	default:
		return platformv2.OperationState_OPERATION_STATE_UNSPECIFIED
	}
}
