[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot
Import-Module (Join-Path $PSScriptRoot "../PodmanCompose.psm1") -Force
Import-Module (Join-Path $PSScriptRoot "../DeploymentComposition.psm1") -Force

if (-not (Test-Path -LiteralPath ".env")) {
    throw "Missing deploy/podman-platform-mihomo/.env"
}
if (-not (Get-Command podman -ErrorAction SilentlyContinue)) {
    throw "podman is required"
}
$podmanCompose = Assert-PodmanComposeAvailable

function Get-EnvValue {
    param([Parameter(Mandatory)][string]$Name)
    $prefix = "$Name="
    $line = Get-Content -LiteralPath ".env" | Where-Object { $_.StartsWith($prefix) } | Select-Object -Last 1
    if (-not $line) { return "" }
    return $line.Substring($prefix.Length)
}

$composeEnvironmentKeys = @(
    "PAI_PLATFORM_INSTANCE", "PAI_PLATFORM_NETWORK", "PAI_PLATFORM_IMAGE", "PAI_RUNTIME_BIND",
    "PAI_RUNTIME_PORT", "PAI_RUNTIME_SERVER_NAME", "PAI_MIHOMO_UPSTREAM_BASE_URL"
)
foreach ($name in $composeEnvironmentKeys) {
    [Environment]::SetEnvironmentVariable($name, (Get-EnvValue -Name $name), "Process")
}

$instance = Get-EnvValue -Name "PAI_PLATFORM_INSTANCE"
if (-not $instance) { $instance = "paigram-platform-mihomo" }
if ($instance -notmatch '^[a-z0-9][a-z0-9-]*$') {
    throw "PAI_PLATFORM_INSTANCE must contain only lowercase letters, digits, and hyphens"
}
$platformImage = Get-EnvValue -Name "PAI_PLATFORM_IMAGE"
if ($platformImage -notmatch '^\S+@sha256:[a-f0-9]{64}$') {
    throw "PAI_PLATFORM_IMAGE must be an immutable image reference containing a 64-character sha256 digest"
}
& podman pull $platformImage
if ($LASTEXITCODE -ne 0) {
    throw "Could not pull PAI_PLATFORM_IMAGE"
}
$network = Get-EnvValue -Name "PAI_PLATFORM_NETWORK"
if (-not $network) { $network = "paigram-platform-backplane" }
& podman network inspect $network *> $null
if ($LASTEXITCODE -ne 0) {
    & podman network create $network
    if ($LASTEXITCODE -ne 0) { throw "Could not create shared Platform network $network" }
}

$composeArguments = @("--env-file", ".env", "-p", $instance, "-f", "compose.yaml")
Invoke-ImmutableComposeDeployment `
    -PodmanCompose $podmanCompose `
    -ComposeArguments $composeArguments `
    -FailureMessage "Platform Mihomo deployment failed"

for ($attempt = 1; $attempt -le 60; $attempt++) {
    $status = & podman inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' $instance 2>$null
    if ($status -eq "healthy") { break }
    if ($status -in @("exited", "unhealthy") -or $attempt -eq 60) {
        & podman logs --tail 80 $instance
        throw "$instance did not become healthy"
    }
    Start-Sleep -Seconds 2
}

foreach ($privateContainer in @("$instance-postgres", "$instance-redis")) {
    $publishedPorts = & podman port $privateContainer
    if ($LASTEXITCODE -ne 0 -or $publishedPorts) {
        throw "$privateContainer unexpectedly publishes a host port"
    }
}
Write-Host "Platform Mihomo is ready; apply registry-descriptor.json through the authenticated Account Center admin API."
