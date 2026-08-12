# Account Center Podman deployment

This project deploys Account Center, its PostgreSQL and Redis stores, and the user/admin frontend. Platform Mihomo has an independent project under `deploy/podman-platform-mihomo` and must be deployed first so the shared private network exists.

Run `init-env.ps1` to create only non-secret settings. It requires immutable Account and frontend image references in `registry/repository@sha256:<64 hex characters>` form. Build those images from the checked-in Containerfiles in CI with `VCS_REF` set to the full source commit, `CONTRACT_BASELINE` set to the full commit returned by the contract breaking-baseline query, and `SDK_VERSION` set to the Python project version. The build rejects missing or malformed metadata and stores it in `org.opencontainers.image.revision`, `org.paigram.contract-baseline`, and `org.paigram.sdk-version`. Push each image once, record its registry-reported manifest digest, and pass that canonical reference to the initializer. Production Compose never builds from a mutable working tree and the deployment entry point rejects tag-only images. Base images are also pinned by digest; update their readable tag and digest together during a reviewed dependency update. This follows [Docker's digest-pinning guidance](https://docs.docker.com/build/building/best-practices/#pin-base-image-versions).

Provision every external secret named by `compose.yaml` with `podman secret create`; do not place database credentials, Redis credentials, signing/encryption keys, TLS private keys, or the bootstrap administrator password in `.env`, Compose environment entries, shell arguments, or logs.

The Account Center control client trusts `platform_control_ca.pem` and presents its dedicated mTLS client certificate to `platform-mihomo:9000`. The certificate must be valid for client authentication. The Platform control certificate must contain `platform-control.internal`, which is the exact configured SNI name.

## Secret formats

Generate one Ed25519 key pair, JSON-escape the PEM values, and provision the private and public halves under the same non-secret key ID. The Account signing secret has this shape:

```json
{"kid":"ticket-2026-08","private_key_pem":"-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----\n"}
```

The matching Platform secret `paigram-account-center-service-ticket-public-keyring` has this shape. Keep both old and new entries during rotation:

```json
{"keys":[{"kid":"ticket-2026-08","public_key_pem":"-----BEGIN PUBLIC KEY-----\n...\n-----END PUBLIC KEY-----\n"}]}
```

Generate the key pair with `openssl genpkey -algorithm ED25519` and derive the public key with `openssl pkey -pubout`. Build JSON with a JSON serializer so PEM newlines are escaped correctly. The OAuth signing key must contain at least 32 random bytes. The independent Account encryption secret must be either exactly 32 raw ASCII bytes or the padded standard-Base64 encoding of exactly 32 random bytes. Do not reuse either value as the ticket key or database password.

For Redis, generate one random password. Store its raw value in `account_redis_password`, and create `account_redis_config` from a file containing `requirepass <the same password>`. A mismatch makes both the health check and Account Center fail authentication. Supply Podman secrets from files or standard input, not literal command arguments.

The remaining Account secrets use these formats and pairings:

| Secret | Required content |
| --- | --- |
| `account_postgres_password` | Raw PostgreSQL password. |
| `account_database_dsn` | One PostgreSQL DSN for user/database `paigram` at `postgres:5432`; its URL-escaped password must represent the same raw value as `account_postgres_password`. |
| `account_oauth_signing_key` | Raw random OAuth HMAC key of at least 32 bytes. |
| `account_encryption_key` | Exactly 32 raw ASCII bytes, or padded standard Base64 for exactly 32 random bytes. |
| `account_admin_password` | Raw bootstrap administrator password. |
| `platform_control_ca` | PEM CA bundle that validates the Platform control server certificate. |
| `account_control_client_cert` / `account_control_client_key` | Matching PEM certificate and PKCS#8 private key; the certificate must have client-auth usage and chain to the CA trusted by Platform. |
| `account_grpc_cert` / `account_grpc_key` | Matching PEM server certificate and PKCS#8 private key; the certificate must have server-auth usage and a SAN equal to the SDK-facing Account gRPC server name. |

Encode reserved DSN password characters using PostgreSQL URI percent-encoding. Do not copy a percent-encoded DSN password into the raw PostgreSQL password secret.

```powershell annotate
# Initializes deployment settings with immutable release images before starting the project.
cd deploy/podman
./init-env.ps1 `
  -FrontendBaseUrl https://account.example.com `
  -AccountImage ghcr.io/paigramteam/paigram-account-center@sha256:<64-hex-digest> `
  -FrontendImage ghcr.io/paigramteam/paigram-account-frontend@sha256:<64-hex-digest>
./deploy.ps1
```

The frontend and Account Center Bot gRPC listener publish only their configured loopback ports. PostgreSQL, Redis, Account Center HTTP, and the Platform control listener remain private. Terminate public HTTPS and gRPC TLS routing at trusted ingress where required, and preserve the configured secure-cookie and trusted-proxy policy.

The private `account-center` network intentionally uses `10.77.20.0/24`: the frontend proxy is fixed at `10.77.20.10`, and Account Center trusts only that single address for `X-Forwarded-For`. The network gateway `10.77.20.1` is the only source Nginx trusts to supply an upstream forwarding chain, matching the loopback-published host ingress path. This topology treats the host OS and every process allowed to reach the loopback-published port as part of the trusted computing base; it is not suitable for a multi-tenant host. On a shared host, remove the loopback publication and attach a dedicated ingress workload directly to a private network instead. Check for subnet conflicts and confirm the observed gateway source address in Nginx access logs before first deployment; do not widen either trust entry to an RFC1918 range. If the ingress topology or Podman network backend differs, update the subnet, both exact trust addresses, and the corresponding real-IP test together.

Nginx emits the same CSP, MIME-sniffing, clickjacking, and referrer protections for static user/admin assets and proxied responses. Public TLS ingress remains responsible for preserving these headers and enforcing HSTS at the public HTTPS boundary.

Podman injects secrets only when it creates a container. After every `podman secret create --replace`, recreate each consumer with `podman compose up -d --force-recreate`; restarting an existing container does not load the replacement.

Rotate service-ticket keys by adding the new public key and recreating Platform first, replacing the Account signing secret and recreating Account Center, waiting the ticket TTL plus clock skew, and only then retiring the old public key and recreating Platform again. For TLS CA rotation, first publish an old+new trust bundle and recreate every verifier; next replace leaf certificates and recreate servers and clients; finally remove the old CA and recreate verifiers. New handshakes reload mounted files without a plaintext fallback, but external-secret replacement still requires container recreation.
