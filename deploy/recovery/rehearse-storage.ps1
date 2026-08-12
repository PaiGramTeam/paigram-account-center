#requires -Version 7.4

[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$postgresImage = "docker.io/library/postgres:16-alpine@sha256:57c72fd2a128e416c7fcc499958864df5301e940bca0a56f58fddf30ffc07777"
$redisImage = "docker.io/library/redis:7-alpine@sha256:e7723ff73d963f5cc6d9c4643ea3d989527a402a319239054e9472a7fb9219a2"
$alpineImage = "docker.io/library/alpine:3.23@sha256:fd791d74b68913cbb027c6546007b3f0d3bc45125f797758156952bc2d6daf40"
$sourceCommit = "0000000000000000000000000000000000000000"
$contractBaseline = "1111111111111111111111111111111111111111"
$sdkVersion = "0.1.0"
$suffix = [Guid]::NewGuid().ToString("N").Substring(0, 10)
$accountInstance = "pai-rehearsal-account-$suffix"
$platformInstance = "pai-rehearsal-platform-$suffix"
$accountNetwork = $accountInstance
$platformNetwork = "$platformInstance-private"
$containers = @(
    "$accountInstance-postgres",
    "$accountInstance-redis",
    "$platformInstance-postgres",
    "$platformInstance-redis",
    $accountInstance,
    "$accountInstance-frontend",
    $platformInstance
)
$secrets = @(
    "$accountInstance-postgres-password",
    "$accountInstance-redis-password",
    "$platformInstance-postgres-password",
    "$platformInstance-redis-password"
)
$networks = @($accountNetwork, $platformNetwork)
$testPassword = "rehearsal-$suffix"
$temporaryRoot = Join-Path ([System.IO.Path]::GetTempPath()) "paigram-recovery-rehearsal-$suffix"
$originalGPGHome = $env:GNUPGHOME

function Invoke-Checked {
    param(
        [Parameter(Mandatory)][string]$Command,
        [Parameter(Mandatory)][string[]]$Arguments,
        [Parameter(Mandatory)][string]$FailureMessage
    )
    & $Command @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw $FailureMessage
    }
}

function New-PodmanSecret {
    param(
        [Parameter(Mandatory)][string]$Name,
        [Parameter(Mandatory)][string]$Value
    )
    $source = Join-Path $temporaryRoot ".secret-$([Guid]::NewGuid().ToString('N'))"
    try {
        [System.IO.File]::WriteAllText($source, $Value)
        & podman secret create $Name $source *> $null
        if ($LASTEXITCODE -ne 0) {
            throw "Could not create rehearsal secret $Name"
        }
    } finally {
        if (Test-Path -LiteralPath $source) {
            Remove-Item -LiteralPath $source -Force
        }
    }
}

function Wait-ForPostgres {
    param(
        [Parameter(Mandatory)][string]$Container,
        [Parameter(Mandatory)][string]$User,
        [Parameter(Mandatory)][string]$Database
    )
    for ($attempt = 1; $attempt -le 60; $attempt++) {
        & podman exec $Container pg_isready --username $User --dbname $Database *> $null
        if ($LASTEXITCODE -eq 0) {
            return
        }
        Start-Sleep -Milliseconds 500
    }
    throw "PostgreSQL did not become ready: $Container"
}

function Invoke-PostgresCommand {
    param(
        [Parameter(Mandatory)][string]$Network,
        [Parameter(Mandatory)][string]$PasswordSecret,
        [Parameter(Mandatory)][string]$User,
        [Parameter(Mandatory)][string]$Database,
        [Parameter(Mandatory)][string]$SQL
    )
    $secret = "source=$PasswordSecret,target=db_password"
    $command = 'export PGPASSWORD="$(cat /run/secrets/db_password)"; exec psql --host=postgres --username="$1" --dbname="$2" --set=ON_ERROR_STOP=1 --quiet --command="$3"'
    Invoke-Checked -Command "podman" -Arguments @(
        "run", "--rm", "--network", $Network,
        "--secret", $secret,
        $postgresImage,
        "sh", "-eu", "-c", $command, "rehearsal", $User, $Database, $SQL
    ) -FailureMessage "PostgreSQL command failed for $Database"
}

function Get-PostgresScalar {
    param(
        [Parameter(Mandatory)][string]$Network,
        [Parameter(Mandatory)][string]$PasswordSecret,
        [Parameter(Mandatory)][string]$User,
        [Parameter(Mandatory)][string]$Database,
        [Parameter(Mandatory)][string]$SQL
    )
    $secret = "source=$PasswordSecret,target=db_password"
    $command = 'export PGPASSWORD="$(cat /run/secrets/db_password)"; exec psql --host=postgres --username="$1" --dbname="$2" --quiet --tuples-only --no-align --command="$3"'
    $output = @(& podman run --rm --network $Network --secret $secret $postgresImage "sh" "-eu" "-c" $command "rehearsal" $User $Database $SQL)
    if ($LASTEXITCODE -ne 0) {
        throw "Could not read rehearsal value from $Database"
    }
    return ($output -join "`n").Trim()
}

function Invoke-RedisCommand {
    param(
        [Parameter(Mandatory)][string]$Network,
        [Parameter(Mandatory)][string]$PasswordSecret,
        [Parameter(Mandatory)][string[]]$Arguments
    )
    $secret = "source=$PasswordSecret,target=redis_password"
    $command = 'exec redis-cli -h redis -a "$(cat /run/secrets/redis_password)" --no-auth-warning "$@"'
    $output = @(& podman run --rm --network $Network --secret $secret $redisImage "sh" "-eu" "-c" $command "redis" @Arguments)
    if ($LASTEXITCODE -ne 0) {
        throw "Redis command failed for $Network"
    }
    return ($output -join "`n").Trim()
}

function Assert-Equal {
    param(
        [Parameter(Mandatory)]$Actual,
        [Parameter(Mandatory)]$Expected,
        [Parameter(Mandatory)][string]$Message
    )
    if ($Actual -ne $Expected) {
        throw "$Message. Expected '$Expected', got '$Actual'"
    }
}

function Remove-TemporaryRoot {
    $tempBase = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath()).TrimEnd([System.IO.Path]::DirectorySeparatorChar)
    $resolved = [System.IO.Path]::GetFullPath($temporaryRoot)
    $expectedPrefix = $tempBase + [System.IO.Path]::DirectorySeparatorChar + "paigram-recovery-rehearsal-"
    if (-not $resolved.StartsWith($expectedPrefix, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to remove unexpected rehearsal directory $resolved"
    }
    if (Test-Path -LiteralPath $resolved) {
        Remove-Item -LiteralPath $resolved -Recurse -Force
    }
}

New-Item -ItemType Directory -Path $temporaryRoot | Out-Null
$gpgHome = Join-Path $temporaryRoot "gnupg"
$backupDirectory = Join-Path $temporaryRoot "backups"
$secretSource = Join-Path $temporaryRoot "secret-source"
$recoveredSecrets = Join-Path $temporaryRoot "recovered-secrets"
New-Item -ItemType Directory -Path $gpgHome, $backupDirectory, $secretSource | Out-Null
$env:GNUPGHOME = $gpgHome

try {
    foreach ($network in $networks) {
        Invoke-Checked -Command "podman" -Arguments @("network", "create", $network) -FailureMessage "Could not create rehearsal network $network"
    }
    foreach ($secret in $secrets) {
        New-PodmanSecret -Name $secret -Value $testPassword
    }

    Invoke-Checked -Command "podman" -Arguments @(
        "run", "--detach", "--name", "$accountInstance-postgres", "--network", $accountNetwork, "--network-alias", "postgres",
        "--secret", "source=$accountInstance-postgres-password,target=db_password",
        "--env", "POSTGRES_USER=paigram", "--env", "POSTGRES_DB=paigram", "--env", "POSTGRES_PASSWORD_FILE=/run/secrets/db_password",
        $postgresImage
    ) -FailureMessage "Could not start Account rehearsal PostgreSQL"
    Invoke-Checked -Command "podman" -Arguments @(
        "run", "--detach", "--name", "$platformInstance-postgres", "--network", $platformNetwork, "--network-alias", "postgres",
        "--secret", "source=$platformInstance-postgres-password,target=db_password",
        "--env", "POSTGRES_USER=platform_mihomo", "--env", "POSTGRES_DB=platform_mihomo", "--env", "POSTGRES_PASSWORD_FILE=/run/secrets/db_password",
        $postgresImage
    ) -FailureMessage "Could not start Platform rehearsal PostgreSQL"
    foreach ($redis in @(
        @{ Container = "$accountInstance-redis"; Network = $accountNetwork; Secret = "$accountInstance-redis-password" },
        @{ Container = "$platformInstance-redis"; Network = $platformNetwork; Secret = "$platformInstance-redis-password" }
    )) {
        Invoke-Checked -Command "podman" -Arguments @(
            "run", "--detach", "--name", $redis.Container, "--network", $redis.Network, "--network-alias", "redis",
            "--secret", "source=$($redis.Secret),target=redis_password",
            $redisImage, "sh", "-eu", "-c", 'exec redis-server --requirepass "$(cat /run/secrets/redis_password)"'
        ) -FailureMessage "Could not start rehearsal Redis $($redis.Container)"
    }
    foreach ($app in @(
        @{ Name = $accountInstance; Network = $accountNetwork },
        @{ Name = "$accountInstance-frontend"; Network = $accountNetwork },
        @{ Name = $platformInstance; Network = $platformNetwork }
    )) {
        Invoke-Checked -Command "podman" -Arguments @(
            "run", "--detach", "--name", $app.Name, "--network", $app.Network,
            "--label", "org.opencontainers.image.revision=$sourceCommit",
            "--label", "org.paigram.contract-baseline=$contractBaseline",
            "--label", "org.paigram.sdk-version=$sdkVersion",
            $alpineImage, "sleep", "3600"
        ) -FailureMessage "Could not start rehearsal application $($app.Name)"
    }

    Wait-ForPostgres -Container "$accountInstance-postgres" -User "paigram" -Database "paigram"
    Wait-ForPostgres -Container "$platformInstance-postgres" -User "platform_mihomo" -Database "platform_mihomo"
    Invoke-PostgresCommand -Network $accountNetwork -PasswordSecret "$accountInstance-postgres-password" -User "paigram" -Database "paigram" -SQL "CREATE TABLE schema_migrations (version bigint NOT NULL, dirty boolean NOT NULL); INSERT INTO schema_migrations VALUES (1, false); CREATE TABLE recovery_probe (value text NOT NULL); INSERT INTO recovery_probe VALUES ('account-original');"
    Invoke-PostgresCommand -Network $platformNetwork -PasswordSecret "$platformInstance-postgres-password" -User "platform_mihomo" -Database "platform_mihomo" -SQL "CREATE TABLE schema_migrations (version bigint NOT NULL, dirty boolean NOT NULL); INSERT INTO schema_migrations VALUES (1, false); CREATE TABLE recovery_probe (value text NOT NULL); INSERT INTO recovery_probe VALUES ('platform-original');"

    $keyFiles = [ordered]@{
        AccountEncryptionKeyFile = Join-Path $secretSource "account-encryption-key"
        AccountServiceTicketSigningKeyFile = Join-Path $secretSource "account-service-ticket-signing-key.json"
        PlatformEncryptionKeyringFile = Join-Path $secretSource "platform-encryption-keyring.json"
        AccountServiceTicketPublicKeyringFile = Join-Path $secretSource "account-service-ticket-public-keyring.json"
    }
    [System.IO.File]::WriteAllText($keyFiles.AccountEncryptionKeyFile, [Guid]::NewGuid().ToString("N"))
    $ticketKeyJSON = ((@(& go -C (Resolve-Path (Join-Path $PSScriptRoot "../../contracts/runtime/go")) run ./cmd/service-ticket-keygen)) -join "`n") | ConvertFrom-Json
    if ($LASTEXITCODE -ne 0 -or -not $ticketKeyJSON.private_key_pem -or -not $ticketKeyJSON.public_key_pem) {
        throw "Could not generate valid service-ticket recovery keys"
    }
    [ordered]@{
        kid = "rehearsal-ticket"
        private_key_pem = $ticketKeyJSON.private_key_pem
    } | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $keyFiles.AccountServiceTicketSigningKeyFile -Encoding utf8NoBOM
    [ordered]@{
        keys = @([ordered]@{
            kid = "rehearsal-ticket"
            public_key_pem = $ticketKeyJSON.public_key_pem
        })
    } | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $keyFiles.AccountServiceTicketPublicKeyringFile -Encoding utf8NoBOM
    $encryptionKey = [byte[]]::new(32)
    [System.Security.Cryptography.RandomNumberGenerator]::Fill($encryptionKey)
    [ordered]@{
        active_kid = "rehearsal-encryption"
        keys = @([ordered]@{
            kid = "rehearsal-encryption"
            key_base64 = [Convert]::ToBase64String($encryptionKey).TrimEnd("=")
        })
    } | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $keyFiles.PlatformEncryptionKeyringFile -Encoding utf8NoBOM
    $identity = "PaiGram Recovery Rehearsal $suffix <recovery-$suffix@example.invalid>"
    & gpg --batch --passphrase "" --quick-generate-key $identity future-default default 1d
    if ($LASTEXITCODE -ne 0) {
        throw "Could not generate the rehearsal GPG key"
    }
    $fingerprintLine = @(& gpg --batch --with-colons --list-keys $identity) | Where-Object { $_ -like "fpr:*" } | Select-Object -First 1
    if (-not $fingerprintLine) {
        throw "Could not read the rehearsal signing fingerprint"
    }
    $fingerprint = $fingerprintLine.Split(":")[9]

    & (Join-Path $PSScriptRoot "backup.ps1") -BackupDirectory $backupDirectory -GPGRecipient $identity -GPGSigningKey $identity @keyFiles -AccountInstance $accountInstance -PlatformInstance $platformInstance
    if (-not $?) {
        throw "Recovery backup rehearsal failed"
    }
    foreach ($appName in @("$accountInstance-frontend", $accountInstance, $platformInstance)) {
        $running = ((@(& podman inspect --format '{{.State.Running}}' $appName)) -join "`n").Trim()
        Assert-Equal -Actual $running -Expected "true" -Message "Backup did not restart $appName"
    }

    Invoke-PostgresCommand -Network $accountNetwork -PasswordSecret "$accountInstance-postgres-password" -User "paigram" -Database "paigram" -SQL "UPDATE recovery_probe SET value = 'account-mutated';"
    Invoke-PostgresCommand -Network $platformNetwork -PasswordSecret "$platformInstance-postgres-password" -User "platform_mihomo" -Database "platform_mihomo" -SQL "UPDATE recovery_probe SET value = 'platform-mutated';"
    Invoke-RedisCommand -Network $accountNetwork -PasswordSecret "$accountInstance-redis-password" -Arguments @("SET", "recovery-probe", "mutated") | Out-Null
    Invoke-RedisCommand -Network $platformNetwork -PasswordSecret "$platformInstance-redis-password" -Arguments @("SET", "recovery-probe", "mutated") | Out-Null

    $archives = @(Get-ChildItem -LiteralPath $backupDirectory -Filter "*.tar.gpg" -File)
    if ($archives.Count -ne 1) {
        throw "Expected exactly one encrypted rehearsal archive"
    }
    $archive = $archives[0]
    & (Join-Path $PSScriptRoot "restore.ps1") -BackupFile $archive.FullName -ExpectedSignerFingerprint $fingerprint -RecoveredSecretsDirectory $recoveredSecrets -AccountInstance $accountInstance -PlatformInstance $platformInstance -Confirm:$false
    if (-not $?) {
        throw "Recovery restore rehearsal failed"
    }

    Assert-Equal -Actual (Get-PostgresScalar -Network $accountNetwork -PasswordSecret "$accountInstance-postgres-password" -User "paigram" -Database "paigram" -SQL "SELECT value FROM recovery_probe") -Expected "account-original" -Message "Account database was not restored"
    Assert-Equal -Actual (Get-PostgresScalar -Network $platformNetwork -PasswordSecret "$platformInstance-postgres-password" -User "platform_mihomo" -Database "platform_mihomo" -SQL "SELECT value FROM recovery_probe") -Expected "platform-original" -Message "Platform database was not restored"
    Assert-Equal -Actual (Invoke-RedisCommand -Network $accountNetwork -PasswordSecret "$accountInstance-redis-password" -Arguments @("DBSIZE")) -Expected "0" -Message "Account Redis was not cleared"
    Assert-Equal -Actual (Invoke-RedisCommand -Network $platformNetwork -PasswordSecret "$platformInstance-redis-password" -Arguments @("DBSIZE")) -Expected "0" -Message "Platform Redis was not cleared"
    foreach ($entry in $keyFiles.GetEnumerator()) {
        $recoveredName = switch ($entry.Key) {
            "AccountEncryptionKeyFile" { "account-encryption-key" }
            "AccountServiceTicketSigningKeyFile" { "account-service-ticket-signing-key.json" }
            "PlatformEncryptionKeyringFile" { "platform-encryption-keyring.json" }
            "AccountServiceTicketPublicKeyringFile" { "account-service-ticket-public-keyring.json" }
        }
        Assert-Equal -Actual ([System.IO.File]::ReadAllText((Join-Path $recoveredSecrets $recoveredName))) -Expected ([System.IO.File]::ReadAllText($entry.Value)) -Message "Recovered key material differs for $recoveredName"
    }
    foreach ($appName in @("$accountInstance-frontend", $accountInstance, $platformInstance)) {
        $running = ((@(& podman inspect --format '{{.State.Running}}' $appName)) -join "`n").Trim()
        Assert-Equal -Actual $running -Expected "false" -Message "Restore did not leave $appName stopped"
    }
    Write-Host "Storage recovery rehearsal passed for both PostgreSQL databases, both Redis databases, and all recovered key files."
} finally {
    foreach ($container in $containers) {
        & podman rm --force $container *> $null
    }
    foreach ($secret in $secrets) {
        & podman secret rm $secret *> $null
    }
    foreach ($network in $networks) {
        & podman network rm $network *> $null
    }
    if ($null -eq $originalGPGHome) {
        Remove-Item Env:GNUPGHOME -ErrorAction SilentlyContinue
    } else {
        $env:GNUPGHOME = $originalGPGHome
    }
    Remove-TemporaryRoot
}
