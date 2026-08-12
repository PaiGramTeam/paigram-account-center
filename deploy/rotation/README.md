# External-secret rotation rehearsal

`rehearse-external-secrets.ps1` exercises the deployment boundary that application tests cannot model: Podman external secrets are immutable for the lifetime of a container. The rehearsal uses valid Ed25519 ticket keys, 32-byte credential-encryption keys, and old/new CA and leaf certificates for Platform control mTLS, Platform runtime server-auth TLS, and Account Bot gRPC server-auth TLS. It performs the production three-stage sequence through `podman-compose` 1.6.0 or later:

1. publish overlapping ticket, encryption, and CA trust material, then recreate consumers;
2. switch the ticket signer and TLS server/client identities, then recreate consumers;
3. run the real service-ticket overlap and PostgreSQL credential re-encryption checks, wait the five-minute ticket TTL plus the 30-second verifier leeway, and only then retire the old verification, encryption, and CA keys and recreate consumers.

At every stage the rehearsal replaces the external secret, restarts the existing consumers, and proves that they still see the old mounted bytes. It then force-recreates the consumers through the same provider used by production and proves that every mounted byte matches the new source. It also checks that ticket, AEAD, and TLS private-key payloads are absent from container environment, inspect output, and logs. This directly exercises Podman's documented behavior that [`podman secret create --replace` affects only newly created containers](https://docs.podman.io/en/v6.0.0/markdown/podman-secret-create.1.html).

Prerequisites are PowerShell 7.4 or later, Podman, `podman-compose` 1.6.0 or later, Go, and OpenSSL. The default 330-second retirement delay cannot be shortened. The rehearsal creates only randomly named containers, a Compose pod/network, secrets, and temporary files, and removes them in `finally`.

```powershell annotate
# Runs the deployment-level replacement and recreation rehearsal.
cd deploy/rotation
./rehearse-external-secrets.ps1
```

The deployment rehearsal complements, rather than substitutes for, the application-level rotation checks. Run the targeted owners when rotation code or configuration changes:

```powershell annotate
# Returns to the repository root before running the owning Go checks directly.
cd ../..
go test ./contracts/runtime/go/serviceticket ./contracts/runtime/go/transporttls
go test ./services/platform-mihomo/internal/crypto ./services/platform-mihomo/internal/usecase
go test -tags=integration -run '^TestCredentialKeyRotationReencryptsPersistentPostgreSQLRecords$' ./services/platform-mihomo/integration
```

The final command starts a disposable PostgreSQL dependency. A release-candidate exercise must additionally run the production cross-service tracer after each applicable application recreate; this probe does not claim to replace that release evidence.
