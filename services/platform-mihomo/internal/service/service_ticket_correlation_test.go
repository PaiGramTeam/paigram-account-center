package service

import (
	"context"
	"testing"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/correlation"
	"github.com/stretchr/testify/assert"

	"platform-mihomo-service/internal/biz"
)

func TestVerifiedServiceTicketOperationReplacesUntrustedCorrelation(t *testing.T) {
	ctx := correlation.Ensure(context.Background(), correlation.Fields{OperationID: "untrusted-operation"})
	claims := &biz.ServiceTicketClaims{OperationID: "verified-operation"}

	ctx = contextWithVerifiedServiceTicketClaims(ctx, claims)

	assert.Equal(t, "verified-operation", correlation.FromContext(ctx).OperationID)
	assert.Same(t, claims, ctx.Value(verifiedServiceTicketClaimsKey{}))
}

func TestVerifiedServiceTicketWithoutOperationClearsUntrustedCorrelation(t *testing.T) {
	ctx := correlation.Ensure(context.Background(), correlation.Fields{OperationID: "untrusted-operation"})

	ctx = contextWithVerifiedServiceTicketClaims(ctx, &biz.ServiceTicketClaims{})

	assert.Empty(t, correlation.FromContext(ctx).OperationID)
}
