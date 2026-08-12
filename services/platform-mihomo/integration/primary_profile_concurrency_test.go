//go:build integration

package integration

import (
	"context"
	"sync"
	"testing"

	mihomov2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/mihomo/v2"
	platformv2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v2"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/platformaction"
	contractticket "github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/serviceticket"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestConcurrentPrimaryProfileSwitchesUseRevisionCAS(t *testing.T) {
	stack := newIntegrationStack(t)
	control, runtime := newV2ClientsForTest(t, stack)
	bindOperation := operationRef("concurrent-primary-bind", platformv2.OperationKind_OPERATION_KIND_BIND_CREDENTIAL, 0, 1)
	bound, err := control.BindCredential(
		ticketContextForOperation(t, "", platformaction.MihomoCredentialBind, contractticket.TypeControl, bindOperation.GetOperationId(), 0),
		&platformv2.BindCredentialRequest{
			Operation:             bindOperation,
			CredentialPayloadJson: `{"cookie_bundle":"{\"account_id\":\"10001\",\"cookie_token\":\"abc\"}","device_id":"device-concurrent","device_fp":"fingerprint-concurrent"}`,
		},
	)
	require.NoError(t, err)
	accountKey := bound.GetResult().GetAccountKey()
	profiles, err := runtime.ListProfiles(
		ticketContext(t, accountKey, platformaction.MihomoProfileRead),
		&mihomov2.ListProfilesRequest{Resource: bindingResource(accountKey)},
	)
	require.NoError(t, err)
	require.Len(t, profiles.GetSnapshot().GetProfiles(), 2)

	refs := []string{profiles.GetSnapshot().GetProfiles()[0].GetProfileRef(), profiles.GetSnapshot().GetProfiles()[1].GetProfileRef()}
	operations := []*platformv2.OperationRef{
		primaryProfileOperationRef("concurrent-primary-one", 1, refs[0], 1),
		primaryProfileOperationRef("concurrent-primary-two", 1, refs[1], 1),
	}
	contexts := []context.Context{
		ticketContextForProfileOperation(t, accountKey, platformaction.MihomoProfileWrite, operations[0].GetOperationId(), 1, refs[0]),
		ticketContextForProfileOperation(t, accountKey, platformaction.MihomoProfileWrite, operations[1].GetOperationId(), 1, refs[1]),
	}

	start := make(chan struct{})
	errorsByIndex := make([]error, 2)
	var wait sync.WaitGroup
	for index := range operations {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			_, errorsByIndex[index] = control.SetPrimaryProfile(contexts[index], &platformv2.SetPrimaryProfileRequest{
				Operation: operations[index], AccountKey: accountKey, ProfileRef: refs[index], ExpectedProfileRevision: 1,
			})
		}(index)
	}
	close(start)
	wait.Wait()

	successes := 0
	aborted := 0
	for _, callErr := range errorsByIndex {
		if callErr == nil {
			successes++
		} else if status.Code(callErr) == codes.Aborted {
			aborted++
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, aborted)

	primary, err := runtime.GetPrimaryProfile(
		ticketContextForDelegationGeneration(t, accountKey, platformaction.MihomoProfileRead, 1),
		&mihomov2.GetPrimaryProfileRequest{Resource: bindingResource(accountKey)},
	)
	require.NoError(t, err)
	require.Contains(t, refs, primary.GetProfile().GetProfileRef())
	latest, err := runtime.ListProfiles(
		ticketContextForDelegationGeneration(t, accountKey, platformaction.MihomoProfileRead, 1),
		&mihomov2.ListProfilesRequest{Resource: bindingResource(accountKey)},
	)
	require.NoError(t, err)
	require.Equal(t, uint64(2), latest.GetSnapshot().GetRevision())
	require.Equal(t, uint64(2), latest.GetSnapshot().GetObservedRevision())
}
