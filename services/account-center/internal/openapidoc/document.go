package openapidoc

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"paigram/internal/config"
	"paigram/internal/router"
	"paigram/internal/sessioncache"
)

const documentationSigningKey = "openapi-generation-only-key-000000"

// Generate builds the production route catalog without external services and returns deterministic OpenAPI JSON.
func Generate() (contents []byte, returnErr error) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open OpenAPI catalog database: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get OpenAPI catalog database handle: %w", err)
	}
	defer func() {
		returnErr = errors.Join(returnErr, sqlDB.Close())
	}()

	documentationConfig, err := newDocumentationConfig()
	if err != nil {
		return nil, err
	}
	engine, err := router.New(documentationConfig, sessioncache.NewNoopStore(), db, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("build OpenAPI route catalog: %w", err)
	}

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if response.Code != http.StatusOK {
		return nil, fmt.Errorf("read OpenAPI document: status %d", response.Code)
	}

	return normalize(response.Body.Bytes())
}

func normalize(raw []byte) ([]byte, error) {
	var document struct {
		OpenAPI string                     `json:"openapi"`
		Paths   map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("decode OpenAPI document: %w", err)
	}
	if document.OpenAPI == "" || len(document.Paths) == 0 {
		return nil, errors.New("OpenAPI document has no version or paths")
	}

	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode OpenAPI output: %w", err)
	}
	contents, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode OpenAPI output: %w", err)
	}
	return append(contents, '\n'), nil
}

func newDocumentationConfig() (*config.Config, error) {
	privateKey := ed25519.NewKeyFromSeed([]byte("0123456789abcdef0123456789abcdef"))
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("encode OpenAPI service ticket key: %w", err)
	}

	return &config.Config{
		App: config.AppConfig{
			Name:           "Paigram Account Center",
			Mode:           "test",
			TrustedProxies: []string{"127.0.0.1"},
		},
		OpenAPI: config.OpenAPIConfig{Enabled: true, Path: "/openapi"},
		Auth: config.AuthConfig{
			AccessTokenTTLSeconds:      900,
			RefreshTokenTTLSeconds:     604800,
			ServiceTicketTTLSeconds:    300,
			ServiceTicketIssuer:        "paigram-account-center",
			ServiceTicketKeyID:         "openapi-test-key",
			ServiceTicketPrivateKeyPEM: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})),
			OAuthIssuer:                "account-center",
			OAuthAccessTokenTTLSeconds: 3600,
			OAuthSigningKey:            documentationSigningKey,
		},
		Security: config.SecurityConfig{BcryptCost: 10},
	}, nil
}
