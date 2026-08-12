package service

import (
	"context"
	"testing"

	pb "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/account/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"

	"paigram/internal/grpc/interceptor"
	"paigram/internal/model"
	"paigram/internal/service/credentials"
)

func TestGetPlatformRuntimeRoute(t *testing.T) {
	platform := model.PlatformService{
		PlatformKey:          "mihomo",
		DisplayName:          "Mihomo",
		ServiceKey:           "platform-mihomo-service",
		ServiceAudience:      "platform-mihomo-runtime",
		DiscoveryType:        "static",
		ControlEndpoint:      "control.internal:50051",
		RuntimeEndpoint:      "runtime.internal:50052",
		RuntimeServerName:    "runtime.internal",
		Enabled:              true,
		SupportedActionsJSON: `["mihomo.status.read"]`,
		CredentialSchemaJSON: `{}`,
	}
	service := NewBotAccessService(nil, nil, nil)
	service.runtimeRouteLookup = func(serviceKey string) (model.PlatformService, error) {
		if serviceKey != platform.ServiceKey || !platform.Enabled {
			return model.PlatformService{}, gorm.ErrRecordNotFound
		}
		return platform, nil
	}
	authenticated := interceptor.WithCredentialClaims(context.Background(), &credentials.AccessClaims{
		ClientID: "paigram-bot",
		BotID:    "paigram-bot",
		Scope:    "bot.access.read",
	})

	t.Run("requires machine authentication", func(t *testing.T) {
		_, routeErr := service.GetPlatformRuntimeRoute(context.Background(), &pb.GetPlatformRuntimeRouteRequest{
			PlatformServiceKey: "platform-mihomo-service",
		})
		assert.Equal(t, codes.Unauthenticated, status.Code(routeErr))
	})

	t.Run("returns only enabled complete runtime routes", func(t *testing.T) {
		route, routeErr := service.GetPlatformRuntimeRoute(authenticated, &pb.GetPlatformRuntimeRouteRequest{
			PlatformServiceKey: "platform-mihomo-service",
		})
		require.NoError(t, routeErr)
		assert.Equal(t, "mihomo", route.PlatformKey)
		assert.Equal(t, "runtime.internal:50052", route.RuntimeEndpoint)
		assert.Equal(t, "runtime.internal", route.RuntimeServerName)
		assert.Equal(t, "platform-mihomo-runtime", route.ServiceAudience)
		assert.Equal(t, []string{"mihomo.status.read"}, route.SupportedActions)
	})

	t.Run("hides disabled services", func(t *testing.T) {
		platform.Enabled = false
		_, routeErr := service.GetPlatformRuntimeRoute(authenticated, &pb.GetPlatformRuntimeRouteRequest{
			PlatformServiceKey: "platform-mihomo-service",
		})
		assert.Equal(t, codes.NotFound, status.Code(routeErr))
	})
}
