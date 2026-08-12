package platform

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type dialFunc func(ctx context.Context, endpoint string) (*grpc.ClientConn, error)

func formatProtoTime(value *timestamppb.Timestamp) any {
	if value == nil {
		return nil
	}

	return value.AsTime().UTC().Format(time.RFC3339)
}
