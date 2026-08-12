package clientauth

import (
	"context"

	"google.golang.org/grpc/metadata"
)

func WithServiceTicket(ctx context.Context, ticket string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+ticket)
}
