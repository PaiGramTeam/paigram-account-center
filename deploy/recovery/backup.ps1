#requires -Version 7.4

[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [string]$BackupDirectory,
    [Parameter(Mandatory)]
    [string]$GPGRecipient,
    [Parameter(Mandatory)]
    [string]$GPGSigningKey,
    [Parameter(Mandatory)]
    [string]$AccountEncryptionKeyFile,
    [Parameter(Mandatory)]
    [string]$AccountServiceTicketSigningKeyFile,
    [Parameter(Mandatory)]
    [string]$PlatformEncryptionKeyringFile,
    [Parameter(Mandatory)]
    [string]$AccountServiceTicketPublicKeyringFile,
    [string]$AccountInstance = "paigram-account-center",
    [string]$PlatformInstance = "paigram-platform-mihomo"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
Import-Module (Join-Path $PSScriptRoot "RecoveryKeyMaterial.psm1") -Force

$postgresImage = "docker.io/library/postgres:16-alpine@sha256:57c72fd2a128e416c7fcc499958864df5301e940bca0a56f58fddf30ffc07777"
$requiredFiles = @{
    "account-encryption-key" = $AccountEncryptionKeyFile
    "account-service-ticket-signing-key.json" = $AccountServiceTicketSigningKeyFile
    "platform-encryption-keyring.json" = $PlatformEncryptionKeyringFile
    "account-service-ticket-public-keyring.json" = $AccountServiceTicketPublicKeyringFile
}

function Assert-Command {
    param([Parameter(Mandatory)][string]$Name)
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "$Name is required"
    }
}

function Assert-InstanceName {
    param([Parameter(Mandatory)][string]$Name)
    if ($Name -notmatch '^[a-z0-9][a-z0-9-]*$') {
        throw "Instance names must contain only lowercase letters, digits, and hyphens"
    }
}

function Assert-ImmutableImageReference {
    param([Parameter(Mandatory)][string]$Reference)
    if ($Reference -notmatch '^\S+@sha256:[a-f0-9]{64}$') {
        throw "Running application image is not pinned by manifest digest: $Reference"
    }
}

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

function Invoke-DatabaseDump {
    param(
        [Parameter(Mandatory)][string]$Network,
        [Parameter(Mandatory)][string]$PasswordSecret,
        [Parameter(Mandatory)][string]$User,
        [Parameter(Mandatory)][string]$Database,
        [Parameter(Mandatory)][string]$OutputName,
        [Parameter(Mandatory)][string]$OutputDirectory
    )
    $volume = "${OutputDirectory}:/backup"
    $secret = "source=$PasswordSecret,target=db_password"
    $command = 'export PGPASSWORD="$(cat /run/secrets/db_password)"; exec pg_dump --host=postgres --username="$1" --dbname="$2" --format=custom --compress=9 --no-owner --no-privileges --file="/backup/$3"'
    Invoke-Checked -Command "podman" -Arguments @(
        "run", "--rm", "--network", $Network,
        "--secret", $secret,
        "--volume", $volume,
        $postgresImage,
        "sh", "-eu", "-c", $command, "backup", $User, $Database, $OutputName
    ) -FailureMessage "PostgreSQL backup failed for $Database"
}

function Get-DatabaseMigrationState {
    param(
        [Parameter(Mandatory)][string]$Network,
        [Parameter(Mandatory)][string]$PasswordSecret,
        [Parameter(Mandatory)][string]$User,
        [Parameter(Mandatory)][string]$Database
    )
    $secret = "source=$PasswordSecret,target=db_password"
    $command = 'export PGPASSWORD="$(cat /run/secrets/db_password)"; exec psql --host=postgres --username="$1" --dbname="$2" --quiet --tuples-only --no-align --command="SELECT version::text || '':'' || dirty::text FROM schema_migrations ORDER BY version DESC LIMIT 1"'
    $output = @(& podman run --rm --network $Network --secret $secret $postgresImage "sh" "-eu" "-c" $command "migration" $User $Database)
    if ($LASTEXITCODE -ne 0) {
        throw "Could not query the migration state for $Database"
    }
    $state = ($output -join "`n").Trim()
    if ($state -notmatch '^\d+:false$') {
        throw "Database $Database does not have a clean migration state"
    }
    return $state
}

function Test-ContainerExists {
    param([Parameter(Mandatory)][string]$Name)
    & podman container exists $Name
    if ($LASTEXITCODE -eq 0) {
        return $true
    }
    if ($LASTEXITCODE -eq 1) {
        return $false
    }
    throw "Could not inspect container $Name"
}

function Get-ContainerMetadata {
    param([Parameter(Mandatory)][string]$Name)
    if (-not (Test-ContainerExists -Name $Name)) {
        throw "Required application container does not exist: $Name"
    }
    $imageReference = ((@(& podman inspect --format '{{.ImageName}}' $Name)) -join "`n").Trim()
    if ($LASTEXITCODE -ne 0) {
        throw "Could not inspect the image for $Name"
    }
    Assert-ImmutableImageReference -Reference $imageReference
    $sourceCommit = ((@(& podman inspect --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}' $Name)) -join "`n").Trim()
    if ($LASTEXITCODE -ne 0 -or $sourceCommit -notmatch '^[a-f0-9]{40,64}$') {
        throw "Container $Name does not expose a valid org.opencontainers.image.revision label"
    }
    $contractBaseline = ((@(& podman inspect --format '{{ index .Config.Labels "org.paigram.contract-baseline" }}' $Name)) -join "`n").Trim()
    if ($LASTEXITCODE -ne 0 -or $contractBaseline -notmatch '^[a-f0-9]{40,64}$') {
        throw "Container $Name does not expose a valid org.paigram.contract-baseline label"
    }
    $sdkVersion = ((@(& podman inspect --format '{{ index .Config.Labels "org.paigram.sdk-version" }}' $Name)) -join "`n").Trim()
    if ($LASTEXITCODE -ne 0 -or $sdkVersion -notmatch '^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$') {
        throw "Container $Name does not expose a valid org.paigram.sdk-version label"
    }
    return [ordered]@{
        container = $Name
        reference = $imageReference
        source_commit = $sourceCommit
        contract_baseline = $contractBaseline
        sdk_version = $sdkVersion
    }
}

function Get-ContainerRunningState {
    param([Parameter(Mandatory)][string]$Name)
    $running = ((@(& podman inspect --format '{{.State.Running}}' $Name)) -join "`n").Trim()
    if ($LASTEXITCODE -ne 0 -or $running -notin @("true", "false")) {
        throw "Could not inspect the running state for $Name"
    }
    return $running -eq "true"
}

function Remove-StagingDirectory {
    param(
        [Parameter(Mandatory)][string]$Path,
        [Parameter(Mandatory)][string]$Root
    )
    $fullRoot = [System.IO.Path]::GetFullPath($Root).TrimEnd([System.IO.Path]::DirectorySeparatorChar)
    $fullPath = [System.IO.Path]::GetFullPath($Path)
    $expectedPrefix = $fullRoot + [System.IO.Path]::DirectorySeparatorChar + ".paigram-backup-"
    if (-not $fullPath.StartsWith($expectedPrefix, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to remove unexpected staging directory $fullPath"
    }
    if (Test-Path -LiteralPath $fullPath) {
        Remove-Item -LiteralPath $fullPath -Recurse -Force
    }
}

function Set-PrivateFileMode {
    param([Parameter(Mandatory)][string]$Path)
    if (-not $IsWindows) {
        [System.IO.File]::SetUnixFileMode(
            $Path,
            [System.IO.UnixFileMode]::UserRead -bor [System.IO.UnixFileMode]::UserWrite
        )
    }
}

function Set-PrivateDirectoryMode {
    param([Parameter(Mandatory)][string]$Path)
    if ($IsWindows) {
        $identity = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
        Invoke-Checked -Command "icacls" -Arguments @($Path, "/inheritance:r", "/grant:r", "${identity}:(OI)(CI)(F)") -FailureMessage "Could not restrict backup staging permissions"
        return
    }
    [System.IO.Directory]::SetUnixFileMode(
        $Path,
        [System.IO.UnixFileMode]::UserRead -bor [System.IO.UnixFileMode]::UserWrite -bor [System.IO.UnixFileMode]::UserExecute
    )
}

Assert-Command -Name "podman"
Assert-Command -Name "gpg"
Assert-Command -Name "tar"
Assert-InstanceName -Name $AccountInstance
Assert-InstanceName -Name $PlatformInstance
foreach ($source in $requiredFiles.Values) {
    if (-not (Test-Path -LiteralPath $source -PathType Leaf)) {
        throw "Required key material is missing: $source"
    }
}
Invoke-Checked -Command "gpg" -Arguments @("--batch", "--list-keys", $GPGRecipient) -FailureMessage "GPG recipient is unavailable"
Invoke-Checked -Command "gpg" -Arguments @("--batch", "--list-secret-keys", $GPGSigningKey) -FailureMessage "GPG signing key is unavailable"

if (-not (Test-Path -LiteralPath $BackupDirectory)) {
    New-Item -ItemType Directory -Path $BackupDirectory | Out-Null
}
$backupRoot = (Resolve-Path -LiteralPath $BackupDirectory).Path
$stamp = [DateTime]::UtcNow.ToString("yyyyMMddTHHmmssZ")
$backupName = "paigram-recovery-$stamp"
$staging = Join-Path $backupRoot ".paigram-backup-$([Guid]::NewGuid().ToString('N'))"
$payload = Join-Path $staging "payload"
$archive = Join-Path $staging "$backupName.tar"
$encryptedArchive = Join-Path $backupRoot "$backupName.tar.gpg"
if (Test-Path -LiteralPath $encryptedArchive) {
    throw "Backup already exists: $encryptedArchive"
}
New-Item -ItemType Directory -Path $staging | Out-Null
Set-PrivateDirectoryMode -Path $staging
New-Item -ItemType Directory -Path $payload | Out-Null
$backupCompleted = $false

try {
    $images = [ordered]@{
        account = Get-ContainerMetadata -Name $AccountInstance
        frontend = Get-ContainerMetadata -Name "$AccountInstance-frontend"
        platform = Get-ContainerMetadata -Name $PlatformInstance
    }
    $containersToRestart = [System.Collections.Generic.List[string]]::new()
    $maintenanceStartedAt = [DateTime]::UtcNow
    try {
        foreach ($container in @("$AccountInstance-frontend", $AccountInstance, $PlatformInstance)) {
            if (Get-ContainerRunningState -Name $container) {
                Invoke-Checked -Command "podman" -Arguments @("stop", $container) -FailureMessage "Could not stop $container for the backup window"
                $containersToRestart.Add($container)
            }
        }

        $accountMigration = Get-DatabaseMigrationState -Network $AccountInstance -PasswordSecret "$AccountInstance-postgres-password" -User "paigram" -Database "paigram"
        $platformMigration = Get-DatabaseMigrationState -Network "$PlatformInstance-private" -PasswordSecret "$PlatformInstance-postgres-password" -User "platform_mihomo" -Database "platform_mihomo"
        Invoke-DatabaseDump -Network $AccountInstance -PasswordSecret "$AccountInstance-postgres-password" -User "paigram" -Database "paigram" -OutputName "account-center.dump" -OutputDirectory $payload
        Invoke-DatabaseDump -Network "$PlatformInstance-private" -PasswordSecret "$PlatformInstance-postgres-password" -User "platform_mihomo" -Database "platform_mihomo" -OutputName "platform-mihomo.dump" -OutputDirectory $payload

        foreach ($entry in $requiredFiles.GetEnumerator()) {
            Copy-Item -LiteralPath $entry.Value -Destination (Join-Path $payload $entry.Key)
        }
        Assert-RecoveryKeyMaterial `
            -AccountEncryptionKeyFile (Join-Path $payload "account-encryption-key") `
            -AccountServiceTicketSigningKeyFile (Join-Path $payload "account-service-ticket-signing-key.json") `
            -PlatformEncryptionKeyringFile (Join-Path $payload "platform-encryption-keyring.json") `
            -AccountServiceTicketPublicKeyringFile (Join-Path $payload "account-service-ticket-public-keyring.json") `
            -WorkingDirectory $payload
        $maintenanceCompletedAt = [DateTime]::UtcNow
    } finally {
        $restartFailures = [System.Collections.Generic.List[string]]::new()
        for ($index = $containersToRestart.Count - 1; $index -ge 0; $index--) {
            $container = $containersToRestart[$index]
            & podman start $container
            if ($LASTEXITCODE -ne 0) {
                $restartFailures.Add($container)
            }
        }
        if ($restartFailures.Count -gt 0) {
            throw "Could not restart application containers: $($restartFailures -join ', ')"
        }
    }

    $fileRecords = Get-ChildItem -LiteralPath $payload -File | Sort-Object Name | ForEach-Object {
        [ordered]@{
            name = $_.Name
            bytes = $_.Length
            sha256 = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        }
    }
    $manifest = [ordered]@{
        format_version = 1
        created_at = [DateTime]::UtcNow.ToString("O")
        maintenance_started_at = $maintenanceStartedAt.ToString("O")
        maintenance_completed_at = $maintenanceCompletedAt.ToString("O")
        account_instance = $AccountInstance
        platform_instance = $PlatformInstance
        migrations = [ordered]@{
            account = $accountMigration
            platform = $platformMigration
        }
        images = $images
        files = @($fileRecords)
    }
    $manifest | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $payload "manifest.json") -Encoding utf8NoBOM

    New-Item -ItemType File -Path $archive | Out-Null
    Set-PrivateFileMode -Path $archive
    Invoke-Checked -Command "tar" -Arguments @("-C", $payload, "-cf", $archive, ".") -FailureMessage "Could not create recovery archive"
    Invoke-Checked -Command "gpg" -Arguments @("--batch", "--yes", "--trust-model", "always", "--local-user", $GPGSigningKey, "--recipient", $GPGRecipient, "--output", $encryptedArchive, "--sign", "--encrypt", $archive) -FailureMessage "Could not sign and encrypt recovery archive"
    $backupCompleted = $true
    Write-Host "Encrypted recovery backup created at $encryptedArchive"
    Write-Host "Encrypted archive SHA-256: $((Get-FileHash -LiteralPath $encryptedArchive -Algorithm SHA256).Hash.ToLowerInvariant())"
} finally {
    if (-not $backupCompleted -and (Test-Path -LiteralPath $encryptedArchive)) {
        Remove-Item -LiteralPath $encryptedArchive -Force
    }
    Remove-StagingDirectory -Path $staging -Root $backupRoot
}
