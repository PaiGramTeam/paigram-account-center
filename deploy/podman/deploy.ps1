[CmdletBinding()]
param(
    [switch]$NoBuild
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

Set-Location -LiteralPath $PSScriptRoot
if (-not (Test-Path -LiteralPath ".env")) {
    throw "Missing deploy/podman/.env; run ./init-env.ps1 first"
}
if (-not (Get-Command podman -ErrorAction SilentlyContinue)) {
    throw "podman is required"
}

function Get-EnvValue {
    param([Parameter(Mandatory)][string]$Name)

    $prefix = "$Name="
    $line = Get-Content -LiteralPath ".env" | Where-Object { $_.StartsWith($prefix) } | Select-Object -Last 1
    if (-not $line) {
        return ""
    }
    return $line.Substring($prefix.Length)
}

$instance = Get-EnvValue -Name "PAI_INSTANCE"
if (-not $instance) {
    $instance = "paigram-account-center"
}
if ($instance -notmatch '^[a-z0-9][a-z0-9-]*$') {
    throw "PAI_INSTANCE must contain only lowercase letters, digits, and hyphens"
}

$composeArguments = @("compose", "--env-file", ".env", "-p", $instance, "-f", "compose.yaml")
$upArguments = if ($NoBuild) { @("--no-build", "-d") } else { @("--build", "-d") }
& podman @composeArguments up @upArguments
if ($LASTEXITCODE -ne 0) {
    throw "Podman Compose deployment failed"
}

foreach ($container in @("$instance-frontend", "$instance-platform-mihomo")) {
    for ($attempt = 1; $attempt -le 60; $attempt++) {
        $status = & podman inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' $container 2>$null
        if ($status -eq "healthy") {
            break
        }
        if ($status -in @("exited", "unhealthy")) {
            & podman logs --tail 80 $container
            throw "$container entered state $status"
        }
        if ($attempt -eq 60) {
            & podman logs --tail 80 $container
            throw "Timed out waiting for $container"
        }
        Start-Sleep -Seconds 2
    }
}

foreach ($privateContainer in @($instance, "$instance-postgres", "$instance-redis", "$instance-platform-postgres", "$instance-platform-redis")) {
    $publishedPorts = & podman port $privateContainer
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to inspect $privateContainer ports"
    }
    if ($publishedPorts) {
        throw "$privateContainer unexpectedly publishes a host port: $publishedPorts"
    }
}

$platformPorts = & podman port "$instance-platform-mihomo"
if ($LASTEXITCODE -ne 0 -or -not $platformPorts) {
    throw "Platform Mihomo does not publish its configured loopback gRPC port"
}

$httpPort = Get-EnvValue -Name "PAI_HTTP_PORT"
if (-not $httpPort) {
    $httpPort = "18080"
}
$httpBind = Get-EnvValue -Name "PAI_HTTP_BIND"
if (-not $httpBind -or $httpBind -eq "0.0.0.0") {
    $httpBind = "127.0.0.1"
}
$response = Invoke-RestMethod -Uri "http://${httpBind}:$httpPort/healthz" -TimeoutSec 10
if ($response.code -ne 200 -or $response.data.status -ne "ok") {
    throw "Account Center health check returned an unexpected response"
}

foreach ($path in @("/", "/admin/", "/api-docs")) {
    $page = Invoke-WebRequest -Uri "http://${httpBind}:$httpPort$path" -TimeoutSec 10
    if ($page.StatusCode -ne 200 -or $page.Headers["Content-Type"] -notlike "text/html*") {
        throw "Frontend check for $path returned an unexpected response"
    }
}

Write-Host "PaiGram Account Center is ready at http://${httpBind}:$httpPort (admin: /admin/)"
