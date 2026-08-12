//go:build integration

package e2ereal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	dockercontainer "github.com/moby/moby/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"paigram/integration/testenv"
)

const (
	adminEmail    = "admin@paigram.local"
	adminPassword = "AdminPass123!"
	userEmail     = "browser.user@example.test"
	userPassword  = "BrowserPass123!"
)

type Config struct {
	RepositoryRoot string
	StateFile      string
	FrontendImage  string
}

type State struct {
	FrontendURL   string `json:"frontend_url"`
	AdminEmail    string `json:"admin_email"`
	AdminPassword string `json:"admin_password"`
	UserEmail     string `json:"user_email"`
	UserPassword  string `json:"user_password"`
}

func Run(ctx context.Context, cfg Config) (runErr error) {
	if err := validateConfig(cfg); err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	_ = os.Remove(cfg.StateFile)
	temporaryDirectory, err := os.MkdirTemp("", "paigram-e2e-real-")
	if err != nil {
		return fmt.Errorf("create fixture directory: %w", err)
	}
	defer os.RemoveAll(temporaryDirectory)

	accountRoot := filepath.Join(cfg.RepositoryRoot, "services", "account-center")
	platformRoot := filepath.Join(cfg.RepositoryRoot, "services", "platform-mihomo")
	stack, err := testenv.Bootstrap(runCtx, accountRoot)
	if err != nil {
		return fmt.Errorf("start storage dependencies: %w", err)
	}
	defer func() {
		teardownCtx, teardownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer teardownCancel()
		runErr = errors.Join(runErr, testenv.Teardown(teardownCtx))
	}()

	platformDSN, err := createPlatformDatabase(runCtx, stack.PostgresAdminDSN, stack.BaselineDSN)
	if err != nil {
		return err
	}
	if err := registerMihomoPlatform(runCtx, stack.BaselineDSN); err != nil {
		return err
	}
	material, err := writeFixtureMaterial(temporaryDirectory)
	if err != nil {
		return fmt.Errorf("write fixture secrets: %w", err)
	}
	upstream, upstreamURL, err := startUpstream()
	if err != nil {
		return fmt.Errorf("start Mihomo simulator: %w", err)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		runErr = errors.Join(runErr, upstream.Shutdown(shutdownCtx))
	}()

	platformConfig := filepath.Join(temporaryDirectory, "platform.yaml")
	if err := writePlatformConfig(platformConfig, platformDSN, stack.RedisAddr, upstreamURL, material); err != nil {
		return err
	}
	accountBinary := executablePath(temporaryDirectory, "paigram")
	platformBinary := executablePath(temporaryDirectory, "platform-mihomo-service")
	if err := buildBinary(runCtx, accountRoot, accountBinary, "./cmd/paigram"); err != nil {
		return err
	}
	if err := buildBinary(runCtx, platformRoot, platformBinary, "./cmd/platform-mihomo-service"); err != nil {
		return err
	}
	platformProcess, err := startProcess(runCtx, "Platform", platformBinary, platformRoot, []string{"-conf", platformConfig}, nil)
	if err != nil {
		return err
	}
	startupCtx, startupCancel := context.WithTimeout(runCtx, 3*time.Minute)
	defer startupCancel()
	if err := waitTCP(startupCtx, "127.0.0.1:19000", platformProcess); err != nil {
		return err
	}
	if err := waitTCP(startupCtx, "127.0.0.1:19001", platformProcess); err != nil {
		return err
	}

	frontend, frontendURL, err := startFrontend(startupCtx, cfg.FrontendImage)
	if err != nil {
		return err
	}
	defer func() {
		teardownCtx, teardownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer teardownCancel()
		runErr = errors.Join(runErr, frontend.Terminate(teardownCtx))
	}()
	accountWorkdir := filepath.Join(temporaryDirectory, "account")
	accountConfig := filepath.Join(accountWorkdir, "config", "config.yaml")
	if err := writeAccountConfig(accountConfig, cfg.RepositoryRoot, stack.BaselineDSN, stack.RedisAddr, frontendURL, material); err != nil {
		return err
	}
	accountProcess, err := startProcess(runCtx, "Account", accountBinary, accountWorkdir, []string{"serve"}, []string{
		"ADMIN_EMAIL=" + adminEmail,
		"ADMIN_PASSWORD=" + adminPassword,
		"ADMIN_NAME=Browser Administrator",
	})
	if err != nil {
		return err
	}
	if err := waitHTTP(startupCtx, "http://127.0.0.1:8080/readyz", accountProcess); err != nil {
		return err
	}
	if err := waitHTTP(startupCtx, frontendURL+"/readyz", accountProcess); err != nil {
		return err
	}
	state := State{
		FrontendURL: frontendURL, AdminEmail: adminEmail, AdminPassword: adminPassword,
		UserEmail: userEmail, UserPassword: userPassword,
	}
	if err := writeState(cfg.StateFile, state); err != nil {
		return err
	}
	defer os.Remove(cfg.StateFile)

	select {
	case <-runCtx.Done():
		return nil
	case err := <-platformProcess.done:
		return fmt.Errorf("Platform process stopped: %w", normalizeProcessError(err))
	case err := <-accountProcess.done:
		return fmt.Errorf("Account process stopped: %w", normalizeProcessError(err))
	}
}

func startFrontend(ctx context.Context, image string) (testcontainers.Container, string, error) {
	provider, err := containerProvider(os.Getenv("PAI_TESTCONTAINERS_PROVIDER"))
	if err != nil {
		return nil, "", err
	}
	request := testcontainers.ContainerRequest{
		Image:        image,
		ExposedPorts: []string{"8080/tcp"},
		WaitingFor: wait.ForHTTP("/nginx-health").
			WithPort("8080/tcp").
			WithStartupTimeout(2 * time.Minute),
	}
	if provider == testcontainers.ProviderDocker && runtime.GOOS != "windows" {
		request.HostConfigModifier = func(hostConfig *dockercontainer.HostConfig) {
			hostConfig.ExtraHosts = append(hostConfig.ExtraHosts, "account-center:host-gateway")
		}
	} else {
		request.HostAccessPorts = []int{8080}
		request.HostConfigModifier = func(hostConfig *dockercontainer.HostConfig) {
			for _, mapping := range hostConfig.ExtraHosts {
				hostname, address, found := strings.Cut(mapping, ":")
				if found && hostname == testcontainers.HostInternal {
					hostConfig.ExtraHosts = append(hostConfig.ExtraHosts, "account-center:"+address)
					return
				}
			}
		}
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ProviderType:     provider,
		Started:          true,
		ContainerRequest: request,
	})
	if err != nil {
		return nil, "", fmt.Errorf("start production frontend: %w", err)
	}
	endpoint, err := container.PortEndpoint(ctx, "8080/tcp", "http")
	if err != nil {
		_ = container.Terminate(context.Background())
		return nil, "", fmt.Errorf("resolve production frontend endpoint: %w", err)
	}
	return container, strings.TrimSuffix(endpoint, "/"), nil
}

func validateConfig(cfg Config) error {
	if !filepath.IsAbs(cfg.RepositoryRoot) || !filepath.IsAbs(cfg.StateFile) {
		return fmt.Errorf("repository root and state file must be absolute paths")
	}
	if strings.TrimSpace(cfg.FrontendImage) == "" {
		return fmt.Errorf("frontend image is required")
	}
	for _, path := range []string{
		filepath.Join(cfg.RepositoryRoot, "frontend", "package.json"),
		filepath.Join(cfg.RepositoryRoot, "services", "account-center", "go.mod"),
		filepath.Join(cfg.RepositoryRoot, "services", "platform-mihomo", "go.mod"),
	} {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("validate repository root: %w", err)
		}
	}
	return nil
}

func writeState(path string, state State) error {
	temporaryPath := path + ".tmp"
	if err := writeJSON(temporaryPath, state); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish fixture state: %w", err)
	}
	return nil
}

func containerProvider(raw string) (testcontainers.ProviderType, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "podman":
		return testcontainers.ProviderPodman, nil
	case "auto", "default":
		return testcontainers.ProviderDefault, nil
	case "docker":
		return testcontainers.ProviderDocker, nil
	default:
		return testcontainers.ProviderDefault, fmt.Errorf("unsupported PAI_TESTCONTAINERS_PROVIDER %q", raw)
	}
}

func normalizeProcessError(err error) error {
	if err == nil {
		return fmt.Errorf("process exited unexpectedly")
	}
	return err
}
