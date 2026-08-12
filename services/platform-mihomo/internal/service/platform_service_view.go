package service

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func toTimestamp(ts *time.Time) *timestamppb.Timestamp {
	if ts == nil {
		return nil
	}
	return timestamppb.New(*ts)
}
