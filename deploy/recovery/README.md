# Encrypted backup and recovery rehearsal

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

The script rejects a missing/invalid signature, path traversal, unknown format, invalid provenance or migration metadata, duplicate/missing files, or any SHA-256/length mismatch before changing databases. It leaves applications stopped and writes recovered key files plus `recovery-manifest.json` into a new restricted directory. Replace the four corresponding Podman secrets from those files, force-recreate the frontend and both application containers from the exact manifest image references, and do not reopen ingress until all of the following pass:

1. Account `/livez` and `/readyz`, and Platform gRPC `liveness` and aggregate readiness;
2. migration/schema checks for both databases;
3. login and 2FA decryption using a restored Account record;
4. restored Platform credential validation and profile snapshot read;
5. the production Account signer → Platform verifier → Python SDK tracer;
6. an AuthKey issued after restore, proving the recovered AEAD keyring can decrypt an existing credential;
7. inspection showing the recreated containers use the recorded immutable image digests.

After evidence is captured, remove the isolated containers, networks, volumes, recovered plaintext secret directory, and temporary Podman secrets by exact rehearsal instance name. Retain the encrypted backup and evidence according to policy.
