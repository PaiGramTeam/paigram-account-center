$ErrorActionPreference = "Stop"

$exitCode = 0
Push-Location $PSScriptRoot
try {
    uv run python tools/generate_contracts.py
    $exitCode = $LASTEXITCODE
}
finally {
    Pop-Location
}
if ($exitCode -ne 0) {
    exit $exitCode
}
