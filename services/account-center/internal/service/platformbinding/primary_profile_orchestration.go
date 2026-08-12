package platformbinding

import (
	"context"
	"errors"
	"time"

	platformv2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v2"
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
	kind := platformv2.OperationKind_OPERATION_KIND_SET_PRIMARY_PROFILE
	reference := CredentialOperationReference{
		OperationID: operationID, Kind: kind.String(), BindingRef: binding.BindingRef,
		PreGeneration: binding.Generation, TargetGeneration: binding.Generation,
		RequestFingerprint: operationid.PrimaryProfileFingerprint(kind.String(), binding.BindingRef, profile.ProfileRef, binding.Generation, binding.ProfileRevision),
	}
	if s.operationIntents != nil {
		_, err = s.operationIntents.Admit(ctx, CredentialOperationIntentInput{
			OperationID: reference.OperationID, BindingID: binding.ID, BindingRef: reference.BindingRef, Kind: reference.Kind,
			PreGeneration: reference.PreGeneration, TargetGeneration: reference.TargetGeneration, RequestFingerprint: reference.RequestFingerprint,
			ProfileRef: profile.ProfileRef, ProfileRevision: binding.ProfileRevision, ActorType: "user", ActorID: actorID,
		})
		if err != nil {
			return nil, err
		}
	}
	ticket, _, err := s.platformService.IssueProfileScopedOperationTicket("user", actorID, binding, profile.ProfileRef, operationID, []string{platformaction.MihomoProfileWrite})
	if err != nil {
		if s.operationIntents != nil {
			_ = s.operationIntents.Reschedule(ctx, operationID, "ticket_issue_failed", time.Now().UTC().Add(credentialOperationRetryDelay))
			return nil, &CredentialOperationPendingError{OperationID: operationID, BindingID: binding.ID, State: model.PlatformOperationIntentStatePendingDelivery}
		}
		return nil, err
	}
	summary, err := s.gateway.SetPrimaryProfile(ctx, platformRow.Endpoint, ticket, operationID, binding, profile.ProfileRef)
	if err != nil {
		deliveryErr := s.handleNonSensitiveCredentialDeliveryError(ctx, binding, reference, err)
		var pending *CredentialOperationPendingError
		if errors.As(deliveryErr, &pending) {
			return nil, pending
		}
		mapped := normalizePrimaryProfileOperationError(err)
		s.recordBindingAudit(ctx, binding, "primary_profile_change", "failure", reasonCode(mapped), uint64Ptr(binding.OwnerUserID), "user", actorID, map[string]any{"profile_id": profileID})
		return nil, mapped
	}
	updated, err := s.applyAuthoritativeSummary(ctx, operationID, kind.String(), binding, binding.Generation, binding.ProfileRevision, profile.ProfileRef, summary)
	if err != nil {
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
