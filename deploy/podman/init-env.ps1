[CmdletBinding()]
param(
    [ValidateRange(1, 65535)]
    [int]$HttpPort = 18080,
    [Parameter(Mandatory)]
    [string]$FrontendBaseUrl
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$deployDirectory = $PSScriptRoot
$target = Join-Path $deployDirectory ".env"
if (Test-Path -LiteralPath $target) {
    throw "$target already exists"
}
$publicFrontendUri = $null
if (-not [Uri]::TryCreate($FrontendBaseUrl, [UriKind]::Absolute, [ref]$publicFrontendUri) -or
    $publicFrontendUri.Scheme -ne "https" -or
    -not $publicFrontendUri.Host -or
    $publicFrontendUri.UserInfo -or
    $publicFrontendUri.Query -or
    $publicFrontendUri.Fragment) {
    throw "FrontendBaseUrl must be a public HTTPS origin without credentials, path parameters, query, or fragment"
}
if ($publicFrontendUri.AbsolutePath -ne "/") {
    throw "FrontendBaseUrl must be an origin without a path"
}
$FrontendBaseUrl = $publicFrontendUri.GetLeftPart([UriPartial]::Authority)

function New-HexSecret {
    param([Parameter(Mandatory)][int]$ByteCount)

    $bytes = [System.Security.Cryptography.RandomNumberGenerator]::GetBytes($ByteCount)
    return [Convert]::ToHexString($bytes).ToLowerInvariant()
}

$repositoryRoot = (Resolve-Path (Join-Path $deployDirectory "..\..")).Path
Push-Location $repositoryRoot
try {
    $keyPairOutput = & go run ./contracts/runtime/go/cmd/service-ticket-keygen
    if ($LASTEXITCODE -ne 0) {
        throw "Could not generate the service-ticket Ed25519 key pair"
    }
}
finally {
    Pop-Location
}
$keyPair = $keyPairOutput | ConvertFrom-Json
$privateKeyPEM = $keyPair.private_key_pem.Replace("`r", "").Replace("`n", "\n")
$publicKeyPEM = $keyPair.public_key_pem.Replace("`r", "").Replace("`n", "\n")

$values = @(
    "PAI_INSTANCE=paigram-account-center"
    "PAI_NETWORK_SUBNET=10.90.0.0/24"
    "PAI_IMAGE_TAG=latest"
    "PAI_HTTP_BIND=127.0.0.1"
    "PAI_HTTP_PORT=$HttpPort"
    "PAI_FRONTEND_BASE_URL=$FrontendBaseUrl"
    "PAI_REQUIRE_EMAIL_VERIFICATION=false"
    "PAI_POSTGRES_PASSWORD=$(New-HexSecret -ByteCount 24)"
    "PAI_REDIS_PASSWORD=$(New-HexSecret -ByteCount 24)"
    "PAI_PLATFORM_POSTGRES_PASSWORD=$(New-HexSecret -ByteCount 24)"
    "PAI_PLATFORM_REDIS_PASSWORD=$(New-HexSecret -ByteCount 24)"
    "PAI_SERVICE_TICKET_PRIVATE_KEY_PEM=$privateKeyPEM"
    "PAI_SERVICE_TICKET_PUBLIC_KEY_PEM=$publicKeyPEM"
    "PAI_OAUTH_SIGNING_KEY=$(New-HexSecret -ByteCount 32)"
    "PAI_SECURITY_ENCRYPTION_KEY=$(New-HexSecret -ByteCount 16)"
    "PAI_PLATFORM_CREDENTIAL_ENCRYPTION_KEY=$(New-HexSecret -ByteCount 16)"
    "PAI_PLATFORM_GRPC_BIND=127.0.0.1"
    "PAI_PLATFORM_GRPC_PORT=19000"
    "PAI_MIHOMO_UPSTREAM_BASE_URL=https://mihomo-upstream.internal"
    "PAI_MIHOMO_UPSTREAM_BEARER_TOKEN_FILE="
    "PAI_ADMIN_EMAIL=admin@paigram.local"
    "PAI_ADMIN_PASSWORD=Admin-$(New-HexSecret -ByteCount 16)"
    "PAI_ADMIN_NAME=Administrator"
)

[System.IO.File]::WriteAllLines($target, $values, [System.Text.UTF8Encoding]::new($false))
Write-Host "Created $target. Review the generated settings before deployment."
