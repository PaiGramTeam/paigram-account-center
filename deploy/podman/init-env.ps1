[CmdletBinding()]
param(
    [ValidateRange(1, 65535)]
    [int]$HttpPort = 18080,
    [Parameter(Mandatory)]
    [string]$FrontendBaseUrl,
    [Parameter(Mandatory)]
    [ValidatePattern('^\S+@sha256:[a-f0-9]{64}$')]
    [string]$AccountImage,
    [Parameter(Mandatory)]
    [ValidatePattern('^\S+@sha256:[a-f0-9]{64}$')]
    [string]$FrontendImage
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$target = Join-Path $PSScriptRoot ".env"
if (Test-Path -LiteralPath $target) {
    throw "$target already exists"
}
$publicFrontendUri = $null
if (-not [Uri]::TryCreate($FrontendBaseUrl, [UriKind]::Absolute, [ref]$publicFrontendUri) -or
    $publicFrontendUri.Scheme -ne "https" -or
    -not $publicFrontendUri.Host -or
    $publicFrontendUri.UserInfo -or
    $publicFrontendUri.Query -or
    $publicFrontendUri.Fragment -or
    $publicFrontendUri.AbsolutePath -ne "/") {
    throw "FrontendBaseUrl must be a public HTTPS origin without credentials, path, query, or fragment"
}

$values = @(
    "PAI_INSTANCE=paigram-account-center"
    "PAI_PLATFORM_NETWORK=paigram-platform-backplane"
    "PAI_ACCOUNT_IMAGE=$AccountImage"
    "PAI_FRONTEND_IMAGE=$FrontendImage"
    "PAI_HTTP_BIND=127.0.0.1"
    "PAI_HTTP_PORT=$HttpPort"
    "PAI_ACCOUNT_GRPC_BIND=127.0.0.1"
    "PAI_ACCOUNT_GRPC_PORT=15051"
    "PAI_ACCOUNT_GRPC_TLS=false"
    "PAI_PLATFORM_CONTROL_TLS=false"
    "PAI_PLATFORM_CONTROL_SERVER_NAME=platform-control.internal"
    "PAI_FRONTEND_BASE_URL=$($publicFrontendUri.GetLeftPart([UriPartial]::Authority))"
    "PAI_REQUIRE_EMAIL_VERIFICATION=false"
    "PAI_ADMIN_EMAIL=admin@paigram.local"
    "PAI_ADMIN_NAME=Administrator"
)

[System.IO.File]::WriteAllLines($target, $values, [System.Text.UTF8Encoding]::new($false))
Write-Host "Created non-secret settings at $target. Provision the external Podman secrets listed in compose.yaml before deployment."
