[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [ValidatePattern('^https://')]
    [string]$AccountCenterUrl,
    [Parameter(Mandatory)]
	[string]$AdminAccessTokenFile,
	[Parameter(Mandatory)]
	[ValidatePattern('^[^\s/:?#]+:\d{1,5}$')]
	[string]$RuntimeEndpoint,
	[Parameter(Mandatory)]
	[ValidatePattern('^[^\s/:?#]+$')]
	[string]$RuntimeServerName
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$baseUrl = $AccountCenterUrl.TrimEnd('/')
$accessToken = (Get-Content -LiteralPath $AdminAccessTokenFile -Raw).Trim()
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
