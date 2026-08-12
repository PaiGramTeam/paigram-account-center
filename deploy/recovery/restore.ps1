#requires -Version 7.4

[CmdletBinding(SupportsShouldProcess, ConfirmImpact = "High")]
param(
    [Parameter(Mandatory)]
    [string]$BackupFile,
    [Parameter(Mandatory)]
    [string]$RecoveredSecretsDirectory,
    [Parameter(Mandatory)]
    [ValidatePattern('^[A-Fa-f0-9]{40,64}$')]
    [string]$ExpectedSignerFingerprint,
    [string]$AccountInstance = "paigram-account-center",
    [string]$PlatformInstance = "paigram-platform-mihomo"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
Import-Module (Join-Path $PSScriptRoot "RecoveryKeyMaterial.psm1") -Force

$postgresImage = "docker.io/library/postgres:16-alpine@sha256:57c72fd2a128e416c7fcc499958864df5301e940bca0a56f58fddf30ffc07777"
$redisImage = "docker.io/library/redis:7-alpine@sha256:e7723ff73d963f5cc6d9c4643ea3d989527a402a319239054e9472a7fb9219a2"
$secretFiles = @(
    "account-encryption-key",
    "account-service-ticket-signing-key.json",
    "platform-encryption-keyring.json",
    "account-service-ticket-public-keyring.json"
)

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

function Invoke-DatabaseRestore {
    param(
        [Parameter(Mandatory)][string]$Network,
        [Parameter(Mandatory)][string]$PasswordSecret,
        [Parameter(Mandatory)][string]$User,
        [Parameter(Mandatory)][string]$Database,
        [Parameter(Mandatory)][string]$DumpName,
        [Parameter(Mandatory)][string]$InputDirectory
    )
    $volume = "${InputDirectory}:/recovery:ro"
    $secret = "source=$PasswordSecret,target=db_password"
    $command = 'export PGPASSWORD="$(cat /run/secrets/db_password)"; exec pg_restore --host=postgres --username="$1" --dbname="$2" --clean --if-exists --exit-on-error --single-transaction --no-owner --no-privileges "/recovery/$3"'
    Invoke-Checked -Command "podman" -Arguments @(
        "run", "--rm", "--network", $Network,
        "--secret", $secret,
        "--volume", $volume,
        $postgresImage,
        "sh", "-eu", "-c", $command, "restore", $User, $Database, $DumpName
    ) -FailureMessage "PostgreSQL restore failed for $Database"
}

function Clear-RedisDatabase {
    param(
        [Parameter(Mandatory)][string]$Network,
        [Parameter(Mandatory)][string]$PasswordSecret
    )
    $secret = "source=$PasswordSecret,target=redis_password"
    $command = 'exec redis-cli -h redis -a "$(cat /run/secrets/redis_password)" --no-auth-warning FLUSHDB'
    Invoke-Checked -Command "podman" -Arguments @(
        "run", "--rm", "--network", $Network,
        "--secret", $secret,
        $redisImage,
        "sh", "-eu", "-c", $command
    ) -FailureMessage "Redis cache reset failed for $Network"
}

function Remove-StagingDirectory {
    param(
        [Parameter(Mandatory)][string]$Path,
        [Parameter(Mandatory)][string]$Root
    )
    $fullRoot = [System.IO.Path]::GetFullPath($Root).TrimEnd([System.IO.Path]::DirectorySeparatorChar)
    $fullPath = [System.IO.Path]::GetFullPath($Path)
    $expectedPrefix = $fullRoot + [System.IO.Path]::DirectorySeparatorChar + ".paigram-restore-"
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
        Invoke-Checked -Command "icacls" -Arguments @($Path, "/inheritance:r", "/grant:r", "${identity}:(OI)(CI)(F)") -FailureMessage "Could not restrict recovered secret directory permissions"
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
if (-not (Test-Path -LiteralPath $BackupFile -PathType Leaf)) {
    throw "Encrypted recovery backup does not exist: $BackupFile"
}
if (Test-Path -LiteralPath $RecoveredSecretsDirectory) {
    throw "RecoveredSecretsDirectory must not already exist"
}
if (-not $PSCmdlet.ShouldProcess("$AccountInstance and $PlatformInstance", "Stop application services and replace both PostgreSQL databases")) {
    return
}

$backupPath = (Resolve-Path -LiteralPath $BackupFile).Path
$workingRoot = Split-Path -Parent $backupPath
$staging = Join-Path $workingRoot ".paigram-restore-$([Guid]::NewGuid().ToString('N'))"
New-Item -ItemType Directory -Path $staging | Out-Null
Set-PrivateDirectoryMode -Path $staging
$archive = Join-Path $staging "archive.tar"
New-Item -ItemType File -Path $archive | Out-Null
Set-PrivateFileMode -Path $archive

try {
    $signatureStatus = @(& gpg --batch --yes --status-fd 1 --output $archive --decrypt $backupPath)
    if ($LASTEXITCODE -ne 0) {
        throw "Could not decrypt and verify recovery backup"
    }
    $validSigner = @($signatureStatus | ForEach-Object {
        if ($_ -match '^\[GNUPG:\] VALIDSIG ([A-Fa-f0-9]+) ') {
            $Matches[1].ToUpperInvariant()
        }
    })
    if ($ExpectedSignerFingerprint.ToUpperInvariant() -notin $validSigner) {
        throw "Recovery backup does not have the expected valid signature"
    }
    $archiveEntries = @(& tar -tf $archive)
    if ($LASTEXITCODE -ne 0) {
        throw "Could not inspect recovery archive"
    }
    foreach ($entry in $archiveEntries) {
        $normalized = $entry.Replace('\', '/')
        if ([System.IO.Path]::IsPathRooted($entry) -or $normalized -match '(^|/)\.\.(/|$)') {
            throw "Recovery archive contains an unsafe path"
        }
    }
    Invoke-Checked -Command "tar" -Arguments @("-C", $staging, "-xf", $archive) -FailureMessage "Could not extract recovery backup"

    $manifestPath = Join-Path $staging "manifest.json"
    if (-not (Test-Path -LiteralPath $manifestPath -PathType Leaf)) {
        throw "Recovery manifest is missing"
    }
    $manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
    if ($manifest.format_version -ne 1) {
        throw "Unsupported recovery format version"
    }
    foreach ($migration in @($manifest.migrations.account, $manifest.migrations.platform)) {
        if ([string]$migration -notmatch '^\d+:false$') {
            throw "Recovery manifest does not contain clean migration states"
        }
    }
    foreach ($image in @($manifest.images.account, $manifest.images.frontend, $manifest.images.platform)) {
        if ([string]$image.reference -notmatch '^\S+@sha256:[a-f0-9]{64}$' -or
            [string]$image.source_commit -notmatch '^[a-f0-9]{40,64}$' -or
            [string]$image.contract_baseline -notmatch '^[a-f0-9]{40,64}$' -or
            [string]$image.sdk_version -notmatch '^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$') {
            throw "Recovery manifest contains invalid image provenance"
        }
    }
    $seenFiles = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::Ordinal)
    foreach ($record in $manifest.files) {
        if ([System.IO.Path]::GetFileName([string]$record.name) -ne [string]$record.name) {
            throw "Recovery manifest contains an unsafe file name"
        }
        if (-not $seenFiles.Add([string]$record.name)) {
            throw "Recovery manifest contains a duplicate file name"
        }
        $path = Join-Path $staging $record.name
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
            throw "Recovery file is missing: $($record.name)"
        }
        $actualHash = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($actualHash -ne $record.sha256 -or (Get-Item -LiteralPath $path).Length -ne $record.bytes) {
            throw "Recovery file failed integrity validation: $($record.name)"
        }
    }
    foreach ($name in @("account-center.dump", "platform-mihomo.dump") + $secretFiles) {
        if (-not (Test-Path -LiteralPath (Join-Path $staging $name) -PathType Leaf)) {
            throw "Required recovery file is missing: $name"
        }
    }
    Assert-RecoveryKeyMaterial `
        -AccountEncryptionKeyFile (Join-Path $staging "account-encryption-key") `
        -AccountServiceTicketSigningKeyFile (Join-Path $staging "account-service-ticket-signing-key.json") `
        -PlatformEncryptionKeyringFile (Join-Path $staging "platform-encryption-keyring.json") `
        -AccountServiceTicketPublicKeyringFile (Join-Path $staging "account-service-ticket-public-keyring.json") `
        -WorkingDirectory $staging

    New-Item -ItemType Directory -Path $RecoveredSecretsDirectory | Out-Null
    Set-PrivateDirectoryMode -Path $RecoveredSecretsDirectory
    foreach ($name in $secretFiles) {
        $destination = Join-Path $RecoveredSecretsDirectory $name
        Copy-Item -LiteralPath (Join-Path $staging $name) -Destination $destination
        Set-PrivateFileMode -Path $destination
    }
    $recoveredManifest = Join-Path $RecoveredSecretsDirectory "recovery-manifest.json"
    Copy-Item -LiteralPath $manifestPath -Destination $recoveredManifest
    Set-PrivateFileMode -Path $recoveredManifest

    foreach ($container in @("$AccountInstance-frontend", $AccountInstance, $PlatformInstance)) {
        if (Test-ContainerExists -Name $container) {
            Invoke-Checked -Command "podman" -Arguments @("stop", $container) -FailureMessage "Could not stop $container"
        }
    }

    Clear-RedisDatabase -Network $AccountInstance -PasswordSecret "$AccountInstance-redis-password"
    Clear-RedisDatabase -Network "$PlatformInstance-private" -PasswordSecret "$PlatformInstance-redis-password"
    Invoke-DatabaseRestore -Network $AccountInstance -PasswordSecret "$AccountInstance-postgres-password" -User "paigram" -Database "paigram" -DumpName "account-center.dump" -InputDirectory $staging
    Invoke-DatabaseRestore -Network "$PlatformInstance-private" -PasswordSecret "$PlatformInstance-postgres-password" -User "platform_mihomo" -Database "platform_mihomo" -DumpName "platform-mihomo.dump" -InputDirectory $staging

    Write-Host "Database restore completed. Re-provision the recovered secret files, force-recreate the frontend and both application containers, and run the production tracer before reopening ingress."
} finally {
    Remove-StagingDirectory -Path $staging -Root $workingRoot
}
