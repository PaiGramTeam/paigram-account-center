package usecase

import "errors"

var ErrActionScopeDenied = errors.New("action is outside ticket scope")
var ErrBindingScopeDenied = errors.New("binding is outside ticket scope")
var ErrProfileScopeDenied = errors.New("profile is outside ticket scope")

const (
	ActionStatusRead       = "mihomo.status.read"
	ActionProfileRead      = "mihomo.profile.read"
	ActionProfileWrite     = "mihomo.profile.write"
	ActionAuthKeyIssue     = "mihomo.authkey.issue"
	ActionCredentialBind   = "mihomo.credential.bind"
	ActionDeviceUpdate     = "mihomo.device.update"
	ActionCredentialRead   = "mihomo.credential.read_meta"
	ActionCredentialUpdate = "mihomo.credential.update"
	ActionCredentialRefresh = "mihomo.credential.refresh"
	ActionCredentialDelete = "mihomo.credential.delete"
)

type ScopeGuard struct {
	AllowedActions map[string]struct{}
	BindingID      uint64
	ProfileID      uint64
}

func (g ScopeGuard) RequireAction(action string) error {
	if _, ok := g.AllowedActions[action]; !ok {
		return ErrActionScopeDenied
	}
	return nil
}

func (g ScopeGuard) RequirePlatformAccountID(platformAccountID string) error {
	bindingID, err := BindingIDFromPlatformAccountID(platformAccountID)
	if err != nil || bindingID == 0 || g.BindingID != bindingID {
		return ErrBindingScopeDenied
	}
	return nil
}

func (g ScopeGuard) RequireBindingWide() error {
	if g.ProfileID != 0 {
		return ErrProfileScopeDenied
	}
	return nil
}

func (g ScopeGuard) RequireProfile(bindingID, profileID uint64) error {
	if g.BindingID == 0 || g.BindingID != bindingID {
		return ErrBindingScopeDenied
	}
	if g.ProfileID != 0 && g.ProfileID != profileID {
		return ErrProfileScopeDenied
	}
	return nil
}
