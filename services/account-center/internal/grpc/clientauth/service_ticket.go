package clientauth

import (
	"context"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/correlation"
	"google.golang.org/grpc/metadata"
)

func WithServiceTicket(ctx context.Context, ticket string) context.Context {
	ctx = correlation.Ensure(ctx, correlation.Fields{})
	fields := correlation.FromContext(ctx)
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	md.Set("authorization", "Bearer "+ticket)
	md.Set(correlation.RequestIDHeader, fields.RequestID)
	md.Set(correlation.TraceParentHeader, fields.TraceParent)
	if fields.OperationID == "" {
		md.Delete(correlation.OperationIDHeader)
	} else {
		md.Set(correlation.OperationIDHeader, fields.OperationID)
	}
	return metadata.NewOutgoingContext(ctx, md)
}
