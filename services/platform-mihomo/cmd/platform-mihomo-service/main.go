package main

import (
	"errors"
	"flag"
	"log"
	"time"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/env"
	"github.com/go-kratos/kratos/v2/config/file"
	_ "github.com/go-kratos/kratos/v2/encoding/yaml"
	"github.com/redis/go-redis/v9"

	"platform-mihomo-service/internal/conf"
	"platform-mihomo-service/internal/data"
	internaldatabase "platform-mihomo-service/internal/database"
	platformmihomo "platform-mihomo-service/internal/platform/mihomo"
	"platform-mihomo-service/internal/server"
	"platform-mihomo-service/internal/service"
	"platform-mihomo-service/internal/usecase"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "conf", "configs/config.yaml", "config path")
	flag.Parse()

	c := config.New(config.WithSource(file.NewSource(configPath), env.NewSource("PAI_")))
	defer c.Close()

	if err := c.Load(); err != nil {
		log.Fatal(err)
	}

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		log.Fatal(err)
	}
	if err := validateBootstrap(&bc); err != nil {
		log.Fatal(err)
	}

	databaseConfig := bc.GetData().GetDatabase()
	database, err := internaldatabase.Connect(internaldatabase.Config{DSN: databaseConfig.GetDsn()})
	if err != nil {
		log.Fatal(err)
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr:     bc.GetData().GetRedis().GetAddr(),
		Password: bc.GetData().GetRedis().GetPassword(),
		DB:       int(bc.GetData().GetRedis().GetDb()),
	})

	credentialRepo := data.NewCredentialRepo(database)
	deviceRepo := data.NewDeviceRepo(database)
	profileRepo := data.NewProfileRepo(database)
	artifactRepo := data.NewArtifactRepo(database, redisClient, bc.GetData().GetRedis().GetPrefix())
	managementRepo := data.NewManagementRepo(database, redisClient, bc.GetData().GetRedis().GetPrefix())
	grantInvalidationRepo := data.NewGrantInvalidationRepo(database)
	client, err := newMihomoUpstreamClient(bc.GetUpstream())
	if err != nil {
		log.Fatal(err)
	}
	ticketVerifier, err := newTicketVerifierFromSecurity(bc.GetSecurity())
	if err != nil {
		log.Fatal(err)
	}
	ticketVerifier.WithGrantVersionLookup(grantInvalidationRepo)

	bindUC := usecase.NewBindUsecase(credentialRepo, deviceRepo, profileRepo, client, []byte(bc.GetSecurity().GetCredentialEncryptionKey()), artifactRepo)
	statusUC := usecase.NewStatusUsecase(credentialRepo, client, []byte(bc.GetSecurity().GetCredentialEncryptionKey()))
	profileUC := usecase.NewProfileUsecase(profileRepo)
	authkeyUC := usecase.NewAuthkeyUsecase(credentialRepo, artifactRepo, client, []byte(bc.GetSecurity().GetCredentialEncryptionKey()))
	managementUC := usecase.NewManagementUsecase(credentialRepo, deviceRepo, profileRepo, artifactRepo, managementRepo, bindUC, profileUC)
	genericSvc := service.NewGenericPlatformService(ticketVerifier, bindUC, statusUC, managementUC, grantInvalidationRepo).
		WithConsumerUsecases(profileUC, authkeyUC)

	grpcSrv := server.NewGRPCServer(&bc, genericSvc)
	app := kratos.New(
		kratos.Name("platform-mihomo-service"),
		kratos.Server(grpcSrv),
	)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func validateBootstrap(bc *conf.Bootstrap) error {
	grpcConf := bc.GetServer().GetGrpc()
	if grpcConf.GetNetwork() == "" {
		return errors.New("server.grpc.network is required")
	}
	if grpcConf.GetAddr() == "" {
		return errors.New("server.grpc.addr is required")
	}
	if grpcConf.GetTimeoutSeconds() <= 0 {
		return errors.New("server.grpc.timeout_seconds must be greater than zero")
	}
	databaseConf := bc.GetData().GetDatabase()
	if databaseConf.GetDsn() == "" {
		return errors.New("data.database.dsn is required")
	}
	upstream := bc.GetUpstream()
	if upstream.GetBaseUrl() == "" {
		return errors.New("upstream.base_url is required")
	}
	if upstream.GetTimeoutSeconds() <= 0 {
		return errors.New("upstream.timeout_seconds must be greater than zero")
	}

	security := bc.GetSecurity()
	if security.GetServiceTicketIssuer() == "" {
		return errors.New("security.service_ticket_issuer is required")
	}
	if len(security.GetCredentialEncryptionKey()) != 32 {
		return errors.New("security.credential_encryption_key must be 32 bytes")
	}
	if security.GetServiceTicketKeyId() == "" {
		return errors.New("security.service_ticket_key_id is required")
	}
	if security.GetServiceTicketPublicKeyPem() == "" {
		return errors.New("security.service_ticket_public_key_pem is required")
	}
	if _, err := data.ParseEd25519PublicKeyPEM(security.GetServiceTicketPublicKeyPem()); err != nil {
		return err
	}

	return nil
}

func newMihomoUpstreamClient(upstream *conf.Upstream) (*platformmihomo.HTTPClient, error) {
	return platformmihomo.NewHTTPClient(platformmihomo.HTTPClientConfig{
		BaseURL:           upstream.GetBaseUrl(),
		Timeout:           time.Duration(upstream.GetTimeoutSeconds()) * time.Second,
		BearerTokenFile:   upstream.GetBearerTokenFile(),
		AllowInsecureHTTP: upstream.GetAllowInsecureHttp(),
	})
}

func newTicketVerifierFromSecurity(security *conf.Security) (*data.TicketVerifier, error) {
	publicKey, err := data.ParseEd25519PublicKeyPEM(security.GetServiceTicketPublicKeyPem())
	if err != nil {
		return nil, err
	}
	return data.NewStaticKeyTicketVerifier(security.GetServiceTicketIssuer(), security.GetServiceTicketKeyId(), publicKey), nil
}
