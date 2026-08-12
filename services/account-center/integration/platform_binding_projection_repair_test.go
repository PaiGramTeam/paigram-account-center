//go:build integration

package integration

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	platformv2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"paigram/internal/model"
	serviceplatform "paigram/internal/service/platform"
	"paigram/internal/tasks"
)

func TestPlatformBindingProjectionRepairTaskRepairsStaleProjection(t *testing.T) {
	stack := newIntegrationStack(t)
	ownerID, _, _, _, _ := registerAndLogin(t, stack, fmt.Sprintf("binding-repair-%d@example.com", time.Now().UnixNano()), "OwnerPass123!")
	staleTime := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	binding := model.PlatformAccountBinding{
		OwnerUserID:        ownerID,
		Platform:           "mihomo",
		ExternalAccountKey: sql.NullString{String: "cn:repair-route", Valid: true},
		PlatformServiceKey: "platform-mihomo-service",
		DisplayName:        "Repair Route",
		Status:             model.PlatformAccountBindingStatusRefreshRequired,
		LastSyncedAt:       sql.NullTime{Time: staleTime, Valid: true},
		LastValidatedAt:    sql.NullTime{Time: staleTime, Valid: true},
	}
	require.NoError(t, stack.DB.Create(&binding).Error)
	require.NoError(t, stack.DB.Create(&[]model.PlatformAccountProfile{
		{
			BindingID:          binding.ID,
			PlatformProfileKey: "mihomo:stale",
			GameBiz:            "hk4e_global",
			Region:             "os_asia",
			PlayerUID:          "99999",
			Nickname:           "Stale",
		},
		{
			BindingID:          binding.ID,
			PlatformProfileKey: "mihomo:10001",
			GameBiz:            "hk4e_cn",
			Region:             "cn_gf01",
			PlayerUID:          "10001",
			Nickname:           "Traveler Old",
			IsPrimary:          true,
		},
	}).Error)

	stub := &platformBindingRouteStub{summaryResponse: &routeCredentialSummary{
		PlatformAccountId: "cn:repair-route",
		Status:            platformv2.CredentialStatus_CREDENTIAL_STATUS_ACTIVE,
		LastValidatedAt:   timestamppb.New(time.Date(2026, 4, 20, 12, 34, 56, 0, time.UTC)),
		LastRefreshedAt:   timestamppb.New(time.Date(2026, 4, 20, 12, 35, 56, 0, time.UTC)),
		Profiles: []*routeProfileSummary{{
			Id:                42,
			PlatformAccountId: "cn:repair-route",
			GameBiz:           "hk4e_cn",
			Region:            "cn_gf01",
			PlayerId:          "10001",
			Nickname:          "Traveler Repaired",
			Level:             60,
			IsDefault:         true,
		}},
	}}
	seedEnabledPlatformService(t, stack, startPlatformBindingRouteServer(t, stack, stub))

	platformGroup := serviceplatform.NewServiceGroup(stack.DB)
	require.NoError(t, platformGroup.PlatformService.ConfigureAuth(newTestConfig(t, stack.RedisPrefix).Auth))
	require.NoError(t, platformGroup.PlatformService.ConfigureTransport(stack.ControlDial))
	handler := tasks.NewPlatformBindingProjectionRepairHandler(stack.DB, &platformGroup.PlatformService)
	repairTask, err := tasks.NewPlatformBindingProjectionRepairTask(binding.ID)
	require.NoError(t, err)
	require.NoError(t, handler.ProcessTask(context.Background(), repairTask))

	var repaired model.PlatformAccountBinding
	require.NoError(t, stack.DB.First(&repaired, binding.ID).Error)
	assert.Equal(t, model.PlatformAccountBindingStatusActive, repaired.Status)
	assert.True(t, repaired.LastSyncedAt.Valid)
	assert.True(t, repaired.LastValidatedAt.Valid)
	assert.True(t, repaired.LastSyncedAt.Time.After(staleTime))

	var profiles []model.PlatformAccountProfile
	require.NoError(t, stack.DB.Where("binding_id = ?", binding.ID).Order("id ASC").Find(&profiles).Error)
	require.Len(t, profiles, 1)
	assert.Equal(t, "mihomo:42", profiles[0].PlatformProfileKey)
	assert.Equal(t, "Traveler Repaired", profiles[0].Nickname)
	assert.True(t, profiles[0].IsPrimary)
}
