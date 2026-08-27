package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/secretfile"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/transporttls"
	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/env"
	"github.com/go-kratos/kratos/v2/config/file"
	_ "github.com/go-kratos/kratos/v2/encoding/yaml"
	"github.com/redis/go-redis/v9"

	initmigrate "platform-mihomo-service/initialize/migrate"
	"platform-mihomo-service/internal/conf"
	internalcrypto "platform-mihomo-service/internal/crypto"
	"platform-mihomo-service/internal/data"
	internaldatabase "platform-mihomo-service/internal/database"
	"platform-mihomo-service/internal/healthcheck"
	platformmihomo "platform-mihomo-service/internal/platform/mihomo"
	"platform-mihomo-service/internal/server"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "conf", "configs/config.yaml", "config path")
	flag.Parse()
	command, err := parseCommand(flag.Args())
	if err != nil {
		log.Print(err)
		os.Exit(2)
	}

	c := config.New(config.WithSource(file.NewSource(configPath), env.NewSource("PAI_")))
	defer c.Close()

	if err := c.Load(); err != nil {
		log.Fatal(err)
	}

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		log.Fatal(err)
	}
	if err := loadDatabaseSecretFile(&bc); err != nil {
		log.Fatal(err)
	}
	databaseConfig := bc.GetData().GetDatabase()
	if databaseConfig.GetDsn() == "" {
		log.Fatal("data.database.dsn is required")
	}
	if command == "migrate" {
		if err := runMigration(databaseConfig.GetDsn()); err != nil {
			log.Fatal(err)
		}
		return
	}
	database, err := internaldatabase.Connect(internaldatabase.Config{DSN: databaseConfig.GetDsn()})
	if err != nil {
		log.Fatal(err)
	}
	sqlDatabase, err := database.DB()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := sqlDatabase.Close(); err != nil {
			log.Printf("close database: %v", err)
		}
	}()
	if err := loadRuntimeSecretFiles(&bc); err != nil {
		log.Fatal(err)
	}
	if err := validateBootstrap(&bc); err != nil {
		log.Fatal(err)
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr:     bc.GetData().GetRedis().GetAddr(),
		Password: bc.GetData().GetRedis().GetPassword(),
		DB:       int(bc.GetData().GetRedis().GetDb()),
	})
	defer func() {
		if err := redisClient.Close(); err != nil {
			log.Printf("close redis: %v", err)
		}
	}()

	components, err := buildProductionComponents(&bc, database, redisClient)
	if err != nil {
		log.Fatal(err)
	}

	readiness := healthcheck.NewReadiness(database, redisClient)
	grpcServers, err := server.NewGRPCServersWithReadiness(&bc, components.controlService, components.runtimeService, readiness)
	if err != nil {
		log.Fatal(err)
	}
	metricsServer := server.NewMetricsServer(bc.GetMetrics().GetAddr(), components.metrics.Handler())
	app := kratos.New(
		kratos.Name("platform-mihomo-service"),
		kratos.Server(grpcServers.Control, grpcServers.Runtime, metricsServer, components.artifactCleanupServer, components.credentialReencryptionServer),
		kratos.AfterStart(func(context.Context) error {
			grpcServers.Health.Start()
			return nil
		}),
		kratos.BeforeStop(func(context.Context) error {
			grpcServers.Health.Shutdown()
			return nil
		}),
	)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func runMigration(dsn string) (err error) {
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open migration database: %w", err)
	}
	defer func() {
		if closeErr := database.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close migration database: %w", closeErr)
		}
	}()
	if err := initmigrate.Run(database); err != nil {
		return err
	}
	return nil
}

func parseCommand(arguments []string) (string, error) {
	if len(arguments) == 0 {
		return "serve", nil
	}
	if len(arguments) != 1 || (arguments[0] != "serve" && arguments[0] != "migrate") {
		return "", fmt.Errorf("unsupported command %q", strings.Join(arguments, " "))
	}
	return arguments[0], nil
}

func loadDatabaseSecretFile(bc *conf.Bootstrap) error {
	if bc == nil || bc.GetData() == nil {
		return errors.New("data configuration is required")
	}
	database := bc.GetData().GetDatabase()
	if database.GetDsnFile() == "" {
		return nil
	}
	value, err := secretfile.Read(database.GetDsnFile())
	if err != nil {
		return errors.Join(errors.New("data.database.dsn_file"), err)
	}
	bc.Data.Database.Dsn = value
	return nil
}

func loadRuntimeSecretFiles(bc *conf.Bootstrap) error {
	if bc == nil || bc.GetData() == nil {
		return errors.New("data configuration is required")
	}
	targets := []struct {
		name string
		path string
		set  func(string)
	}{
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
	if err := validateGRPCBootstrap("server.control", bc.GetServer().GetControl()); err != nil {
		return err
	}
	if err := validateGRPCBootstrap("server.runtime", bc.GetServer().GetRuntime()); err != nil {
		return err
	}
	databaseConf := bc.GetData().GetDatabase()
	if databaseConf.GetDsn() == "" {
		return errors.New("data.database.dsn is required")
	}
	if bc.GetData().GetRedis().GetAddr() == "" {
		return errors.New("data.redis.addr is required")
	}
	if bc.GetMetrics().GetAddr() == "" {
		return errors.New("metrics.addr is required")
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

func validateGRPCBootstrap(name string, grpcConf *conf.Server_GRPC) error {
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
	_, err := transporttls.NewOptionalServerConfig(transporttls.ServerFiles{
		CertificateFile: tlsFiles.GetCertificateFile(),
		PrivateKeyFile:  tlsFiles.GetPrivateKeyFile(),
		ClientCAFile:    tlsFiles.GetClientCaFile(),
	})
	return err
}

func newMihomoUpstreamClient(upstream *conf.Upstream) (*platformmihomo.HTTPClient, error) {
	return platformmihomo.NewHTTPClient(platformmihomo.HTTPClientConfig{
		BaseURL:           upstream.GetBaseUrl(),
		Timeout:           time.Duration(upstream.GetTimeoutSeconds()) * time.Second,
		BearerTokenFile:   upstream.GetBearerTokenFile(),
		RootCAFile:        upstream.GetRootCaFile(),
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
