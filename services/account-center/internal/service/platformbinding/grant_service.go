package platformbinding

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/platformaction"
	"gorm.io/gorm"

	"paigram/internal/model"
	serviceaudit "paigram/internal/service/audit"
)

var defaultConsumerActions = platformaction.MihomoDelegationActions()

type GrantService struct {
	db                  *gorm.DB
	invalidator         GrantInvalidator
	transactionObserver GrantTransactionObserver
}

func NewGrantService(db *gorm.DB, dependencies ...any) *GrantService {
	service := &GrantService{db: db}
	for _, dependency := range dependencies {
		if invalidator, ok := dependency.(GrantInvalidator); ok {
			service.invalidator = invalidator
		}
		if observer, ok := dependency.(GrantTransactionObserver); ok {
			service.transactionObserver = observer
		}
	}
	return service
}

func (s *GrantService) UpsertGrant(input UpsertGrantInput) (*model.ConsumerGrant, bool, error) {
	if err := s.validateConsumer(input.Consumer); err != nil {
		return nil, false, err
	}
	actions, err := normalizeGrantActions(input.Actions)
	if err != nil {
		return nil, false, err
	}

	binding, err := s.getBinding(input.BindingID)
	if err != nil {
		return nil, false, err
	}

	grantedAt := input.GrantedAt.UTC()
	if grantedAt.IsZero() {
		grantedAt = time.Now().UTC()
	}

	var grant model.ConsumerGrant
	created := false
	shouldInvalidate := false
	minimumGrantVersion := uint64(0)
	err = runGrantTransaction(s.db, s.transactionObserver, func(tx *gorm.DB) error {
		grant = model.ConsumerGrant{}
		created = false
		shouldInvalidate = false
		minimumGrantVersion = 0
		lookup := tx.Preload("Actions").Where("binding_id = ? AND consumer = ?", input.BindingID, input.Consumer).First(&grant)
		if lookup.Error != nil {
			if !errors.Is(lookup.Error, gorm.ErrRecordNotFound) {
				return lookup.Error
			}

			grant = model.ConsumerGrant{
				BindingID:     input.BindingID,
				Consumer:      input.Consumer,
				Status:        model.ConsumerGrantStatusActive,
				TicketVersion: 1,
				GrantedBy:     input.GrantedBy,
				GrantedAt:     grantedAt,
			}
			created = true
			if err := tx.Create(&grant).Error; err != nil {
				return err
			}
			if err := replaceGrantActions(tx, &grant, actions); err != nil {
				return err
			}
			return writeGrantAudit(tx, binding, input.BindingID, input.Consumer, auditActorUserID(input.GrantedBy), true, created)
		}

		statusChanged := grant.Status != model.ConsumerGrantStatusActive || grant.RevokedAt.Valid
		grant.Status = model.ConsumerGrantStatusActive
		actionsChanged := !slices.Equal(grantActionNames(grant.Actions), actions)
		if actionsChanged || statusChanged {
			grant.TicketVersion = nextTicketVersion(grant.TicketVersion)
			shouldInvalidate = true
			minimumGrantVersion = grant.TicketVersion
			grant.LastInvalidatedAt = sql.NullTime{}
		}
		if grant.TicketVersion == 0 {
			grant.TicketVersion = 1
		}
		if actionsChanged {
			if err := replaceGrantActions(tx, &grant, actions); err != nil {
				return err
			}
		}
		grant.GrantedBy = input.GrantedBy
		grant.GrantedAt = grantedAt
		grant.RevokedAt = sql.NullTime{}
		if !created && !grant.LastInvalidatedAt.Valid && grant.TicketVersion > 1 {
			shouldInvalidate = true
			minimumGrantVersion = grant.TicketVersion
		}
		if err := tx.Save(&grant).Error; err != nil {
			return err
		}
		return writeGrantAudit(tx, binding, input.BindingID, input.Consumer, auditActorUserID(input.GrantedBy), true, created)
	})
	if err != nil {
		return nil, false, err
	}
	if shouldInvalidate {
		ctx := input.Context
		if ctx == nil {
			ctx = context.Background()
		}
		if err := s.invalidateGrant(ctx, binding, input.Consumer, input.GrantedBy, minimumGrantVersion, grant.PendingEntryEpoch); err != nil {
			return nil, false, newGrantPropagationPendingError(binding.ID, input.Consumer, minimumGrantVersion, err)
		}
		if err := s.completeGrantInvalidation(&grant, minimumGrantVersion, grant.PendingEntryEpoch, grantedAt); err != nil {
			return nil, false, newGrantPropagationPendingError(binding.ID, input.Consumer, minimumGrantVersion, err)
		}
	}

	return &grant, created, nil
}

func grantActionNames(rows []model.ConsumerGrantAction) []string {
	actions := make([]string, 0, len(rows))
	for _, row := range rows {
		actions = append(actions, row.Action)
	}
	slices.Sort(actions)
	return actions
}

func replaceGrantActions(tx *gorm.DB, grant *model.ConsumerGrant, actions []string) error {
	if err := tx.Where("grant_id = ?", grant.ID).Delete(&model.ConsumerGrantAction{}).Error; err != nil {
		return err
	}

	rows := make([]model.ConsumerGrantAction, 0, len(actions))
	for _, action := range actions {
		rows = append(rows, model.ConsumerGrantAction{GrantID: grant.ID, Action: action})
	}
	if err := tx.Create(&rows).Error; err != nil {
		return err
	}
	grant.Actions = rows
	return nil
}

func normalizeGrantActions(actions []string) ([]string, error) {
	if len(actions) == 0 {
		return slices.Clone(defaultConsumerActions), nil
	}

	normalized := make([]string, 0, len(actions))
	seen := make(map[string]struct{}, len(actions))
	for _, action := range actions {
		action = strings.TrimSpace(action)
		if !platformaction.IsMihomoDelegationAction(action) {
			return nil, ErrGrantActionNotAllowed
		}
		if _, exists := seen[action]; exists {
			continue
		}
		seen[action] = struct{}{}
		normalized = append(normalized, action)
	}
	if len(normalized) == 0 {
		return nil, ErrGrantActionNotAllowed
	}
	slices.Sort(normalized)
	return normalized, nil
}

func nextTicketVersion(current uint64) uint64 {
	if current == 0 {
		return 1
	}
	return current + 1
}

func (s *GrantService) RevokeGrant(input RevokeGrantInput) (*model.ConsumerGrant, error) {
	if err := s.validateConsumer(input.Consumer); err != nil {
		return nil, err
	}
	ctx := input.Context
	if ctx == nil {
		ctx = context.Background()
	}

	binding, err := s.getBinding(input.BindingID)
	if err != nil {
		return nil, err
	}

	revokedAt := input.RevokedAt.UTC()
	if revokedAt.IsZero() {
		revokedAt = time.Now().UTC()
	}

	var grant model.ConsumerGrant
	shouldInvalidate := false
	minimumGrantVersion := uint64(0)
	err = runGrantTransaction(s.db, s.transactionObserver, func(tx *gorm.DB) error {
		grant = model.ConsumerGrant{}
		shouldInvalidate = false
		minimumGrantVersion = 0
		if err := tx.Preload("Actions").Where("binding_id = ? AND consumer = ?", input.BindingID, input.Consumer).First(&grant).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				grant = model.ConsumerGrant{
					BindingID:     input.BindingID,
					Consumer:      input.Consumer,
					Status:        model.ConsumerGrantStatusRevoked,
					TicketVersion: 1,
					GrantedAt:     revokedAt,
					RevokedAt:     sql.NullTime{Time: revokedAt, Valid: true},
				}
				if err := tx.Create(&grant).Error; err != nil {
					return err
				}
				shouldInvalidate = true
				minimumGrantVersion = 1
				writeGrantAuditBestEffort(tx, binding, input.BindingID, input.Consumer, auditActorUserID(input.ActorUserID), false, false)
				return nil
			}

			return err
		}

		if grant.Status == model.ConsumerGrantStatusRevoked && grant.RevokedAt.Valid {
			if !grant.LastInvalidatedAt.Valid {
				minimumGrantVersion = grant.TicketVersion
				if minimumGrantVersion == 0 {
					minimumGrantVersion = 1
				}
				shouldInvalidate = true
			}
			writeGrantAuditBestEffort(tx, binding, input.BindingID, input.Consumer, auditActorUserID(input.ActorUserID), false, true)
			return nil
		}

		nextVersion := grant.TicketVersion
		if nextVersion == 0 {
			nextVersion = 1
		}
		nextVersion++
		grant.Status = model.ConsumerGrantStatusRevoked
		grant.TicketVersion = nextVersion
		grant.RevokedAt = sql.NullTime{Time: revokedAt, Valid: true}
		grant.LastInvalidatedAt = sql.NullTime{}
		if err := tx.Save(&grant).Error; err != nil {
			return err
		}
		shouldInvalidate = true
		minimumGrantVersion = nextVersion
		writeGrantAuditBestEffort(tx, binding, input.BindingID, input.Consumer, auditActorUserID(input.ActorUserID), false, false)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if shouldInvalidate {
		if err := s.invalidateGrant(ctx, binding, input.Consumer, input.ActorUserID, minimumGrantVersion, grant.PendingEntryEpoch); err != nil {
			return nil, newGrantPropagationPendingError(binding.ID, input.Consumer, minimumGrantVersion, err)
		}
		if err := s.completeGrantInvalidation(&grant, minimumGrantVersion, grant.PendingEntryEpoch, revokedAt); err != nil {
			return nil, newGrantPropagationPendingError(binding.ID, input.Consumer, minimumGrantVersion, err)
		}
	}

	return &grant, nil
}

func (s *GrantService) ListGrants(bindingID uint64, params ListParams) ([]model.ConsumerGrant, int64, error) {
	params = normalizeListParams(params)

	if err := s.ensureBindingExists(bindingID); err != nil {
		return nil, 0, err
	}

	query := s.db.Model(&model.ConsumerGrant{}).Where("binding_id = ?", bindingID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var grants []model.ConsumerGrant
	if err := query.Preload("Actions").Order("id ASC").Offset(pageOffset(params)).Limit(params.PageSize).Find(&grants).Error; err != nil {
		return nil, 0, err
	}

	return grants, total, nil
}

func (s *GrantService) ListGrantsForOwner(ownerUserID, bindingID uint64, params ListParams) ([]model.ConsumerGrant, int64, error) {
	if err := s.ensureBindingOwnedByUser(ownerUserID, bindingID); err != nil {
		return nil, 0, err
	}

	return s.ListGrants(bindingID, params)
}

func (s *GrantService) DeleteGrants(bindingID uint64) error {
	if err := s.ensureBindingExists(bindingID); err != nil {
		return err
	}

	return s.db.Where("binding_id = ?", bindingID).Delete(&model.ConsumerGrant{}).Error
}

func (s *GrantService) ListPendingGrantInvalidationIDs(ctx context.Context, limit int) ([]uint64, error) {
	if limit <= 0 {
		limit = 100
	}
	var grantIDs []uint64
	err := s.db.WithContext(ctx).Model(&model.ConsumerGrant{}).
		Where("last_invalidated_at IS NULL AND ticket_version > 0").
		Order("updated_at ASC, id ASC").Limit(limit).Pluck("id", &grantIDs).Error
	return grantIDs, err
}

func (s *GrantService) ReconcileGrantInvalidation(ctx context.Context, grantID uint64) error {
	var grant model.ConsumerGrant
	if err := s.db.WithContext(ctx).Preload("Actions").First(&grant, grantID).Error; err != nil {
		return err
	}
	if grant.LastInvalidatedAt.Valid {
		return nil
	}
	binding, err := s.getBinding(grant.BindingID)
	if err != nil {
		return err
	}
	minimumVersion := grant.TicketVersion
	if minimumVersion == 0 {
		minimumVersion = 1
	}
	minimumEntryEpoch := grant.PendingEntryEpoch
	if err := s.invalidateGrant(ctx, binding, grant.Consumer, grant.GrantedBy, minimumVersion, minimumEntryEpoch); err != nil {
		return newGrantPropagationPendingError(binding.ID, grant.Consumer, minimumVersion, err)
	}
	if err := s.completeGrantInvalidation(&grant, minimumVersion, minimumEntryEpoch, time.Now().UTC()); err != nil {
		return newGrantPropagationPendingError(binding.ID, grant.Consumer, minimumVersion, err)
	}
	return nil
}

func (s *GrantService) UpsertGrantForOwner(ownerUserID uint64, input UpsertGrantInput) (*model.ConsumerGrant, bool, error) {
	if err := s.ensureBindingOwnedByUser(ownerUserID, input.BindingID); err != nil {
		return nil, false, err
	}

	return s.UpsertGrant(input)
}

func (s *GrantService) RevokeGrantForOwner(ownerUserID uint64, input RevokeGrantInput) (*model.ConsumerGrant, error) {
	if err := s.ensureBindingOwnedByUser(ownerUserID, input.BindingID); err != nil {
		return nil, err
	}

	return s.RevokeGrant(input)
}

func IsGrantActive(grant model.ConsumerGrant) bool {
	return grant.Status == model.ConsumerGrantStatusActive && !grant.RevokedAt.Valid
}

func (s *GrantService) ensureBindingExists(bindingID uint64) error {
	_, err := s.getBinding(bindingID)
	return err
}

func (s *GrantService) getBinding(bindingID uint64) (*model.PlatformAccountBinding, error) {
	var binding model.PlatformAccountBinding
	if err := s.db.Select("id", "binding_ref", "generation", "owner_user_id", "platform", "platform_service_key").First(&binding, bindingID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBindingNotFound
		}

		return nil, err
	}

	return &binding, nil
}

func (s *GrantService) ensureBindingOwnedByUser(ownerUserID, bindingID uint64) error {
	var binding model.PlatformAccountBinding
	if err := s.db.Select("id").Where("owner_user_id = ?", ownerUserID).First(&binding, bindingID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrBindingNotFound
		}

		return err
	}

	return nil
}

func (s *GrantService) invalidateGrant(ctx context.Context, binding *model.PlatformAccountBinding, consumer string, actorUserID sql.NullInt64, minimumGrantVersion uint64, minimumEntryEpoch ...uint64) error {
	if s.invalidator == nil {
		return nil
	}
	actorType := "user"
	actorID := "system:grant-revoke"
	if actorUserID.Valid && actorUserID.Int64 > 0 {
		actorID = strconv.FormatInt(actorUserID.Int64, 10)
		if uint64(actorUserID.Int64) != binding.OwnerUserID {
			actorType = "admin"
		}
	}
	err := s.invalidator.InvalidateConsumerGrant(ctx, GrantInvalidationInput{
		BindingID:           binding.ID,
		BindingRef:          binding.BindingRef,
		Generation:          binding.Generation,
		OwnerUserID:         binding.OwnerUserID,
		Platform:            binding.Platform,
		PlatformServiceKey:  binding.PlatformServiceKey,
		Consumer:            consumer,
		MinimumGrantVersion: minimumGrantVersion,
		MinimumEntryEpoch:   firstEpoch(minimumEntryEpoch),
		ActorType:           actorType,
		ActorID:             actorID,
	})
	if err != nil {
		return err
	}
	return nil
}

func firstEpoch(values []uint64) uint64 {
	if len(values) == 0 {
		return 0
	}
	return values[0]
}

func (s *GrantService) validateConsumer(consumer string) error {
	for _, supportedConsumer := range SupportedConsumers {
		if consumer == supportedConsumer {
			return nil
		}
	}
	if s != nil && s.db != nil {
		var count int64
		if err := s.db.Model(&model.ServiceCredential{}).
			Where("client_id = ? AND status = ?", consumer, model.ServiceCredentialStatusActive).
			Count(&count).Error; err == nil && count == 1 {
			return nil
		}
	}

	return ErrConsumerNotSupported
}

func newGrantPropagationPendingError(bindingID uint64, consumer string, minimumVersion uint64, cause error) error {
	return &GrantPropagationPendingError{
		BindingID: bindingID, Consumer: consumer, MinimumGrantVersion: minimumVersion, Cause: cause,
	}
}

func writeGrantAudit(tx *gorm.DB, binding *model.PlatformAccountBinding, bindingID uint64, consumer string, actorUserID *uint64, enabled bool, idempotent bool) error {
	ownerUserID := uint64(0)
	if binding != nil {
		ownerUserID = binding.OwnerUserID
	}
	actorType := "user"
	if actorUserID != nil && ownerUserID != 0 && *actorUserID != ownerUserID {
		actorType = "admin"
	}
	return serviceaudit.RecordTx(tx, serviceaudit.WriteInput{
		Category:    "platform_binding",
		ActorType:   actorType,
		ActorUserID: actorUserID,
		Action:      "grant_change",
		TargetType:  "binding",
		TargetID:    strconv.FormatUint(bindingID, 10),
		BindingID:   &bindingID,
		OwnerUserID: zeroableUint64(ownerUserID),
		Result:      "success",
		Metadata: map[string]any{
			"consumer":      consumer,
			"grant_enabled": enabled,
			"idempotent":    idempotent,
		},
	})
}

func writeGrantAuditBestEffort(tx *gorm.DB, binding *model.PlatformAccountBinding, bindingID uint64, consumer string, actorUserID *uint64, enabled bool, idempotent bool) {
	_ = tx.Transaction(func(nested *gorm.DB) error {
		return writeGrantAudit(nested, binding, bindingID, consumer, actorUserID, enabled, idempotent)
	})
}

func auditActorUserID(value sql.NullInt64) *uint64 {
	if !value.Valid || value.Int64 <= 0 {
		return nil
	}
	converted := uint64(value.Int64)
	return &converted
}

func zeroableUint64(value uint64) *uint64 {
	if value == 0 {
		return nil
	}
	return &value
}
