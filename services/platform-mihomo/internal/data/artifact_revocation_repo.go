package data

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"platform-mihomo-service/internal/biz"
	"platform-mihomo-service/internal/data/model"
)

func (r *ArtifactRepo) PutRevocationIntentImmediately(ctx context.Context, intent *biz.ArtifactRevocationIntent) (*biz.ArtifactRevocationIntent, error) {
	record := revocationIntentRecord(intent)
	assignments := clause.Assignments(map[string]any{
		"intent_id": gorm.Expr("artifact_revocation_intents.intent_id"),
	})
	if intent.State == "ready" {
		assignments = clause.Assignments(map[string]any{
			"state":            "ready",
			"ready_after":      intent.ReadyAfter,
			"lease_token":      nil,
			"lease_expires_at": nil,
		})
	}
	err := r.db.WithContext(ctx).Clauses(
		clause.OnConflict{
			Columns:   []clause.Column{{Name: "binding_ref"}, {Name: "artifact_type"}, {Name: "scope_key"}, {Name: "artifact_value"}},
			DoUpdates: assignments,
		},
		clause.Returning{},
	).Create(&record).Error
	if err != nil {
		return nil, err
	}
	return revocationIntentFromRecord(record), nil
}

func (r *ArtifactRepo) MarkRevocationIntentReadyImmediately(ctx context.Context, intentID string) error {
	return r.db.WithContext(ctx).Model(&model.ArtifactRevocationIntent{}).
		Where("intent_id = ?", intentID).
		Updates(map[string]any{
			"state": "ready", "ready_after": time.Now().UTC(),
			"lease_token": nil, "lease_expires_at": nil,
		}).Error
}

func (r *ArtifactRepo) ClaimRevocationIntents(
	ctx context.Context,
	now, leaseExpiresAt time.Time,
	leaseToken string,
) ([]*biz.ArtifactRevocationIntent, error) {
	var records []model.ArtifactRevocationIntent
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("(state = 'ready' OR (state = 'provisional' AND ready_after <= ?)) AND (lease_expires_at IS NULL OR lease_expires_at <= ?)", now, now).
			Order("ready_after asc, intent_id asc").Limit(100).Find(&records).Error; err != nil {
			return err
		}
		if len(records) == 0 {
			return nil
		}
		ids := make([]string, 0, len(records))
		for _, record := range records {
			ids = append(ids, record.IntentID)
		}
		return tx.Model(&model.ArtifactRevocationIntent{}).Where("intent_id IN ?", ids).
			Updates(map[string]any{"lease_token": leaseToken, "lease_expires_at": leaseExpiresAt}).Error
	})
	if err != nil {
		return nil, err
	}
	intents := make([]*biz.ArtifactRevocationIntent, 0, len(records))
	for _, record := range records {
		record.LeaseToken = &leaseToken
		record.LeaseExpiresAt = &leaseExpiresAt
		intents = append(intents, revocationIntentFromRecord(record))
	}
	return intents, nil
}

func (r *ArtifactRepo) ResolveProvisionalRevocationIntent(ctx context.Context, intentID, leaseToken string) (bool, error) {
	shouldRevoke := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var intent model.ArtifactRevocationIntent
		if err := tx.Where("intent_id = ? AND lease_token = ?", intentID, leaseToken).Take(&intent).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		var credential model.CredentialRecord
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("binding_ref = ?", intent.BindingRef).Take(&credential).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("intent_id = ? AND lease_token = ?", intentID, leaseToken).
			Take(&intent).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		var artifact model.RuntimeArtifact
		err = tx.Where(
			"binding_ref = ? AND artifact_type = ? AND scope_key = ? AND artifact_value = ?",
			intent.BindingRef, intent.ArtifactType, intent.ScopeKey, intent.ArtifactValue,
		).Take(&artifact).Error
		if err == nil && !artifact.RevocationPending {
			return tx.Where("intent_id = ? AND lease_token = ?", intentID, leaseToken).
				Delete(&model.ArtifactRevocationIntent{}).Error
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		shouldRevoke = true
		return tx.Model(&model.ArtifactRevocationIntent{}).
			Where("intent_id = ? AND lease_token = ?", intentID, leaseToken).
			Update("state", "ready").Error
	})
	return shouldRevoke, err
}

func (r *ArtifactRepo) ReleaseRevocationIntentClaim(ctx context.Context, intentID, leaseToken string) error {
	return r.db.WithContext(ctx).Model(&model.ArtifactRevocationIntent{}).
		Where("intent_id = ? AND lease_token = ?", intentID, leaseToken).
		Updates(map[string]any{"lease_token": nil, "lease_expires_at": nil}).Error
}

func (r *ArtifactRepo) FinalizeRevocationIntentImmediately(ctx context.Context, intentID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var intent model.ArtifactRevocationIntent
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("intent_id = ?", intentID).Take(&intent).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if intent.State != "provisional" {
			return biz.ErrArtifactRevocationPending
		}
		return tx.Where("intent_id = ? AND state = 'provisional'", intentID).
			Delete(&model.ArtifactRevocationIntent{}).Error
	})
}

func (r *ArtifactRepo) DeleteRevocationIntentImmediately(ctx context.Context, intentID string) error {
	return r.db.WithContext(ctx).Where("intent_id = ?", intentID).Delete(&model.ArtifactRevocationIntent{}).Error
}

func revocationIntentRecord(intent *biz.ArtifactRevocationIntent) model.ArtifactRevocationIntent {
	return model.ArtifactRevocationIntent{
		IntentID: intent.IntentID, BindingRef: intent.BindingRef, AccountKey: intent.AccountKey,
		ArtifactType: intent.ArtifactType, ArtifactValue: intent.ArtifactValue, ScopeKey: intent.ScopeKey,
		ExpiresAt: intent.ExpiresAt, State: intent.State, ReadyAfter: intent.ReadyAfter,
		LeaseToken: optionalString(intent.LeaseToken), LeaseExpiresAt: intent.LeaseExpiresAt,
	}
}

func revocationIntentFromRecord(record model.ArtifactRevocationIntent) *biz.ArtifactRevocationIntent {
	return &biz.ArtifactRevocationIntent{
		IntentID: record.IntentID, BindingRef: record.BindingRef, AccountKey: record.AccountKey,
		ArtifactType: record.ArtifactType, ArtifactValue: record.ArtifactValue, ScopeKey: record.ScopeKey,
		ExpiresAt: record.ExpiresAt, State: record.State, ReadyAfter: record.ReadyAfter,
		LeaseToken: stringValue(record.LeaseToken), LeaseExpiresAt: record.LeaseExpiresAt,
	}
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
