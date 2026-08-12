package usecase

import (
	"context"

	"platform-mihomo-service/internal/biz"
	internalcrypto "platform-mihomo-service/internal/crypto"
)

const credentialReencryptionBatchSize = 100

type credentialReencryptionRepository interface {
	ListCredentialReencryptionBatch(ctx context.Context, afterBindingRef string, limit int) ([]*biz.Credential, error)
	ReencryptCredentialBlob(ctx context.Context, bindingRef, expectedBlob, replacementBlob string) (bool, error)
}

type CredentialReencryptionUsecase struct {
	repository credentialReencryptionRepository
	keyring    internalcrypto.KeyProvider
}

func NewCredentialReencryptionUsecase(repository credentialReencryptionRepository, keyring internalcrypto.KeyProvider) *CredentialReencryptionUsecase {
	return &CredentialReencryptionUsecase{repository: repository, keyring: keyring}
}

func (uc *CredentialReencryptionUsecase) ReencryptAll(ctx context.Context) (int64, error) {
	if uc == nil || uc.repository == nil || uc.keyring == nil {
		return 0, internalcrypto.ErrInvalidKeyring
	}
	var updated int64
	afterBindingRef := ""
	for {
		credentials, err := uc.repository.ListCredentialReencryptionBatch(ctx, afterBindingRef, credentialReencryptionBatchSize)
		if err != nil {
			return updated, err
		}
		if len(credentials) == 0 {
			return updated, nil
		}
		for _, credential := range credentials {
			afterBindingRef = credential.BindingRef
			needsReencryption, err := internalcrypto.EnvelopeNeedsReencryption(uc.keyring, credential.CredentialBlob)
			if err != nil {
				return updated, err
			}
			if !needsReencryption {
				continue
			}
			plaintext, err := internalcrypto.DecryptString(uc.keyring, credential.CredentialBlob)
			if err != nil {
				return updated, err
			}
			replacement, err := internalcrypto.EncryptString(uc.keyring, plaintext)
			if err != nil {
				return updated, err
			}
			replaced, err := uc.repository.ReencryptCredentialBlob(ctx, credential.BindingRef, credential.CredentialBlob, replacement)
			if err != nil {
				return updated, err
			}
			if replaced {
				updated++
			}
		}
		if len(credentials) < credentialReencryptionBatchSize {
			return updated, nil
		}
	}
}
