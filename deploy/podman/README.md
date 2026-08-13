# Account Center Podman deployment

This project deploys Account Center, its PostgreSQL and Redis stores, and the user/admin frontend. Platform Mihomo has an independent project under `deploy/podman-platform-mihomo` and must be deployed first so the shared private network exists.

Run `init-env.ps1` to create only non-secret settings. It requires immutable Account and frontend image references in `registry/repository@sha256:<64 hex characters>` form. Build those images from the checked-in Containerfiles in CI with `VCS_REF` set to the full source commit, `CONTRACT_BASELINE` set to the full commit returned by the contract breaking-baseline query, and `SDK_VERSION` set to the Python project version. The build rejects missing or malformed metadata and stores it in `org.opencontainers.image.revision`, `org.paigram.contract-baseline`, and `org.paigram.sdk-version`. Push each image once, record its registry-reported manifest digest, and pass that canonical reference to the initializer. Production Compose never builds from a mutable working tree and the deployment entry point rejects tag-only images. Base images are also pinned by digest; update their readable tag and digest together during a reviewed dependency update. This follows [Docker's digest-pinning guidance](https://docs.docker.com/build/building/best-practices/#pin-base-image-versions).

Install `podman-compose` as the secret-aware Compose provider before deployment; the entry point invokes it directly because `podman compose` is only a wrapper and prefers Docker Compose when both providers exist. Docker Compose over Podman's compatibility API cannot consume Podman external secrets. A reproducible installation is `uv tool install podman-compose==1.6.0`. The provider behavior follows Podman's [Compose wrapper documentation](https://docs.podman.io/en/v5.4.0/markdown/podman-compose.1.html).

Provision every external secret named by `compose.yaml` with `podman secret create`; do not place database credentials, Redis credentials, signing/encryption keys, TLS private keys, or the bootstrap administrator password in `.env`, Compose environment entries, shell arguments, or logs. Names below use `<account-instance>`, whose default is `paigram-account-center`; replace it with the exact `PAI_INSTANCE` value for a custom instance.

The Account Center control client trusts the CA mounted from `<account-instance>-platform-control-ca` and presents the certificate/key mounted from `<account-instance>-control-client-cert` and `<account-instance>-control-client-key` to `platform-mihomo:9000`. The certificate must be valid for client authentication. The Platform control certificate must contain `platform-control.internal`, which is the exact configured SNI name.

## Secret formats

Generate one Ed25519 key pair, JSON-escape the PEM values, and provision the private and public halves under the same non-secret key ID. The Account signing secret has this shape:

```json
{"kid":"ticket-2026-08","private_key_pem":"-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----\n"}
```

The Account secret `<account-instance>-service-ticket-signing-key` contains the private document above. The matching Platform secret `paigram-account-center-service-ticket-public-keyring` has this shape. Keep both old and new entries during rotation:

```json
{"keys":[{"kid":"ticket-2026-08","public_key_pem":"-----BEGIN PUBLIC KEY-----\n...\n-----END PUBLIC KEY-----\n"}]}
```

Generate the key pair with `openssl genpkey -algorithm ED25519` and derive the public key with `openssl pkey -pubout`. Build JSON with a JSON serializer so PEM newlines are escaped correctly. The OAuth signing key must contain at least 32 random bytes. The independent Account encryption secret must be either exactly 32 raw ASCII bytes or the padded standard-Base64 encoding of exactly 32 random bytes. Do not reuse either value as the ticket key or database password.

For Redis, generate one random password. Store its raw value in `<account-instance>-redis-password`, and create `<account-instance>-redis-config` from a file containing `requirepass <the same password>`. A mismatch makes both the health check and Account Center fail authentication. Supply Podman secrets from files or standard input, not literal command arguments.

The remaining Account secrets use these formats and pairings:

| Secret | Required content |
| --- | --- |
| `<account-instance>-postgres-password` | Raw PostgreSQL password. |
| `<account-instance>-database-dsn` | One PostgreSQL DSN for user/database `paigram` at `postgres:5432`; its URL-escaped password must represent the same raw value as `<account-instance>-postgres-password`. |
| `<account-instance>-oauth-signing-key` | Raw random OAuth HMAC key of at least 32 bytes. |
| `<account-instance>-encryption-key` | Exactly 32 raw ASCII bytes, or padded standard Base64 for exactly 32 random bytes. |
| `<account-instance>-admin-password` | Raw bootstrap administrator password. |
| `<account-instance>-platform-control-ca` | PEM CA bundle that validates the Platform control server certificate. |
| `<account-instance>-control-client-cert` / `<account-instance>-control-client-key` | Matching PEM certificate and PKCS#8 private key; the certificate must have client-auth usage and chain to the CA trusted by Platform. |
| `<account-instance>-grpc-cert` / `<account-instance>-grpc-key` | Matching PEM server certificate and PKCS#8 private key; the certificate must have server-auth usage and a SAN equal to the SDK-facing Account gRPC server name. |

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

Each deployment first stops the previous Compose project and verifies that no project container remains, then runs digest-pinned one-shot `migrate` and `seed` services. The Account Center service starts only after both jobs exit successfully. The seed job idempotently reconciles the managed permission and role catalog and creates the bootstrap administrator only when no active recovery administrator exists. The long-running service has automatic migration and seeding disabled, so multiple replicas never race to change the schema during startup. A failed teardown, migration, or seed keeps Account Center and the frontend stopped; preserve the failing `deploy.ps1` output before retrying because successful and failed one-shot containers are removed after execution.

The frontend and Account Center Bot gRPC listener publish only their configured loopback ports. PostgreSQL, Redis, Account Center HTTP, and the Platform control listener remain private. Terminate public HTTPS and gRPC TLS routing at trusted ingress where required, and preserve the configured secure-cookie and trusted-proxy policy.

Account Center exposes Prometheus metrics only inside the container networks at `account-center:8080/metrics`; the frontend returns `404` for that path. See the [monitoring rules and scrape topology](../monitoring/README.md).

The private `account-center` network intentionally uses `10.77.20.0/24`: the frontend proxy is fixed at `10.77.20.10`, and Account Center trusts only that single address for `X-Forwarded-For`. The network gateway `10.77.20.1` is the only source Nginx trusts to supply an upstream forwarding chain, matching the loopback-published host ingress path. This topology treats the host OS and every process allowed to reach the loopback-published port as part of the trusted computing base; it is not suitable for a multi-tenant host. On a shared host, remove the loopback publication and attach a dedicated ingress workload directly to a private network instead. Check for subnet conflicts and confirm the observed gateway source address in Nginx access logs before first deployment; do not widen either trust entry to an RFC1918 range. If the ingress topology or Podman network backend differs, update the subnet, both exact trust addresses, and the corresponding real-IP test together.

Nginx emits the same CSP, MIME-sniffing, clickjacking, and referrer protections for static user/admin assets and proxied responses. Public TLS ingress remains responsible for preserving these headers and enforcing HSTS at the public HTTPS boundary.

Podman injects secrets only when it creates a container. After every `podman secret create --replace`, recreate each consumer with `podman-compose up -d --force-recreate`; restarting an existing container does not load the replacement.

Rotate service-ticket keys by adding the new public key and recreating Platform first, replacing the Account signing secret and recreating Account Center, waiting the ticket TTL plus clock skew, and only then retiring the old public key and recreating Platform again. For TLS CA rotation, first publish an old+new trust bundle and recreate every verifier; next replace leaf certificates and recreate servers and clients; finally remove the old CA and recreate verifiers. New handshakes reload mounted files without a plaintext fallback, but external-secret replacement still requires container recreation.

Run the checked-in [external-secret rotation rehearsal](../rotation/README.md) before approving a key or certificate rotation procedure for an environment.
