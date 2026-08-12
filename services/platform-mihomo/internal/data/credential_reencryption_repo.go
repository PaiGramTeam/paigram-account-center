package data

import (
	"context"

	"platform-mihomo-service/internal/biz"
	"platform-mihomo-service/internal/data/model"
)

func (r *CredentialRepo) ListCredentialReencryptionBatch(ctx context.Context, afterBindingRef string, limit int) ([]*biz.Credential, error) {
	if limit <= 0 {
		return []*biz.Credential{}, nil
	}
	var records []model.CredentialRecord
	if err := dbFromContext(ctx, r.db).
		Where("binding_ref > ?", afterBindingRef).
		Order("binding_ref ASC").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	credentials := make([]*biz.Credential, 0, len(records))
	for _, record := range records {
		credentials = append(credentials, credentialFromRecord(record))
	}
	return credentials, nil
}

func (r *CredentialRepo) ReencryptCredentialBlob(ctx context.Context, bindingRef, expectedBlob, replacementBlob string) (bool, error) {
	write := dbFromContext(ctx, r.db).Model(&model.CredentialRecord{}).
		Where("binding_ref = ? AND credential_blob = ?", bindingRef, expectedBlob).
		Updates(map[string]any{"credential_blob": replacementBlob, "credential_version": "v2"})
	return write.RowsAffected == 1, write.Error
}
