# Platform Mihomo Podman deployment

This project deploys Platform Mihomo independently from Account Center. It owns its PostgreSQL database, Redis instance, image, configuration, and lifecycle.

Set `PAI_PLATFORM_IMAGE` to a canonical `registry/repository@sha256:<64 hex characters>` reference built and pushed from the checked-in Containerfile. CI must supply the full source commit as `VCS_REF`, the full contract breaking-baseline commit as `CONTRACT_BASELINE`, and the Python project version as `SDK_VERSION`. The build rejects missing or malformed metadata and stores it in `org.opencontainers.image.revision`, `org.paigram.contract-baseline`, and `org.paigram.sdk-version`. Production Compose never builds from the current working tree, and `deploy.ps1` rejects tag-only images. Base images are pinned the same way; update tags and digests together during reviewed dependency updates. See [Docker's digest-pinning guidance](https://docs.docker.com/build/building/best-practices/#pin-base-image-versions).

Install `podman-compose` as the secret-aware Compose provider before deployment; the entry point invokes it directly because `podman compose` is only a wrapper and prefers Docker Compose when both providers exist. Docker Compose over Podman's compatibility API cannot consume Podman external secrets. A reproducible installation is `uv tool install podman-compose==1.6.0`. The provider behavior follows Podman's [Compose wrapper documentation](https://docs.podman.io/en/v5.4.0/markdown/podman-compose.1.html).

Create the shared private network once, then provision every value under `secrets:` with `podman secret create`. Secret contents must not be placed in `.env`, Compose environment entries, command-line arguments, or logs. The Account Center project must join the same network. Names below use `<platform-instance>`, whose default is `paigram-platform-mihomo`; replace it with the exact `PAI_PLATFORM_INSTANCE` value for a custom instance.

The control listener is available only as `platform-mihomo:9000` on the shared network and requires an Account Center client certificate. The runtime listener publishes TLS port `9001` through the loopback binding configured in `.env`. Its certificate SAN must match the runtime server name distributed to Bot operators.

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
| `<platform-instance>-control-cert` / `<platform-instance>-control-key` | Matching PEM server certificate and PKCS#8 private key; the certificate must have server-auth usage and SAN `platform-control.internal`. |
| `paigram-account-center-control-client-ca` | PEM CA bundle that validates the Account Center control client certificate. |
| `<platform-instance>-runtime-cert` / `<platform-instance>-runtime-key` | Matching PEM server certificate and PKCS#8 private key; the server-auth SAN must equal the runtime server name registered with Account Center. |
| `<platform-instance>-runtime-ca` | PEM CA bundle that validates `<platform-instance>-runtime-cert`; the in-container readiness probe uses it with `PAI_RUNTIME_SERVER_NAME`. |
| `<platform-instance>-upstream-token` | Raw upstream bearer token with no `Bearer ` prefix. |

Encode reserved DSN password characters using PostgreSQL URI percent-encoding. Do not copy a percent-encoded DSN password into the raw PostgreSQL password secret.

Podman injects secrets only when it creates a container. After every `podman secret create --replace`, recreate each consumer with `podman-compose up -d --force-recreate`; restarting an existing container does not load the replacement.

Service-ticket rotation order is fixed: add the new public key to the Platform keyring and recreate Platform; replace the Account signing key and recreate Account Center; wait at least the maximum ticket TTL plus clock skew; then remove the old public key and recreate Platform again. Encryption rotation keeps the old and new keys in the Platform keyring while the new key is active. Recreate Platform to activate it. The background credential re-encryption worker and normal credential reads migrate persistent envelopes; short-lived AuthKey artifacts are migrated on read or expire within five minutes. Confirm that no `credential_records.credential_blob` values retain the retiring `v2.<kid>.` prefix before removing that key and recreating Platform.

For TLS CA rotation, first deploy a trust bundle containing old and new CAs and recreate every verifier. Only then replace leaf certificates and recreate their servers and clients. After all new handshakes succeed, remove the old CA and recreate verifiers a second time.

Run the checked-in [external-secret rotation rehearsal](../rotation/README.md) before approving a key or certificate rotation procedure for an environment.

The checked-in `registry-descriptor.json` is the desired Account Center registry payload. After both projects are healthy, apply it idempotently with `register-descriptor.ps1 -AccountCenterUrl https://account.example.com -AdminAccessTokenFile <temporary-file> -RuntimeEndpoint runtime.example.com:443 -RuntimeServerName runtime.example.com`. The explicit runtime values must match the published ingress and certificate SAN. Delete the temporary token file after registration. The control endpoint stays private; the authenticated machine route exposes only the runtime endpoint, exact TLS server name, audience, and supported actions.
