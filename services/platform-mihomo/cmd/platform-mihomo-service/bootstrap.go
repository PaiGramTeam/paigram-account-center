package main

import (
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"platform-mihomo-service/internal/conf"
	"platform-mihomo-service/internal/data"
	"platform-mihomo-service/internal/server"
	"platform-mihomo-service/internal/service"
	"platform-mihomo-service/internal/usecase"
)

type productionComponents struct {
	controlService        *service.PlatformControlService
	runtimeService        *service.MihomoRuntimeService
	artifactCleanupServer *server.ArtifactCleanupServer
}

func buildProductionComponents(bc *conf.Bootstrap, database *gorm.DB, redisClient *redis.Client) (*productionComponents, error) {
	credentialRepo := data.NewCredentialRepo(database)
	deviceRepo := data.NewDeviceRepo(database)
	profileRepo := data.NewProfileRepo(database)
	redisPrefix := bc.GetData().GetRedis().GetPrefix()
	artifactRepo := data.NewArtifactRepo(database, redisClient, redisPrefix)
	encryptionKey := []byte(bc.GetSecurity().GetCredentialEncryptionKey())
	managementRepo := data.NewManagementRepo(database, redisClient, redisPrefix)
	grantInvalidationRepo := data.NewGrantInvalidationRepo(database)
	operationRepo := data.NewOperationRepo(database)
	authorizationFenceRepo := data.NewAuthorizationFenceRepo(database)
	client, err := newMihomoUpstreamClient(bc.GetUpstream())
	if err != nil {
		return nil, err
	}
	artifactLifecycle := usecase.NewArtifactLifecycle(artifactRepo, usecase.ArtifactLifecycleConfig{
		Revoker: client, EncryptionKey: encryptionKey,
	})
	ticketVerifier, err := newTicketVerifierFromSecurity(bc.GetSecurity())
	if err != nil {
		return nil, err
	}
	ticketVerifier.WithGrantVersionLookup(grantInvalidationRepo).
		WithAuthorizationStateLookup(data.NewTicketAuthorizationStateLookup(database))

	bindUC := usecase.NewBindUsecase(credentialRepo, deviceRepo, profileRepo, client, encryptionKey, artifactRepo)
	statusUC := usecase.NewStatusUsecase(credentialRepo, profileRepo, client, encryptionKey, artifactLifecycle)
	profileUC := usecase.NewProfileUsecase(profileRepo)
	authkeyUC := usecase.NewAuthkeyUsecase(credentialRepo, artifactRepo, artifactLifecycle, client, encryptionKey)
	managementUC := usecase.NewManagementUsecase(
		credentialRepo, deviceRepo, profileRepo, artifactRepo, managementRepo, bindUC, profileUC,
	)
	controlService := service.NewPlatformControlService(
		ticketVerifier,
		usecase.NewOperationUsecase(operationRepo),
		bindUC,
		statusUC,
		profileUC,
		managementUC,
		credentialRepo,
		authorizationFenceRepo,
		grantInvalidationRepo,
		artifactLifecycle,
	)
	return &productionComponents{
		controlService:        controlService,
		runtimeService:        service.NewMihomoRuntimeService(ticketVerifier, statusUC, profileUC, authkeyUC, managementUC, deviceRepo),
		artifactCleanupServer: server.NewArtifactCleanupServer(artifactLifecycle, 5*time.Minute),
	}, nil
}
