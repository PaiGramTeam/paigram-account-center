package biz

import "context"

type CredentialManagementRepository interface {
	DeleteCredentialGraph(ctx context.Context, platformAccountID string) error
	DeleteCredentialGraphByBindingID(ctx context.Context, bindingID uint64) error
}
