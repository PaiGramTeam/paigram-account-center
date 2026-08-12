$ErrorActionPreference = "Stop"

$repositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$baselineCommit = git -C $repositoryRoot log --diff-filter=A --format=%H -- contracts/proto/platform/v2/types.proto |
    Select-Object -Last 1
if ($LASTEXITCODE -ne 0) {
    throw "Could not resolve the v2 contract baseline"
}

if ([string]::IsNullOrWhiteSpace($baselineCommit)) {
    Write-Warning "The v2 contract is being bootstrapped; breaking checks begin after its first commit."
    exit 0
}

$temporaryRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$checkRoot = Join-Path $temporaryRoot "paigram-contract-breaking-$([guid]::NewGuid().ToString('N'))"
$checkPrefix = Join-Path $temporaryRoot "paigram-contract-breaking-"
if (-not $checkRoot.StartsWith($checkPrefix, [StringComparison]::OrdinalIgnoreCase)) {
    throw "Breaking-check directory escaped the validated temporary prefix"
}

New-Item -ItemType Directory -Path $checkRoot | Out-Null
$locationPushed = $false
try {
    $archive = Join-Path $checkRoot "contracts.zip"
    git -C $repositoryRoot archive --format=zip --output=$archive $baselineCommit contracts
    if ($LASTEXITCODE -ne 0) {
        throw "Could not export the v2 contract baseline"
    }
    Expand-Archive -LiteralPath $archive -DestinationPath $checkRoot
    $against = Join-Path $checkRoot "contracts"

    Push-Location $PSScriptRoot
	$locationPushed = $true
    buf breaking . --against $against --config buf.breaking.yaml --against-config (Join-Path $against "buf.yaml")
    if ($LASTEXITCODE -ne 0) {
        throw "Contracts contain a wire-breaking change from the v2 baseline (exit code $LASTEXITCODE)"
    }
}
finally {
	if ($locationPushed) {
        Pop-Location
    }
    $resolvedCheckRoot = [IO.Path]::GetFullPath($checkRoot)
    if (-not $resolvedCheckRoot.StartsWith($checkPrefix, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to remove an unvalidated breaking-check directory"
    }
    Remove-Item -LiteralPath $resolvedCheckRoot -Recurse -Force
}
