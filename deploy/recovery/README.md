# Encrypted backup and recovery rehearsal

Unless a step explicitly says to run from the repository root, set the working directory to `deploy/recovery` before using the commands below.

The recovery set is one signed, GPG-encrypted archive containing logical dumps of both PostgreSQL databases plus the key material required to read protected Account and Platform data. The backup stops the frontend ingress proxy and both application containers for a bounded maintenance window before reading migration state, key material, and both databases. This prevents cross-service writes between the two sequential `pg_dump` snapshots; containers that were running are restarted in dependency order even when backup creation fails. Redis is deliberately excluded because it contains derived caches and short-lived artifacts; restore clears both Redis databases before loading PostgreSQL.

The backup operator needs PowerShell 7.4 or newer, Podman, GPG, tar, and OpenSSL, plus a GPG encryption recipient, a separate GPG signing key, and the source-of-truth files for:

- Account data-encryption key;
- Account service-ticket signing key document;
- Platform credential-encryption keyring;
- Account service-ticket public keyring used by Platform.

Do not extract these values from container logs, environment variables, or command history. Keep the source files in the secret manager's protected workspace and pass only their paths. `backup.ps1` uses the digest-pinned PostgreSQL client image, writes custom-format `pg_dump` archives, records per-file SHA-256 hashes, clean database migration states, the actual immutable image references, source revisions, contract baselines, and Python SDK versions, signs the tar archive, encrypts it to the recovery recipient, prints the encrypted archive hash, and removes plaintext staging files. The script refuses mutable images, missing provenance labels, missing application containers, or dirty/invalid migration states. PostgreSQL documents custom-format dumps as portable archives restored with `pg_restore`: [pg_dump](https://www.postgresql.org/docs/current/app-pgdump.html) and [pg_restore](https://www.postgresql.org/docs/current/app-pgrestore.html).

```powershell annotate
# Encrypts one recovery set to the named recipient and authenticates it with the signing key.
./backup.ps1 `
  -BackupDirectory D:\ProtectedBackups `
  -GPGRecipient recovery@example.com `
  -GPGSigningKey backup-signer@example.com `
  -AccountEncryptionKeyFile D:\SecretSource\account-encryption-key `
  -AccountServiceTicketSigningKeyFile D:\SecretSource\account-service-ticket-signing-key.json `
  -PlatformEncryptionKeyringFile D:\SecretSource\platform-encryption-keyring.json `
  -AccountServiceTicketPublicKeyringFile D:\SecretSource\account-service-ticket-public-keyring.json
```

Store the encrypted archive off-host with retention and access controls. Record its hash, creation time, image source commits, signer fingerprint, maintenance-window duration, restore-test date, measured RPO, and measured RTO in the operational inventory. A backup is not accepted until a separate environment has completed the rehearsal below.

Before using real data, run `rehearse-storage.ps1`; this synthetic rehearsal additionally requires the repository's Go toolchain to generate a valid Ed25519 ticket-key pair through the shared runtime package. It creates randomly named, isolated synthetic PostgreSQL/Redis instances and a temporary GPG identity, executes the same backup and restore entry points, verifies both restored rows, both Redis flushes, valid key schemas and key-file integrity, and application stop/restart behavior, then removes only those exact synthetic resources. This is an infrastructure test; it does not replace the production-service and SDK tracer required below.

## Restore rehearsal

Use fresh, isolated Account and Platform project names and networks. Provision their PostgreSQL and Redis password secrets before restore. The restore command has `ConfirmImpact=High`; it stops the frontend and both application containers, clears derived Redis state, and executes `pg_restore --clean --if-exists --single-transaction` against both named databases. Never point a rehearsal at the active production instance.

```powershell annotate
# Restores only into the explicitly named isolated rehearsal projects.
./restore.ps1 `
  -BackupFile D:\ProtectedBackups\paigram-recovery-20260813T120000Z.tar.gpg `
  -ExpectedSignerFingerprint 0123456789ABCDEF0123456789ABCDEF01234567 `
  -RecoveredSecretsDirectory D:\RecoveryWorkspace\secrets `
  -AccountInstance paigram-recovery-account `
  -PlatformInstance paigram-recovery-platform
```

The script rejects a missing/invalid signature, path traversal, unknown format, invalid provenance or migration metadata, duplicate/missing files, or any SHA-256/length mismatch before changing databases. It leaves applications stopped and writes recovered key files plus `recovery-manifest.json` into a new restricted directory.

Replace the four corresponding Podman secrets from the recovered files:

| Recovered file | External Podman secret |
| --- | --- |
| `account-encryption-key` | `<account-instance>-encryption-key` |
| `account-service-ticket-signing-key.json` | `<account-instance>-service-ticket-signing-key` |
| `platform-encryption-keyring.json` | `<platform-instance>-encryption-keyring` |
| `account-service-ticket-public-keyring.json` | `paigram-account-center-service-ticket-public-keyring` |

Replace each secret from the repository root, substituting the two isolated instance names exactly:

```powershell annotate
# Podman stores the recovered bytes without exposing them through environment variables.
podman secret create --replace paigram-recovery-account-encryption-key D:\RecoveryWorkspace\secrets\account-encryption-key
podman secret create --replace paigram-recovery-account-service-ticket-signing-key D:\RecoveryWorkspace\secrets\account-service-ticket-signing-key.json
podman secret create --replace paigram-recovery-platform-encryption-keyring D:\RecoveryWorkspace\secrets\platform-encryption-keyring.json
podman secret create --replace paigram-account-center-service-ticket-public-keyring D:\RecoveryWorkspace\secrets\account-service-ticket-public-keyring.json
```

Then update `deploy/podman-platform-mihomo/.env` and `deploy/podman/.env`: use the isolated target instance names, keep all host publications on loopback, and copy the three exact digest references from `recovery-manifest.json`. Set `PAI_RUNTIME_SERVER_NAME` to the restored runtime certificate's DNS name. From the repository root, recreate Platform first and Account second through the production Compose entry points:

```powershell annotate
# Recreates both isolated projects so Podman injects the recovered secret versions.
& ./deploy/podman-platform-mihomo/deploy.ps1
& ./deploy/podman/deploy.ps1
```

These entry points pull and recreate the target containers through the production Compose definitions; replacing a Podman secret does not update an existing container. Keep ingress closed until the recreated containers are healthy.

Create a restricted JSON file outside the repository for the release recovery tracer. It contains live recovery-test credentials and must not be committed, printed, symlinked, or passed directly in a command argument. On Unix set mode `0600`. On Windows remove inherited access and grant only the current operator, for example `icacls <path> /inheritance:r /grant:r "<DOMAIN\\user>:(F)"`. The verifier rejects repository paths, links, group/world Unix permissions, and Windows access grants to another identity. Only the file path is passed to the verifier.

```json annotate
// Every expected identifier must come from the synthetic records created before the backup.
{
  "account_grpc_server_name": "account-bot.internal",
  "account_ca_file": "D:\\RecoveryWorkspace\\account-grpc-ca.pem",
  "platform_service_key": "platform-mihomo-service",
  "platform_ca_file": "D:\\RecoveryWorkspace\\platform-runtime-ca.pem",
  "user_email": "recovery-user@example.test",
  "user_password": "replace-with-the-private-test-password",
  "totp_secret": "replace-with-the-private-base32-secret",
  "external_user_id": "recovery-external-user",
  "client_id": "recovery-bot",
  "client_secret": "replace-with-the-private-client-secret",
  "expected_binding_ref": "replace-with-the-restored-binding-ref",
  "expected_account_key": "replace-with-the-restored-account-key",
  "expected_profile_ref": "replace-with-the-restored-profile-ref",
  "expected_authkey_prefix": "recovery-authkey-",
  "timeout_seconds": 15
}
```

The verifier requires PowerShell 7.4, Podman, Git, uv, and a clean checkout of the source commit recorded in the manifest. From that repository root, update the restored Account registry through the authenticated admin API. Store a short-lived restored-admin access token in a separate restricted file and use the exact loopback Platform runtime publication and `PAI_RUNTIME_SERVER_NAME` from the isolated Platform `.env`. The registration entry point validates the token file with the same private-file policy as the tracer and consumes it before returning, including failure paths:

```powershell annotate
# Repoints only the isolated restored registry to its recovered Platform listener.
& ./deploy/podman-platform-mihomo/register-descriptor.ps1 `
  -AccountCenterUrl http://127.0.0.1:18080 `
  -AdminAccessTokenFile D:\RecoveryWorkspace\private\admin-token.txt `
  -RuntimeEndpoint 127.0.0.1:19001 `
  -RuntimeServerName platform-runtime.internal `
  -AllowLoopbackHTTP
```

Run the joint release verifier only after restore, secret replacement, container recreation, and the isolated registry update. Use a new evidence filename for every attempt; the verifier never overwrites prior evidence:

```powershell annotate
# Proves the exact manifest images loaded the restored databases and recovery keys.
./verify-restored-release.ps1 `
  -RecoveredSecretsDirectory D:\RecoveryWorkspace\secrets `
  -TracerConfigFile D:\RecoveryWorkspace\private\tracer.json `
  -EvidenceFile D:\RecoveryEvidence\release-recovery-evidence-20260813T120000Z.json `
  -AccountInstance paigram-recovery-account `
  -PlatformInstance paigram-recovery-platform `
  -PlatformNetwork paigram-recovery-backplane
```

The `PlatformNetwork` argument must exactly match `PAI_PLATFORM_NETWORK` in both isolated `.env` files. The verifier permits the isolated target instance names to differ from the source instance names recorded in the backup. It fails unless all three target application containers use the recorded digest references with matching source/contract/SDK provenance, the verifier and SDK checkout exactly match that source commit without local changes, both migration rows are clean and unchanged, and the four mounted recovery files hash to the restored files. The Account HTTP, Account gRPC, and Platform runtime ports must each have exactly one loopback publication. Set the restored Mihomo registry runtime route to that target Platform loopback port and its certificate SNI before running the verifier; the verifier reads this route from the restored database and rejects any other endpoint. It explicitly probes Account liveness/readiness and Platform liveness/readiness, then uses the locked Python SDK environment and only the derived loopback endpoints to prove restored login and TOTP decryption, binding/status/profile reads through an Account-issued ticket, and a newly issued AuthKey. The recovery tracer is deliberately excluded from the public SDK wheel. The resulting evidence file contains only non-secret digests, image references, migration states, loopback ports, and the tracer result.

Do not reopen ingress until this verifier covers all of the following:

1. Account `/livez` and `/readyz`, and Platform gRPC `liveness` and aggregate readiness;
2. migration/schema checks for both databases;
3. login and 2FA decryption using a restored Account record;
4. restored Platform credential validation and profile snapshot read;
5. the production Account signer → Platform verifier → Python SDK tracer;
6. an AuthKey issued after restore, proving the recovered AEAD keyring can decrypt an existing credential;
7. inspection showing the recreated containers use the recorded immutable image digests.

After evidence is captured, securely delete the private tracer file and recovered plaintext secret directory, then remove the isolated containers, networks, volumes, and temporary Podman secrets by exact rehearsal instance name. Retain the encrypted backup and separately stored non-secret evidence according to policy. A green `rehearse-storage.ps1` run and an independent production tracer are useful separate checks, but release acceptance requires this joint verifier against the same restored data, key material, and manifest images.
