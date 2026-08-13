[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [string]$AccountCenterUrl,
    [Parameter(Mandatory)]
    [string]$AdminAccessTokenFile,
    [Parameter(Mandatory)]
    [ValidatePattern('^[^\s/:?#]+:\d{1,5}$')]
    [string]$RuntimeEndpoint,
    [Parameter(Mandatory)]
    [ValidatePattern('^[^\s/:?#]+$')]
    [string]$RuntimeServerName,
    [switch]$AllowLoopbackHTTP
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
Import-Module (Join-Path $PSScriptRoot "../OperationalSecurity.psm1") -Force

$repositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot "../..")).Path
$tokenPath = Assert-OperationalPrivateFile -Path $AdminAccessTokenFile -RepositoryRoot $repositoryRoot
$operationError = $null
try {
    $accountCenterURI = Resolve-OperationalServiceURL -URL $AccountCenterUrl -AllowLoopbackHTTP:$AllowLoopbackHTTP
    $baseUrl = $accountCenterURI.AbsoluteUri.TrimEnd('/')
    $accessToken = (Get-Content -LiteralPath $tokenPath -Raw).Trim()
    if (-not $accessToken) { throw "Admin access-token file is empty" }
    $headers = @{ Authorization = "Bearer $accessToken" }
    $descriptor = Get-Content -LiteralPath (Join-Path $PSScriptRoot "registry-descriptor.json") -Raw | ConvertFrom-Json -AsHashtable
    $descriptor.runtime_endpoint = $RuntimeEndpoint
    $descriptor.runtime_server_name = $RuntimeServerName
    $catalog = Invoke-RestMethod -Uri "$baseUrl/api/v1/admin/system/platform-services" -Headers $headers -Method Get -TimeoutSec 15
    $existing = @($catalog.data) | Where-Object { $_.platform_key -eq $descriptor.platform_key } | Select-Object -First 1
    $body = $descriptor | ConvertTo-Json -Depth 10 -Compress
    if ($existing) {
        Invoke-RestMethod -Uri "$baseUrl/api/v1/admin/system/platform-services/$($existing.id)" -Headers $headers -Method Patch -ContentType "application/json" -Body $body -TimeoutSec 15 | Out-Null
        Write-Host "Updated Platform registry descriptor for $($descriptor.platform_key)."
    } else {
        Invoke-RestMethod -Uri "$baseUrl/api/v1/admin/system/platform-services" -Headers $headers -Method Post -ContentType "application/json" -Body $body -TimeoutSec 15 | Out-Null
        Write-Host "Created Platform registry descriptor for $($descriptor.platform_key)."
    }
} catch {
    $operationError = $_.Exception
}
$cleanupError = $null
try {
    Remove-Item -LiteralPath $tokenPath -Force
} catch {
    $cleanupError = $_.Exception
}
if ($null -ne $operationError -and $null -ne $cleanupError) {
    throw [System.AggregateException]::new(
        "Registry update and access-token cleanup both failed",
        [Exception[]]@($operationError, $cleanupError)
    )
}
if ($null -ne $operationError) {
    throw $operationError
}
if ($null -ne $cleanupError) {
    throw $cleanupError
}
