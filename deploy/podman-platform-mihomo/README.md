# Platform Mihomo Podman deployment

This project deploys Platform Mihomo independently from Account Center. It owns its PostgreSQL database, Redis instance, image, configuration, and lifecycle.

Set `PAI_PLATFORM_IMAGE` to a canonical `registry/repository@sha256:<64 hex characters>` reference built and pushed from the checked-in Containerfile. CI must supply the full source commit as `VCS_REF`, the full contract breaking-baseline commit as `CONTRACT_BASELINE`, and the Python project version as `SDK_VERSION`. The build rejects missing or malformed metadata and stores it in `org.opencontainers.image.revision`, `org.paigram.contract-baseline`, and `org.paigram.sdk-version`. Production Compose never builds from the current working tree, and `deploy.ps1` rejects tag-only images. Base images are pinned the same way; update tags and digests together during reviewed dependency updates. See [Docker's digest-pinning guidance](https://docs.docker.com/build/building/best-practices/#pin-base-image-versions).

Install `podman-compose` as the secret-aware Compose provider before deployment; the entry point invokes it directly because `podman compose` is only a wrapper and prefers Docker Compose when both providers exist. Docker Compose over Podman's compatibility API cannot consume Podman external secrets. A reproducible installation is `uv tool install podman-compose==1.6.0`. The provider behavior follows Podman's [Compose wrapper documentation](https://docs.podman.io/en/v5.4.0/markdown/podman-compose.1.html).

Create the shared private network once, then provision every value under the base `compose.yaml` `secrets:` section with `podman secret create`. Secret contents must not be placed in `.env`, Compose environment entries, command-line arguments, or logs. The Account Center project must join the same network. Names below use `<platform-instance>`, whose default is `paigram-platform-mihomo`; replace it with the exact `PAI_PLATFORM_INSTANCE` value for a custom instance.

The control and runtime listeners use plaintext by default. Set `PAI_PLATFORM_CONTROL_TLS=true` to add `compose.control-tls.yaml`; the matching Account deployment must also enable its control TLS overlay. Set `PAI_PLATFORM_RUNTIME_TLS=true` to add `compose.runtime-tls.yaml`; `PAI_RUNTIME_SERVER_NAME` must then match the runtime certificate SAN. Each TLS overlay requires only the secrets it declares, and an enabled TLS connection never falls back to plaintext.

Both listeners expose standard gRPC Health. The empty service name reports readiness and becomes `NOT_SERVING` when PostgreSQL or Redis is unavailable. The `liveness` service remains `SERVING` during dependency outages and changes to `NOT_SERVING` only during process shutdown. The Podman health check intentionally probes readiness.

The private backplane also exposes Prometheus metrics at `platform-mihomo:9090/metrics`; it is not published on the host. See the [monitoring rules and scrape topology](../monitoring/README.md).

## Secret formats

The shared Platform secret `paigram-account-center-service-ticket-public-keyring` must use the same `kid` and public key as the Account Center signing secret:

```json
{"keys":[{"kid":"ticket-2026-08","public_key_pem":"-----BEGIN PUBLIC KEY-----\n...\n-----END PUBLIC KEY-----\n"}]}
```

The `<platform-instance>-encryption-keyring` secret contains independent random 32-byte keys encoded as unpadded standard Base64. Key IDs are 1–64 ASCII letters, digits, underscores, or hyphens:

```json
{"active_kid":"enc-2026-08","keys":[{"kid":"enc-2026-08","key_base64":"<unpadded-base64-of-32-random-bytes>"}]}
```

During rotation, add the new entry and change `active_kid`; do not remove the old entry until persistent envelopes have migrated. Generate key bytes with a cryptographically secure random generator and build the document with a JSON serializer.

Generate one Redis password. Store its raw value in `<platform-instance>-redis-password`, and create `<platform-instance>-redis-config` from a file containing `requirepass <the same password>`. The two contents must match because Redis, its health check, and Platform Mihomo use them independently. Supply all Podman secrets from files or standard input rather than literal command arguments.

The remaining Platform secrets use these formats and pairings:

| Secret | Required content |
| --- | --- |
| `<platform-instance>-postgres-password` | Raw PostgreSQL password. |
| `<platform-instance>-database-dsn` | One PostgreSQL DSN for user/database `platform_mihomo` at `postgres:5432`; its URL-escaped password must represent the same raw value as `<platform-instance>-postgres-password`. |
| `<platform-instance>-control-cert` / `<platform-instance>-control-key` | Optional matching server identity used when `PAI_PLATFORM_CONTROL_TLS=true`. |
| `paigram-account-center-control-client-ca` | Optional CA bundle used by the control mTLS overlay. |
| `<platform-instance>-runtime-cert` / `<platform-instance>-runtime-key` | Optional matching server identity used when `PAI_PLATFORM_RUNTIME_TLS=true`. |
| `<platform-instance>-runtime-ca` | Optional CA bundle used by the TLS readiness probe. |
| `<platform-instance>-upstream-token` | Raw upstream bearer token with no `Bearer ` prefix. |
| `<platform-instance>-upstream-ca` | Optional PEM CA bundle that validates an HTTPS Mihomo upstream using a private CA. |

Encode reserved DSN password characters using PostgreSQL URI percent-encoding. Do not copy a percent-encoded DSN password into the raw PostgreSQL password secret.

The default composition validates the upstream with the image's system roots and does not require an upstream CA secret. For an upstream using a private CA, create `<platform-instance>-upstream-ca`, set `PAI_MIHOMO_UPSTREAM_PRIVATE_CA=true` in `.env`, and deploy through `deploy.ps1`. The entry point then adds `compose.upstream-private-ca.yaml`; enabling it without the external secret fails before the service starts. Do not mount a public system root bundle through this override.

Each deployment first stops the previous Compose project and verifies that no project container remains, then runs a digest-pinned one-shot `migrate` service after PostgreSQL becomes healthy. Platform Mihomo starts only after that job exits successfully. The migration command reads only the database DSN secret and embedded migrations; it does not weaken the runtime requirements for Redis, keyrings, metrics, the HTTPS upstream, or any TLS identity explicitly enabled through an overlay. A failed teardown or migration keeps both gRPC listeners stopped; preserve the failing `deploy.ps1` output before retrying because the one-shot container is removed after execution.

Podman injects secrets only when it creates a container. After every `podman secret create --replace`, recreate each consumer with `podman-compose up -d --force-recreate`; restarting an existing container does not load the replacement.

Service-ticket rotation order is fixed: add the new public key to the Platform keyring and recreate Platform; replace the Account signing key and recreate Account Center; wait at least the maximum ticket TTL plus clock skew; then remove the old public key and recreate Platform again. Encryption rotation keeps the old and new keys in the Platform keyring while the new key is active. Recreate Platform to activate it. The background credential re-encryption worker and normal credential reads migrate persistent envelopes; short-lived AuthKey artifacts are migrated on read or expire within five minutes. Confirm that no `credential_records.credential_blob` values retain the retiring `v2.<kid>.` prefix before removing that key and recreating Platform.

When optional TLS is enabled, rotate its CA by first deploying a trust bundle containing old and new CAs and recreating every verifier. Only then replace leaf certificates and recreate their servers and clients. After all new handshakes succeed, remove the old CA and recreate verifiers a second time.

Run the checked-in [external-secret rotation rehearsal](../rotation/README.md) before approving a key or certificate rotation procedure for an environment.

The checked-in `registry-descriptor.json` is the desired Account Center registry payload. After both projects are healthy, apply it idempotently with `register-descriptor.ps1 -AccountCenterUrl https://account.example.com -AdminAccessTokenFile <temporary-file> -RuntimeEndpoint 127.0.0.1:19001`. Add `-RuntimeServerName runtime.example.com` only when operators will configure the SDK TLS CA mapping for this platform. Delete the temporary token file after registration. The control endpoint stays private; the authenticated machine route exposes the runtime endpoint, optional TLS server name, audience, and supported actions.
