package service

import (
	"testing"

	contractticket "github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/serviceticket"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"platform-mihomo-service/internal/biz"
)

func TestTicketTypesAreExclusiveAcrossPlatformServices(t *testing.T) {
	control := &biz.ServiceTicketClaims{TicketType: contractticket.TypeControl}
	delegation := &biz.ServiceTicketClaims{TicketType: contractticket.TypeDelegation}

	require.NoError(t, requireControlTicket(control))
	require.NoError(t, requireDelegationTicket(delegation))
	require.Equal(t, codes.PermissionDenied, status.Code(requireControlTicket(delegation)))
	require.Equal(t, codes.PermissionDenied, status.Code(requireDelegationTicket(control)))
	require.Equal(t, codes.PermissionDenied, status.Code(requireControlTicket(nil)))
	require.Equal(t, codes.PermissionDenied, status.Code(requireDelegationTicket(nil)))
}
