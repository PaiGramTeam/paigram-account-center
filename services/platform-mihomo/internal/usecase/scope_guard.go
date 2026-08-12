package usecase

import (
	"errors"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/platformaction"
)

var ErrActionScopeDenied = errors.New("action is outside ticket scope")
var ErrBindingScopeDenied = errors.New("binding is outside ticket scope")
var ErrProfileScopeDenied = errors.New("profile is outside ticket scope")

const (
	ActionStatusRead         = platformaction.MihomoStatusRead
	ActionCredentialValidate = platformaction.MihomoCredentialValidate
	ActionProfileRead        = platformaction.MihomoProfileRead
	ActionAuthKeyIssue       = platformaction.MihomoAuthKeyIssue
	ActionCredentialBind     = platformaction.MihomoCredentialBind
	ActionDeviceRead         = platformaction.MihomoDeviceRead
	ActionBindingRead        = platformaction.MihomoBindingRead
	ActionOperationRead      = platformaction.MihomoOperationRead
	ActionOperationResolve   = platformaction.MihomoOperationResolve
	ActionAuthorizationFence = platformaction.MihomoAuthorizationFenceApply
	ActionCredentialUpdate   = platformaction.MihomoCredentialUpdate
	ActionCredentialRefresh  = platformaction.MihomoCredentialRefresh
	ActionCredentialDelete   = platformaction.MihomoCredentialDelete
	ActionProfilePrimarySet  = platformaction.MihomoProfilePrimarySet
)

type ScopeGuard struct {
	AllowedActions map[string]struct{}
	BindingRef     string
	AccountKey     string
	ProfileRef     string
}

func (g ScopeGuard) RequireAction(action string) error {
	if _, ok := g.AllowedActions[action]; !ok {
		return ErrActionScopeDenied
	}
	return nil
}

func (g ScopeGuard) RequireAccountKey(accountKey string) error {
	if g.BindingRef == "" || g.AccountKey == "" || accountKey == "" || g.AccountKey != accountKey {
		return ErrBindingScopeDenied
	}
	return nil
}

func (g ScopeGuard) RequireBindingWide() error {
	if g.ProfileRef != "" {
		return ErrProfileScopeDenied
	}
	return nil
}

func (g ScopeGuard) RequireProfile(bindingRef, profileRef string) error {
	if g.BindingRef == "" || g.BindingRef != bindingRef {
		return ErrBindingScopeDenied
	}
	if g.ProfileRef != "" && g.ProfileRef != profileRef {
		return ErrProfileScopeDenied
	}
	return nil
}
