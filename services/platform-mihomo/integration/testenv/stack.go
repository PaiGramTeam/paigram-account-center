//go:build integration

package testenv

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	initmigrate "platform-mihomo-service/initialize/migrate"
)

const (
	defaultPostgresImage = "public.ecr.aws/docker/library/postgres:16-alpine"
	defaultRedisImage    = "public.ecr.aws/docker/library/redis:7-alpine"
	postgresUser         = "platform_mihomo_it"
	postgresPass         = "platform_mihomo_it_password"
	baselineDBName       = "platform_mihomo_it_baseline"
	resourceTimeout      = 2 * time.Minute
)

var identifierSanitizer = regexp.MustCompile(`[^a-z0-9_]+`)

type SharedStack struct {
	PostgresAdminDSN string
	BaselineDSN      string
	RedisAddr        string

	postgres *tcpostgres.PostgresContainer
	redis    *tcredis.RedisContainer
	provider testcontainers.ProviderType
}

type PerTestDB struct {
	Name string
	DSN  string
	SQL  *sql.DB
	GORM *gorm.DB
}

var (
	shared  *SharedStack
	sharedM sync.RWMutex
	dbSeq   atomic.Uint64
)

func Bootstrap(ctx context.Context) (*SharedStack, error) {
	sharedM.Lock()
	defer sharedM.Unlock()
	if shared != nil {
		return nil, errors.New("integration stack is already running")
	}
	if os.Getenv("TESTCONTAINERS_RYUK_DISABLED") == "" {
		_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	}

	provider, err := resolveProvider(os.Getenv("PAI_TESTCONTAINERS_PROVIDER"))
	if err != nil {
		return nil, err
	}
	stack := &SharedStack{provider: provider}
	type result struct {
		name string
		err  error
	}
	results := make(chan result, 2)
	go func() { results <- result{name: "postgres", err: stack.startPostgres(ctx)} }()
	go func() { results <- result{name: "redis", err: stack.startRedis(ctx)} }()

	var startupErr error
	for range 2 {
		result := <-results
		if result.err != nil {
			startupErr = errors.Join(startupErr, fmt.Errorf("start %s: %w", result.name, result.err))
		}
	}
	if startupErr != nil {
		return nil, errors.Join(startupErr, stack.teardown(ctx))
	}
	if err := stack.migrateBaseline(ctx); err != nil {
		return nil, errors.Join(err, stack.teardown(ctx))
	}
	shared = stack
	return stack, nil
}

func MustShared() *SharedStack {
	sharedM.RLock()
	defer sharedM.RUnlock()
	if shared == nil {
		panic("integration stack is not running")
	}
	return shared
}

func Teardown(ctx context.Context) error {
	sharedM.Lock()
	defer sharedM.Unlock()
	if shared == nil {
		return nil
	}
	err := shared.teardown(ctx)
	shared = nil
	return err
}

func (s *SharedStack) startPostgres(ctx context.Context) error {
	container, err := tcpostgres.Run(ctx, envOrDefault("PAI_INTEGRATION_POSTGRES_IMAGE", defaultPostgresImage),
		providerCustomizer(s.provider),
		tcpostgres.WithDatabase(baselineDBName),
		tcpostgres.WithUsername(postgresUser),
		tcpostgres.WithPassword(postgresPass),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		return err
	}
	s.postgres = container
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return err
	}
	s.BaselineDSN, err = replaceDatabase(dsn, baselineDBName)
	if err != nil {
		return err
	}
	s.PostgresAdminDSN, err = replaceDatabase(s.BaselineDSN, "postgres")
	return err
}

func (s *SharedStack) startRedis(ctx context.Context) error {
	container, err := tcredis.Run(ctx, envOrDefault("PAI_INTEGRATION_REDIS_IMAGE", defaultRedisImage), providerCustomizer(s.provider))
	if err != nil {
		return err
	}
	s.redis = container
	connectionString, err := container.ConnectionString(ctx)
	if err != nil {
		return err
	}
	parsed, err := url.Parse(connectionString)
	if err != nil {
		return fmt.Errorf("parse Redis connection string: %w", err)
	}
	s.RedisAddr = normalizeEndpoint(parsed.Host)
	return nil
}

func (s *SharedStack) migrateBaseline(ctx context.Context) error {
	db, err := sql.Open("pgx", s.BaselineDSN)
	if err != nil {
		return fmt.Errorf("open baseline database: %w", err)
	}
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := db.PingContext(probeCtx); err != nil {
		_ = db.Close()
		return fmt.Errorf("probe baseline database: %w", err)
	}
	if err := initmigrate.Run(db); err != nil {
		_ = db.Close()
		return fmt.Errorf("migrate baseline database: %w", err)
	}
	_ = db.Close()
	return nil
}

func (s *SharedStack) teardown(context.Context) error {
	var result error
	if s.redis != nil {
		result = errors.Join(result, testcontainers.TerminateContainer(s.redis))
	}
	if s.postgres != nil {
		result = errors.Join(result, testcontainers.TerminateContainer(s.postgres))
	}
	return result
}

func NewPerTestDB(t *testing.T) *PerTestDB {
	t.Helper()
	return newPerTestDB(t, baselineDBName)
}

func NewEmptyPerTestDB(t *testing.T) *PerTestDB {
	t.Helper()
	return newPerTestDB(t, "template0")
}

func newPerTestDB(t *testing.T, template string) *PerTestDB {
	t.Helper()
	stack := MustShared()
	ctx, cancel := context.WithTimeout(context.Background(), resourceTimeout)
	defer cancel()

	name := databaseName(t.Name(), dbSeq.Add(1))
	admin, err := sql.Open("pgx", stack.PostgresAdminDSN)
	if err != nil {
		t.Fatalf("open PostgreSQL admin connection: %v", err)
	}
	if _, err := admin.ExecContext(ctx, fmt.Sprintf(
		"CREATE DATABASE %s TEMPLATE %s OWNER %s",
		quoteIdentifier(name), quoteIdentifier(template), quoteIdentifier(postgresUser),
	)); err != nil {
		_ = admin.Close()
		t.Fatalf("create per-test PostgreSQL database: %v", err)
	}
	_ = admin.Close()

	dsn, err := replaceDatabase(stack.BaselineDSN, name)
	if err != nil {
		t.Fatalf("build per-test PostgreSQL DSN: %v", err)
	}
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open per-test PostgreSQL database: %v", err)
	}
	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("open per-test PostgreSQL GORM client: %v", err)
	}
	gormPool, err := gormDB.DB()
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("resolve per-test PostgreSQL GORM pool: %v", err)
	}

	t.Cleanup(func() {
		_ = gormPool.Close()
		_ = sqlDB.Close()
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer dropCancel()
		admin, openErr := sql.Open("pgx", stack.PostgresAdminDSN)
		if openErr != nil {
			t.Errorf("open PostgreSQL admin connection for cleanup: %v", openErr)
			return
		}
		defer admin.Close()
		if _, dropErr := admin.ExecContext(dropCtx, "DROP DATABASE IF EXISTS "+quoteIdentifier(name)+" WITH (FORCE)"); dropErr != nil {
			t.Errorf("drop per-test PostgreSQL database: %v", dropErr)
		}
	})
	return &PerTestDB{Name: name, DSN: dsn, SQL: sqlDB, GORM: gormDB}
}

func replaceDatabase(dsn, database string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	parsed.Path = "/" + database
	parsed.Host = normalizeEndpoint(parsed.Host)
	return parsed.String(), nil
}

func providerCustomizer(provider testcontainers.ProviderType) testcontainers.ContainerCustomizer {
	return testcontainers.CustomizeRequest(testcontainers.GenericContainerRequest{
		ProviderType: provider,
	})
}

func resolveProvider(value string) (testcontainers.ProviderType, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "podman":
		return testcontainers.ProviderPodman, nil
	case "auto", "default":
		return testcontainers.ProviderDefault, nil
	case "docker":
		return testcontainers.ProviderDocker, nil
	default:
		return 0, fmt.Errorf("unsupported PAI_TESTCONTAINERS_PROVIDER %q", value)
	}
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func normalizeEndpoint(endpoint string) string {
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil || !strings.EqualFold(host, "localhost") {
		return endpoint
	}
	return net.JoinHostPort("127.0.0.1", port)
}

func databaseName(testName string, sequence uint64) string {
	name := strings.ToLower(testName)
	name = identifierSanitizer.ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")
	if len(name) > 45 {
		name = name[:45]
	}
	return fmt.Sprintf("it_%s_%d", name, sequence)
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
