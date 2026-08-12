package platformbinding

import (
	"context"
	"errors"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/operationid"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/platformaction"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"gorm.io/gorm"

	"paigram/internal/model"
)

func (s *OrchestrationService) SetPrimaryProfileForOwner(ctx context.Context, ownerUserID, bindingID, profileID uint64, actorID string) (*model.PlatformAccountBinding, error) {
	binding, err := s.bindingReader.GetBindingForOwner(ownerUserID, bindingID)
	if err != nil {
		return nil, err
	}
	if binding == nil || !binding.ExternalAccountKey.Valid || binding.ExternalAccountKey.String == "" {
		return nil, ErrBindingRuntimeSummaryNotReady
	}
	if s.gateway == nil || s.profileSyncer == nil {
		return nil, ErrCredentialGatewayUnavailable
	}
	profile, err := s.profileSyncer.GetProfile(binding.ID, profileID)
	if err != nil {
		return nil, err
	}
	if profile == nil || profile.ProfileRef == "" {
		return nil, ErrPrimaryProfileNotOwned
	}
	platformRow, err := s.platformService.GetEnabledPlatform(binding.Platform)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPlatformServiceUnavailable
		}
		return nil, err
	}
	operationID, err := operationid.NewID()
	if err != nil {
		return nil, err
	}
	ticket, _, err := s.platformService.IssueBindingScopedOperationTicket("user", actorID, binding, operationID, []string{platformaction.MihomoProfilePrimarySet})
	if err != nil {
		return nil, err
	}
	summary, err := s.gateway.SetPrimaryProfile(ctx, platformRow.Endpoint, ticket, operationID, binding, profile.ProfileRef)
	if err != nil {
		mapped := normalizePrimaryProfileOperationError(err)
		s.recordBindingAudit(ctx, binding, "primary_profile_change", "failure", reasonCode(mapped), uint64Ptr(binding.OwnerUserID), "user", actorID, map[string]any{"profile_id": profileID})
		return nil, mapped
	}
	if summary == nil || !summary.ProfileSnapshotComplete || summary.ProfileObservedRevision < summary.ProfileRevision {
		return nil, ErrPlatformSummaryProxyUnavailable
	}
	updated, err := s.bindingReader.PersistRuntimeSummary(binding.ID, *summary)
	if err != nil {
		return nil, err
	}
	if err := s.syncProfiles(binding, updated, summary); err != nil {
		return nil, err
	}
	s.recordBindingAudit(ctx, binding, "primary_profile_change", "success", "", uint64Ptr(binding.OwnerUserID), "user", actorID, map[string]any{"profile_id": profileID})
	return updated, nil
}

func normalizePrimaryProfileOperationError(err error) error {
	if err == nil {
		return nil
	}
	switch grpcstatus.Code(err) {
	case codes.Aborted:
		return ErrBindingGenerationConflict
	case codes.NotFound, codes.PermissionDenied:
		return ErrPrimaryProfileNotOwned
	case codes.Unavailable, codes.DeadlineExceeded, codes.Canceled:
		return ErrCredentialGatewayUnavailable
	default:
		return err
	}
}
