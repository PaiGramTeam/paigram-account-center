package data

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"platform-mihomo-service/internal/biz"
)

func TestDeviceRepoSavePersistsBindingRefAndListByBindingRefFilters(t *testing.T) {
	db := newRepoTestDB(t)
	repo := NewDeviceRepo(db)
	now := time.Now().UTC()

	name := "iPhone"
	require.NoError(t, repo.Save(context.Background(), &biz.Device{
		BindingRef: "binding-42",
		AccountKey: "binding_42_10001",
		DeviceID:   "device-1",
		DeviceFP:   "fp-1",
		DeviceName: &name,
		IsValid:    true,
		LastSeenAt: &now,
	}))
	require.NoError(t, repo.Save(context.Background(), &biz.Device{
		BindingRef: "binding-7",
		AccountKey: "binding_7_20002",
		DeviceID:   "device-2",
		DeviceFP:   "fp-2",
		IsValid:    true,
		LastSeenAt: &now,
	}))

	devices, err := repo.ListByBindingRef(context.Background(), "binding-42")
	require.NoError(t, err)
	require.Len(t, devices, 1)
	require.Equal(t, "binding-42", devices[0].BindingRef)
	require.Equal(t, "binding_42_10001", devices[0].AccountKey)
	require.Equal(t, "device-1", devices[0].DeviceID)
	require.Equal(t, "fp-1", devices[0].DeviceFP)
}
