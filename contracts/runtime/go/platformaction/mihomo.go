package platformaction

import "slices"

const (
	MihomoAuthKeyIssue            = "mihomo.authkey.issue"
	MihomoConsumerGrantInvalidate = "mihomo.consumer_grant.invalidate"
	MihomoCredentialBind          = "mihomo.credential.bind"
	MihomoCredentialDelete        = "mihomo.credential.delete"
	MihomoCredentialRead          = "mihomo.credential.read_meta"
	MihomoCredentialRefresh       = "mihomo.credential.refresh"
	MihomoCredentialUpdate        = "mihomo.credential.update"
	MihomoCredentialValidate      = "mihomo.credential.validate"
	MihomoDeviceUpdate            = "mihomo.device.update"
	MihomoProfileRead             = "mihomo.profile.read"
	MihomoProfileWrite            = "mihomo.profile.write"
	MihomoStatusRead              = "mihomo.status.read"
)

var mihomoDelegationActions = []string{
	MihomoAuthKeyIssue,
	MihomoCredentialRead,
	MihomoCredentialValidate,
	MihomoDeviceUpdate,
	MihomoProfileRead,
	MihomoStatusRead,
}

var mihomoControlActions = []string{
	MihomoConsumerGrantInvalidate,
	MihomoCredentialBind,
	MihomoCredentialDelete,
	MihomoCredentialRefresh,
	MihomoCredentialUpdate,
	MihomoProfileWrite,
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
