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

$against = "$repositoryRoot#format=git,commit=$baselineCommit,subdir=contracts"
Push-Location $PSScriptRoot
try {
    buf breaking . --against $against --config buf.breaking.yaml
    if ($LASTEXITCODE -ne 0) {
        throw "Contracts contain a wire-breaking change from the v2 baseline (exit code $LASTEXITCODE)"
    }
}
finally {
    Pop-Location
}
