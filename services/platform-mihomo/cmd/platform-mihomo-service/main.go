package main

import (
	"errors"
	"flag"
	"log"
	"time"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/secretfile"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/transporttls"
	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/env"
	"github.com/go-kratos/kratos/v2/config/file"
	_ "github.com/go-kratos/kratos/v2/encoding/yaml"
	"github.com/redis/go-redis/v9"

	"platform-mihomo-service/internal/conf"
	internalcrypto "platform-mihomo-service/internal/crypto"
	"platform-mihomo-service/internal/data"
	internaldatabase "platform-mihomo-service/internal/database"
	platformmihomo "platform-mihomo-service/internal/platform/mihomo"
	"platform-mihomo-service/internal/server"
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
	if err := loadBootstrapSecretFiles(&bc); err != nil {
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

	components, err := buildProductionComponents(&bc, database, redisClient)
	if err != nil {
		log.Fatal(err)
	}

	grpcServers, err := server.NewGRPCServers(&bc, components.controlService, components.runtimeService)
	if err != nil {
		log.Fatal(err)
	}
	app := kratos.New(
		kratos.Name("platform-mihomo-service"),
		kratos.Server(grpcServers.Control, grpcServers.Runtime, components.artifactCleanupServer, components.credentialReencryptionServer),
	)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func loadBootstrapSecretFiles(bc *conf.Bootstrap) error {
	if bc == nil || bc.GetData() == nil {
		return errors.New("data configuration is required")
	}
	targets := []struct {
		name string
		path string
		set  func(string)
	}{
		{name: "data.database.dsn_file", path: bc.GetData().GetDatabase().GetDsnFile(), set: func(value string) { bc.Data.Database.Dsn = value }},
		{name: "data.redis.password_file", path: bc.GetData().GetRedis().GetPasswordFile(), set: func(value string) { bc.Data.Redis.Password = value }},
	}
	for _, target := range targets {
		if target.path == "" {
			continue
		}
		value, err := secretfile.Read(target.path)
		if err != nil {
			return errors.Join(errors.New(target.name), err)
		}
		target.set(value)
	}
	return nil
}

func validateBootstrap(bc *conf.Bootstrap) error {
	if err := validateGRPCBootstrap("server.control", bc.GetServer().GetControl(), transporttls.MutualTLS); err != nil {
		return err
	}
	if err := validateGRPCBootstrap("server.runtime", bc.GetServer().GetRuntime(), transporttls.ServerAuthOnly); err != nil {
		return err
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
	if security.GetCredentialEncryptionKeyringFile() == "" {
		return errors.New("security.credential_encryption_keyring_file is required")
	}
	if _, err := internalcrypto.NewFileKeyring(security.GetCredentialEncryptionKeyringFile()); err != nil {
		return err
	}
	if security.GetServiceTicketPublicKeyringFile() == "" {
		return errors.New("security.service_ticket_public_keyring_file is required")
	}
	if _, err := data.NewFilePublicKeyResolver(security.GetServiceTicketPublicKeyringFile()); err != nil {
		return err
	}

	return nil
}

func validateGRPCBootstrap(name string, grpcConf *conf.Server_GRPC, mode transporttls.ServerMode) error {
	if grpcConf.GetNetwork() == "" {
		return errors.New(name + ".network is required")
	}
	if grpcConf.GetAddr() == "" {
		return errors.New(name + ".addr is required")
	}
	if grpcConf.GetTimeoutSeconds() <= 0 {
		return errors.New(name + ".timeout_seconds must be greater than zero")
	}
	tlsFiles := grpcConf.GetTls()
	_, err := transporttls.NewServerConfig(transporttls.ServerFiles{
		CertificateFile: tlsFiles.GetCertificateFile(),
		PrivateKeyFile:  tlsFiles.GetPrivateKeyFile(),
		ClientCAFile:    tlsFiles.GetClientCaFile(),
	}, mode)
	return err
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
	resolver, err := data.NewFilePublicKeyResolver(security.GetServiceTicketPublicKeyringFile())
	if err != nil {
		return nil, err
	}
	return data.NewTicketVerifierWithResolver(security.GetServiceTicketIssuer(), resolver), nil
}
