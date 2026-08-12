package platformaction

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMihomoActionCatalogSeparatesDelegationAndControlActions(t *testing.T) {
	require.Equal(t, []string{
		MihomoAuthKeyIssue,
		MihomoCredentialValidate,
		MihomoDeviceRead,
		MihomoProfileRead,
		MihomoStatusRead,
	}, MihomoDelegationActions())
	require.Equal(t, []string{
		MihomoAuthorizationFenceApply,
		MihomoBindingRead,
		MihomoCredentialBind,
		MihomoCredentialDelete,
		MihomoCredentialRefresh,
		MihomoCredentialUpdate,
		MihomoOperationRead,
		MihomoOperationResolve,
	}, MihomoControlActions())

	for _, action := range MihomoDelegationActions() {
		require.True(t, IsMihomoDelegationAction(action), action)
		require.False(t, IsMihomoControlAction(action), action)
	}
	for _, action := range MihomoControlActions() {
		require.True(t, IsMihomoControlAction(action), action)
		require.False(t, IsMihomoDelegationAction(action), action)
	}
	require.False(t, IsMihomoDelegationAction("mihomo.unknown"))
	require.False(t, IsMihomoControlAction("mihomo.unknown"))
}

func TestMihomoActionCatalogReturnsCopies(t *testing.T) {
	actions := MihomoDelegationActions()
	actions[0] = "modified"
	require.Equal(t, MihomoAuthKeyIssue, MihomoDelegationActions()[0])
}
