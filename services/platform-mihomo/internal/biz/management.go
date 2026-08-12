package biz

import "context"

type CredentialManagementRepository interface {
	DeleteCredentialGraph(ctx context.Context, accountKey string) error
	DeleteCredentialGraphByBindingRef(ctx context.Context, bindingRef string) error
}
