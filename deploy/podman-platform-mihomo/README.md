# Platform Mihomo Podman deployment

This project deploys Platform Mihomo independently from Account Center. It owns its PostgreSQL database, Redis instance, image, configuration, and lifecycle.

Create the shared private network once, then provision every value under `secrets:` with `podman secret create`. Secret contents must not be placed in `.env`, Compose environment entries, command-line arguments, or logs. The Account Center project must join the same network.

The control listener is available only as `platform-mihomo:9000` on the shared network and requires an Account Center client certificate. The runtime listener publishes TLS port `9001` through the loopback binding configured in `.env`. Its certificate SAN must match the runtime server name distributed to Bot operators.

## Secret formats

The Platform public ticket keyring must use the same `kid` and public key as the Account Center signing secret:

```json
{"keys":[{"kid":"ticket-2026-08","public_key_pem":"-----BEGIN PUBLIC KEY-----\n...\n-----END PUBLIC KEY-----\n"}]}
```

The credential encryption keyring contains independent random 32-byte keys encoded as unpadded standard Base64. Key IDs are 1–64 ASCII letters, digits, underscores, or hyphens:

```json
{"active_kid":"enc-2026-08","keys":[{"kid":"enc-2026-08","key_base64":"<unpadded-base64-of-32-random-bytes>"}]}
```

During rotation, add the new entry and change `active_kid`; do not remove the old entry until persistent envelopes have migrated. Generate key bytes with a cryptographically secure random generator and build the document with a JSON serializer.

Generate one Redis password. Store its raw value in `platform_redis_password`, and create `platform_redis_config` from a file containing `requirepass <the same password>`. The two contents must match because Redis, its health check, and Platform Mihomo use them independently. Supply all Podman secrets from files or standard input rather than literal command arguments.

The remaining Platform secrets use these formats and pairings:

| Secret | Required content |
| --- | --- |
| `platform_postgres_password` | Raw PostgreSQL password. |
| `platform_database_dsn` | One PostgreSQL DSN for user/database `platform_mihomo` at `postgres:5432`; its URL-escaped password must represent the same raw value as `platform_postgres_password`. |
| `platform_control_cert` / `platform_control_key` | Matching PEM server certificate and PKCS#8 private key; the certificate must have server-auth usage and SAN `platform-control.internal`. |
| `account_control_client_ca` | PEM CA bundle that validates the Account Center control client certificate. |
| `platform_runtime_cert` / `platform_runtime_key` | Matching PEM server certificate and PKCS#8 private key; the server-auth SAN must equal the runtime server name registered with Account Center. |
| `mihomo_upstream_bearer_token` | Raw upstream bearer token with no `Bearer ` prefix. |

Encode reserved DSN password characters using PostgreSQL URI percent-encoding. Do not copy a percent-encoded DSN password into the raw PostgreSQL password secret.

Podman injects secrets only when it creates a container. After every `podman secret create --replace`, recreate each consumer with `podman compose up -d --force-recreate`; restarting an existing container does not load the replacement.

Service-ticket rotation order is fixed: add the new public key to the Platform keyring and recreate Platform; replace the Account signing key and recreate Account Center; wait at least the maximum ticket TTL plus clock skew; then remove the old public key and recreate Platform again. Encryption rotation keeps the old and new keys in the Platform keyring while the new key is active. Recreate Platform to activate it. The background credential re-encryption worker and normal credential reads migrate persistent envelopes; short-lived AuthKey artifacts are migrated on read or expire within five minutes. Confirm that no `credential_records.credential_blob` values retain the retiring `v2.<kid>.` prefix before removing that key and recreating Platform.

For TLS CA rotation, first deploy a trust bundle containing old and new CAs and recreate every verifier. Only then replace leaf certificates and recreate their servers and clients. After all new handshakes succeed, remove the old CA and recreate verifiers a second time.

The checked-in `registry-descriptor.json` is the desired Account Center registry payload. After both projects are healthy, apply it idempotently with `register-descriptor.ps1 -AccountCenterUrl https://account.example.com -AdminAccessTokenFile <temporary-file> -RuntimeEndpoint runtime.example.com:443 -RuntimeServerName runtime.example.com`. The explicit runtime values must match the published ingress and certificate SAN. Delete the temporary token file after registration. The control endpoint stays private; the authenticated machine route exposes only the runtime endpoint, exact TLS server name, audience, and supported actions.
