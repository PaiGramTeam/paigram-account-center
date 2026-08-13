#requires -Version 7.4

[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$RecoveredSecretsDirectory,
    [Parameter(Mandatory)][string]$TracerConfigFile,
    [Parameter(Mandatory)][string]$EvidenceFile,
    [string]$AccountInstance = "paigram-account-center",
    [string]$PlatformInstance = "paigram-platform-mihomo",
    [string]$PlatformNetwork = "paigram-platform-backplane"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
Import-Module (Join-Path $PSScriptRoot "RestoredReleaseSupport.psm1") -Force

$requiredRecoveredFiles = [ordered]@{
    AccountEncryption = "account-encryption-key"
    TicketSigning = "account-service-ticket-signing-key.json"
    PlatformEncryption = "platform-encryption-keyring.json"
    TicketPublic = "account-service-ticket-public-keyring.json"
}
$postgresImage = "docker.io/library/postgres:16-alpine@sha256:57c72fd2a128e416c7fcc499958864df5301e940bca0a56f58fddf30ffc07777"

function Get-MigrationState {
    param(
        [Parameter(Mandatory)][string]$Network,
        [Parameter(Mandatory)][string]$PasswordSecret,
        [Parameter(Mandatory)][string]$User,
        [Parameter(Mandatory)][string]$Database
    )
    $secret = "source=$PasswordSecret,target=db_password"
    $command = 'export PGPASSWORD="$(cat /run/secrets/db_password)"; exec psql --host=postgres --username="$1" --dbname="$2" --quiet --tuples-only --no-align --command="SELECT version::text || '':'' || dirty::text FROM schema_migrations ORDER BY version DESC LIMIT 1"'
    return Invoke-RecoveryPodmanText -Arguments @(
        "run", "--rm", "--network", $Network,
        "--secret", $secret,
        $postgresImage,
        "sh", "-eu", "-c", $command, "migration", $User, $Database
    ) -FailureMessage "Could not read migration state for $Database"
}

function Get-RuntimeRoute {
    param(
        [Parameter(Mandatory)][string]$Network,
        [Parameter(Mandatory)][string]$PasswordSecret
    )
    $secret = "source=$PasswordSecret,target=db_password"
    $command = 'export PGPASSWORD="$(cat /run/secrets/db_password)"; exec psql --host=postgres --username=paigram --dbname=paigram --quiet --tuples-only --no-align --command="SELECT runtime_endpoint || ''|'' || runtime_server_name FROM platform_services WHERE service_key = ''platform-mihomo-service'' AND enabled = true"'
    $output = Invoke-RecoveryPodmanText -Arguments @(
        "run", "--rm", "--network", $Network,
        "--secret", $secret,
        $postgresImage,
        "sh", "-eu", "-c", $command
    ) -FailureMessage "Could not read the restored Platform runtime route"
    $lines = @($output -split "`n" | Where-Object { $_ })
    if ($lines.Count -ne 1) {
        throw "Restored Account database must contain exactly one enabled Mihomo runtime route"
    }
    $parts = $lines[0] -split '\|', 2
    if ($parts.Count -ne 2 -or $parts[1] -notmatch '^[A-Za-z0-9.-]{1,253}$') {
        throw "Restored Platform runtime route is invalid"
    }
    return [ordered]@{ endpoint = $parts[0]; server_name = $parts[1] }
}

function Assert-MountedSecret {
    param(
        [Parameter(Mandatory)][string]$Container,
        [Parameter(Mandatory)][string]$MountedPath,
        [Parameter(Mandatory)][string]$RecoveredPath
    )
    $expected = (Get-FileHash -LiteralPath $RecoveredPath -Algorithm SHA256).Hash.ToLowerInvariant()
    $output = Invoke-RecoveryPodmanText -Arguments @("exec", $Container, "sha256sum", $MountedPath) -FailureMessage "Could not hash mounted secret $MountedPath"
    $actual = ($output -split '\s+', 2)[0].ToLowerInvariant()
    Assert-RecoveryEqual -Actual $actual -Expected $expected -Message "$Container mounted secret $MountedPath"
}

Assert-RecoveryCommand -Name "podman"
Assert-RecoveryCommand -Name "uv"
Assert-RecoveryCommand -Name "git"
Assert-RecoveryInstanceName -Name $AccountInstance
Assert-RecoveryInstanceName -Name $PlatformInstance
Assert-RecoveryInstanceName -Name $PlatformNetwork

if (-not (Test-Path -LiteralPath $RecoveredSecretsDirectory -PathType Container)) {
    throw "Recovered secrets directory does not exist"
}
if (-not (Test-Path -LiteralPath $TracerConfigFile -PathType Leaf)) {
    throw "Private tracer config file does not exist"
}
$recoveredRoot = (Resolve-Path -LiteralPath $RecoveredSecretsDirectory).Path
$repositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot "../..")).Path
$tracerConfig = Assert-RecoveryPrivateFile -Path $TracerConfigFile -RepositoryRoot $repositoryRoot
$evidenceParent = Split-Path -Parent ([System.IO.Path]::GetFullPath($EvidenceFile))
if (-not (Test-Path -LiteralPath $evidenceParent -PathType Container)) {
    throw "Evidence output directory does not exist"
}
Assert-RecoveryPathHasNoLinks -Path $evidenceParent
$evidencePath = [System.IO.Path]::GetFullPath($EvidenceFile)
if (Test-Path -LiteralPath $evidencePath) {
    throw "Evidence file must be a new path for this verification run"
}
if ($evidencePath.StartsWith($recoveredRoot + [System.IO.Path]::DirectorySeparatorChar, [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "Evidence file must be outside the recovered secrets directory"
}
$manifestPath = Join-Path $recoveredRoot "recovery-manifest.json"
if (-not (Test-Path -LiteralPath $manifestPath -PathType Leaf)) {
    throw "Recovered recovery-manifest.json is missing"
}
foreach ($name in $requiredRecoveredFiles.Values) {
    if (-not (Test-Path -LiteralPath (Join-Path $recoveredRoot $name) -PathType Leaf)) {
        throw "Recovered secret file is missing: $name"
    }
}

$manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
if ($manifest.format_version -ne 1) {
    throw "Unsupported recovery manifest format"
}
foreach ($image in @($manifest.images.account, $manifest.images.frontend, $manifest.images.platform)) {
    Assert-RecoveryImageReference -Reference ([string]$image.reference)
    if ([string]$image.source_commit -notmatch '^[a-f0-9]{40,64}$' -or
        [string]$image.contract_baseline -notmatch '^[a-f0-9]{40,64}$' -or
        [string]$image.sdk_version -notmatch '^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$') {
        throw "Recovery manifest image provenance is invalid"
    }
}
foreach ($migration in @($manifest.migrations.account, $manifest.migrations.platform)) {
    if ([string]$migration -notmatch '^\d+:false$') {
        throw "Recovery manifest does not contain clean migration states"
    }
}

Assert-RecoveryContainerMetadata -Name $AccountInstance -Expected $manifest.images.account
Assert-RecoveryContainerMetadata -Name "$AccountInstance-frontend" -Expected $manifest.images.frontend
Assert-RecoveryContainerMetadata -Name $PlatformInstance -Expected $manifest.images.platform
Assert-RecoveryContainerDeployment `
    -Name "$AccountInstance-postgres" `
    -Project $AccountInstance `
    -Service "postgres" `
    -PostgresVolume "$AccountInstance-postgres-data" `
    -Networks @($AccountInstance)
Assert-RecoveryContainerDeployment `
    -Name "$PlatformInstance-postgres" `
    -Project $PlatformInstance `
    -Service "postgres" `
    -PostgresVolume "$PlatformInstance-postgres-data" `
    -Networks @("$PlatformInstance-private")
Assert-RecoveryServiceConnection `
    -Container $AccountInstance `
    -Project $AccountInstance `
    -Service "account-center" `
    -Networks @($AccountInstance, $PlatformNetwork) `
    -DSNFile "/run/secrets/$AccountInstance-database-dsn" `
    -ExpectedDatabase "paigram"
Assert-RecoveryServiceConnection `
    -Container $PlatformInstance `
    -Project $PlatformInstance `
    -Service "platform-mihomo" `
    -Networks @("$PlatformInstance-private", $PlatformNetwork) `
    -DSNFile "/run/secrets/$PlatformInstance-database-dsn" `
    -ExpectedDatabase "platform_mihomo"

$sdkDirectory = (Resolve-Path (Join-Path $repositoryRoot "sdks/python")).Path
$sourceCommits = @(@(
        [string]$manifest.images.account.source_commit,
        [string]$manifest.images.frontend.source_commit,
        [string]$manifest.images.platform.source_commit
    ) | Select-Object -Unique)
if ($sourceCommits.Count -ne 1) {
    throw "Recovery manifest application images do not share one source commit"
}
$global:LASTEXITCODE = 0
$localCommit = ((@(& git -C $repositoryRoot rev-parse HEAD)) -join "`n").Trim()
if ($LASTEXITCODE -ne 0) {
    throw "Could not resolve the verifier source commit"
}
Assert-RecoveryEqual -Actual $localCommit -Expected $sourceCommits[0] -Message "Verifier source commit"
$trustedPaths = @("deploy/recovery", "deploy/podman", "deploy/podman-platform-mihomo", "deploy/PodmanCompose.psm1", "deploy/DeploymentComposition.psm1", "deploy/OperationalSecurity.psm1", "sdks/python")
$global:LASTEXITCODE = 0
$trustedStatus = ((@(& git -C $repositoryRoot status --porcelain --untracked-files=all -- $trustedPaths)) -join "`n").Trim()
if ($LASTEXITCODE -ne 0 -or $trustedStatus) {
    throw "Recovery verifier and Python SDK checkout must match the recorded source commit without local changes"
}
$pyproject = Get-Content -LiteralPath (Join-Path $sdkDirectory "pyproject.toml") -Raw
if ($pyproject -notmatch '(?m)^version\s*=\s*"([^"]+)"\s*$') {
    throw "Could not read the Python SDK version"
}
$localSDKVersion = $Matches[1]
foreach ($image in @($manifest.images.account, $manifest.images.frontend, $manifest.images.platform)) {
    Assert-RecoveryEqual -Actual $localSDKVersion -Expected $image.sdk_version -Message "Python SDK version"
}
$global:LASTEXITCODE = 0
& uv lock --project $sdkDirectory --check
if ($LASTEXITCODE -ne 0) {
    throw "Python SDK lock file is stale"
}

$accountMigration = Get-MigrationState `
    -Network $AccountInstance `
    -PasswordSecret "$AccountInstance-postgres-password" `
    -User "paigram" `
    -Database "paigram"
$platformMigration = Get-MigrationState `
    -Network "$PlatformInstance-private" `
    -PasswordSecret "$PlatformInstance-postgres-password" `
    -User "platform_mihomo" `
    -Database "platform_mihomo"
Assert-RecoveryEqual -Actual $accountMigration -Expected $manifest.migrations.account -Message "Account migration state"
Assert-RecoveryEqual -Actual $platformMigration -Expected $manifest.migrations.platform -Message "Platform migration state"

$httpPort = Get-RecoveryLoopbackPort -Container "$AccountInstance-frontend" -ContainerPort "8080/tcp"
$accountGRPCPort = Get-RecoveryLoopbackPort -Container $AccountInstance -ContainerPort "50051/tcp"
$platformRuntimePort = Get-RecoveryLoopbackPort -Container $PlatformInstance -ContainerPort "9001/tcp"
$runtimeRoute = Get-RuntimeRoute `
    -Network $AccountInstance `
    -PasswordSecret "$AccountInstance-postgres-password"
if ($runtimeRoute.endpoint -notin @("127.0.0.1:$platformRuntimePort", "localhost:$platformRuntimePort", "[::1]:$platformRuntimePort")) {
    throw "Restored runtime route does not target the recovered Platform loopback publication"
}
Assert-RecoveryAccountHealth -FrontendContainer "$AccountInstance-frontend" -Path "/livez"
Assert-RecoveryAccountHealth -FrontendContainer "$AccountInstance-frontend" -Path "/readyz"
Assert-RecoveryPlatformHealth -Container $PlatformInstance -PlatformInstance $PlatformInstance -ServerName $runtimeRoute.server_name -Service ""
Assert-RecoveryPlatformHealth -Container $PlatformInstance -PlatformInstance $PlatformInstance -ServerName $runtimeRoute.server_name -Service "liveness"

Assert-MountedSecret -Container $AccountInstance -MountedPath "/run/secrets/$AccountInstance-encryption-key" -RecoveredPath (Join-Path $recoveredRoot $requiredRecoveredFiles.AccountEncryption)
Assert-MountedSecret -Container $AccountInstance -MountedPath "/run/secrets/$AccountInstance-service-ticket-signing-key" -RecoveredPath (Join-Path $recoveredRoot $requiredRecoveredFiles.TicketSigning)
Assert-MountedSecret -Container $PlatformInstance -MountedPath "/run/secrets/$PlatformInstance-encryption-keyring" -RecoveredPath (Join-Path $recoveredRoot $requiredRecoveredFiles.PlatformEncryption)
Assert-MountedSecret -Container $PlatformInstance -MountedPath "/run/secrets/paigram-account-center-service-ticket-public-keyring" -RecoveredPath (Join-Path $recoveredRoot $requiredRecoveredFiles.TicketPublic)

$originalRecoveryHTTPURL = $env:PAI_RECOVERY_ACCOUNT_HTTP_URL
$originalRecoveryGRPCTarget = $env:PAI_RECOVERY_ACCOUNT_GRPC_TARGET
$originalPythonPath = $env:PYTHONPATH
$originalPythonSafePath = $env:PYTHONSAFEPATH
$originalPythonNoUserSite = $env:PYTHONNOUSERSITE
$tracerExitCode = -1
try {
    $env:PAI_RECOVERY_ACCOUNT_HTTP_URL = "http://127.0.0.1:$httpPort"
    $env:PAI_RECOVERY_ACCOUNT_GRPC_TARGET = "127.0.0.1:$accountGRPCPort"
    Remove-Item Env:PYTHONPATH -ErrorAction SilentlyContinue
    $env:PYTHONSAFEPATH = "1"
    $env:PYTHONNOUSERSITE = "1"
    $global:LASTEXITCODE = 0
    & uv run --project $sdkDirectory --isolated --locked python -m paigram_account_sdk.recovery_tracer $tracerConfig
    $tracerExitCode = $LASTEXITCODE
} finally {
    if ($null -eq $originalRecoveryHTTPURL) {
        Remove-Item Env:PAI_RECOVERY_ACCOUNT_HTTP_URL -ErrorAction SilentlyContinue
    } else {
        $env:PAI_RECOVERY_ACCOUNT_HTTP_URL = $originalRecoveryHTTPURL
    }
    if ($null -eq $originalRecoveryGRPCTarget) {
        Remove-Item Env:PAI_RECOVERY_ACCOUNT_GRPC_TARGET -ErrorAction SilentlyContinue
    } else {
        $env:PAI_RECOVERY_ACCOUNT_GRPC_TARGET = $originalRecoveryGRPCTarget
    }
    if ($null -eq $originalPythonPath) {
        Remove-Item Env:PYTHONPATH -ErrorAction SilentlyContinue
    } else {
        $env:PYTHONPATH = $originalPythonPath
    }
    if ($null -eq $originalPythonSafePath) {
        Remove-Item Env:PYTHONSAFEPATH -ErrorAction SilentlyContinue
    } else {
        $env:PYTHONSAFEPATH = $originalPythonSafePath
    }
    if ($null -eq $originalPythonNoUserSite) {
        Remove-Item Env:PYTHONNOUSERSITE -ErrorAction SilentlyContinue
    } else {
        $env:PYTHONNOUSERSITE = $originalPythonNoUserSite
    }
}
if ($tracerExitCode -ne 0) {
    throw "Recovered release tracer failed"
}

$evidence = [ordered]@{
    format_version = 1
    verified_at = [DateTime]::UtcNow.ToString("O")
    recovery_manifest_sha256 = (Get-FileHash -LiteralPath $manifestPath -Algorithm SHA256).Hash.ToLowerInvariant()
    account_image = [string]$manifest.images.account.reference
    frontend_image = [string]$manifest.images.frontend.reference
    platform_image = [string]$manifest.images.platform.reference
    account_migration = $accountMigration
    platform_migration = $platformMigration
    account_http_port = $httpPort
    account_grpc_port = $accountGRPCPort
    platform_runtime_port = $platformRuntimePort
    tracer = "passed"
}
$evidenceJSON = $evidence | ConvertTo-Json -Depth 4
try {
    $stream = [System.IO.File]::Open(
        $evidencePath,
        [System.IO.FileMode]::CreateNew,
        [System.IO.FileAccess]::Write,
        [System.IO.FileShare]::None
    )
    $writer = $null
    try {
        $writer = [System.IO.StreamWriter]::new($stream, [System.Text.UTF8Encoding]::new($false))
        $writer.WriteLine($evidenceJSON)
        $writer.Flush()
    } finally {
        if ($null -ne $writer) {
            $writer.Dispose()
        } else {
            $stream.Dispose()
        }
    }
    if (-not $IsWindows) {
        [System.IO.File]::SetUnixFileMode(
            $evidencePath,
            [System.IO.UnixFileMode]::UserRead -bor [System.IO.UnixFileMode]::UserWrite
        )
    }
} catch {
    Remove-Item -LiteralPath $evidencePath -Force -ErrorAction SilentlyContinue
    throw
}
Write-Host "Recovered release verification passed. Evidence written to $evidencePath"
