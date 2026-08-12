package platformaction

import "slices"

const (
	MihomoAuthKeyIssue            = "mihomo.authkey.issue"
	MihomoAuthorizationFenceApply = "mihomo.authorization.fence.apply"
	MihomoBindingRead             = "mihomo.binding.read"
	MihomoCredentialBind          = "mihomo.credential.bind"
	MihomoCredentialDelete        = "mihomo.credential.delete"
	MihomoCredentialRefresh       = "mihomo.credential.refresh"
	MihomoCredentialUpdate        = "mihomo.credential.update"
	MihomoCredentialValidate      = "mihomo.credential.validate"
	MihomoDeviceRead              = "mihomo.device.read"
	MihomoOperationRead           = "mihomo.operation.read"
	MihomoOperationResolve        = "mihomo.operation.resolve"
	MihomoProfileRead             = "mihomo.profile.read"
	MihomoStatusRead              = "mihomo.status.read"
)

var mihomoDelegationActions = []string{
	MihomoAuthKeyIssue,
	MihomoCredentialValidate,
	MihomoDeviceRead,
	MihomoProfileRead,
	MihomoStatusRead,
}

var mihomoControlActions = []string{
	MihomoAuthorizationFenceApply,
	MihomoBindingRead,
	MihomoCredentialBind,
	MihomoCredentialDelete,
	MihomoCredentialRefresh,
	MihomoCredentialUpdate,
	MihomoOperationRead,
	MihomoOperationResolve,
}

func MihomoDelegationActions() []string {
	return slices.Clone(mihomoDelegationActions)
}

func MihomoControlActions() []string {
	return slices.Clone(mihomoControlActions)
}

func IsMihomoDelegationAction(action string) bool {
	return slices.Contains(mihomoDelegationActions, action)
}

func IsMihomoControlAction(action string) bool {
	return slices.Contains(mihomoControlActions, action)
}
