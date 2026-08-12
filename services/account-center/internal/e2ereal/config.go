//go:build integration

package e2ereal

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	contractticket "github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/serviceticket"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/tlstest"
)

type fixtureMaterial struct {
	controlTLS     tlstest.Bundle
	runtimeTLS     tlstest.Bundle
	signingKeyFile string
	publicKeysFile string
	keyringFile    string
}

func writeFixtureMaterial(directory string) (fixtureMaterial, error) {
	controlTLS, err := tlstest.GenerateRSA(filepath.Join(directory, "control-tls"), "platform-control.internal")
	if err != nil {
		return fixtureMaterial{}, err
	}
	runtimeTLS, err := tlstest.GenerateRSA(filepath.Join(directory, "runtime-tls"), "platform-runtime.internal")
	if err != nil {
		return fixtureMaterial{}, err
	}
	privateKey, publicKey, err := contractticket.GenerateKeyPairPEM()
	if err != nil {
		return fixtureMaterial{}, err
	}
	signingKeyFile := filepath.Join(directory, "service-ticket-signing-key.json")
	if err := writeJSON(signingKeyFile, contractticket.SigningKeyFile{KeyID: "e2e-real", PrivateKeyPEM: privateKey}); err != nil {
		return fixtureMaterial{}, err
	}
	publicKeysFile := filepath.Join(directory, "service-ticket-public-keyring.json")
	if err := writeJSON(publicKeysFile, contractticket.PublicKeyringFile{Keys: []contractticket.PublicKeyEntry{{
		KeyID: "e2e-real", PublicKeyPEM: publicKey,
	}}}); err != nil {
		return fixtureMaterial{}, err
	}
	keyringFile := filepath.Join(directory, "credential-keyring.json")
	if err := writeJSON(keyringFile, map[string]any{
		"active_kid": "e2e-real",
		"keys": []map[string]string{{
			"kid": "e2e-real", "key_base64": base64.RawStdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")),
		}},
	}); err != nil {
		return fixtureMaterial{}, err
	}
	return fixtureMaterial{
		controlTLS: controlTLS, runtimeTLS: runtimeTLS, signingKeyFile: signingKeyFile,
		publicKeysFile: publicKeysFile, keyringFile: keyringFile,
	}, nil
}

func writePlatformConfig(path, databaseDSN, redisAddress, upstreamURL string, material fixtureMaterial) error {
	body := fmt.Sprintf(`server:
  control:
    network: tcp
    addr: "127.0.0.1:19000"
    timeout_seconds: 5
    tls:
      certificate_file: %s
      private_key_file: %s
      client_ca_file: %s
  runtime:
    network: tcp
    addr: "127.0.0.1:19001"
    timeout_seconds: 5
    tls:
      certificate_file: %s
      private_key_file: %s
data:
  database:
    dsn: %s
  redis:
    addr: %s
    db: 0
    prefix: "e2e-real-platform:"
security:
  credential_encryption_keyring_file: %s
  service_ticket_issuer: "paigram-account-center"
  service_ticket_public_keyring_file: %s
upstream:
  base_url: %s
  timeout_seconds: 5
  allow_insecure_http: true
`, quote(material.controlTLS.ServerCertFile), quote(material.controlTLS.ServerKeyFile), quote(material.controlTLS.CAFile),
		quote(material.runtimeTLS.ServerCertFile), quote(material.runtimeTLS.ServerKeyFile), quote(databaseDSN),
		quote(redisAddress), quote(material.keyringFile), quote(material.publicKeysFile), quote(upstreamURL))
	return writeFile(path, []byte(body))
}

func writeAccountConfig(path, repositoryRoot, databaseDSN, redisAddress, frontendURL string, material fixtureMaterial) error {
	migrations := filepath.Join(repositoryRoot, "services", "account-center", "initialize", "migrate", "sql")
	body := fmt.Sprintf(`app:
  name: "PaiGram E2E Real"
  host: "0.0.0.0"
  port: 8080
  mode: "test"
  trusted_proxies: []
  cors:
    enabled: false
database:
  dsn: %s
  migrations_dir: %s
  auto_migrate: true
  auto_seed: true
  log_mode: "silent"
auth:
  access_token_ttl: 900
  refresh_token_ttl: 3600
  session_cookie_secure: false
  session_update_age: 300
  session_fresh_age: 300
  require_verified_email_login: false
  service_ticket_ttl: 300
  service_ticket_issuer: "paigram-account-center"
  service_ticket_signing_key_file: %s
  oauth_signing_key: "0123456789abcdef0123456789abcdef"
  allowed_providers: []
frontend:
  base_url: %s
redis:
  enabled: true
  addr: %s
  db: 0
  prefix: "e2e-real-account:"
platform_control:
  root_ca_file: %s
  certificate_file: %s
  private_key_file: %s
  server_name: "platform-control.internal"
  dial_timeout: 5s
grpc:
  enabled: false
email:
  enabled: false
  use_async: false
rate_limit:
  enabled: false
security:
  bcrypt_cost: 10
  encryption_key: "0123456789abcdef0123456789abcdef"
sentry:
  enabled: false
`, quote(databaseDSN), quote(migrations), quote(material.signingKeyFile), quote(frontendURL), quote(redisAddress),
		quote(material.controlTLS.CAFile), quote(material.controlTLS.ClientCertFile), quote(material.controlTLS.ClientKeyFile))
	return writeFile(path, []byte(body))
}

func writeJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(path, append(raw, '\n'))
}

func writeFile(path string, value []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, value, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return nil
}

func quote(value string) string {
	return strconv.Quote(filepath.ToSlash(value))
}
