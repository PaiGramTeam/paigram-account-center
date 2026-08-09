$ErrorActionPreference = "Stop"

$repositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path

function Invoke-Checked {
    param(
        [Parameter(Mandatory = $true)]
        [scriptblock]$Command,
        [Parameter(Mandatory = $true)]
        [string]$FailureMessage
    )

    & $Command
    if ($LASTEXITCODE -ne 0) {
        throw "$FailureMessage (exit code $LASTEXITCODE)"
    }
}

function Invoke-InDirectory {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [scriptblock]$Command,
        [Parameter(Mandatory = $true)]
        [string]$FailureMessage
    )

    Push-Location $Path
    try {
        Invoke-Checked -Command $Command -FailureMessage $FailureMessage
    }
    finally {
        Pop-Location
    }
}

Write-Host "[1/5] Regenerating shared contracts"
Invoke-Checked -Command { & "$repositoryRoot\contracts\generate.ps1" } -FailureMessage "Contract generation failed"
Invoke-InDirectory -Path $repositoryRoot -Command {
    git diff --exit-code -- contracts/gen/go sdks/python/src/paigram_account_sdk/_generated
} -FailureMessage "Generated contracts are not reproducible"

Write-Host "[2/5] Verifying Go modules"
Invoke-InDirectory -Path "$repositoryRoot\contracts\gen\go" -Command { go test ./... } -FailureMessage "Go contract tests failed"
Invoke-InDirectory -Path "$repositoryRoot\services\account-center" -Command { go test ./... } -FailureMessage "Account Center tests failed"
Invoke-InDirectory -Path "$repositoryRoot\services\account-center" -Command { go build ./... } -FailureMessage "Account Center build failed"
Invoke-InDirectory -Path "$repositoryRoot\services\platform-mihomo" -Command { go test ./... } -FailureMessage "Mihomo service tests failed"
Invoke-InDirectory -Path "$repositoryRoot\services\platform-mihomo" -Command { go build ./... } -FailureMessage "Mihomo service build failed"

Write-Host "[3/5] Verifying frontend workspace"
Invoke-InDirectory -Path "$repositoryRoot\frontend" -Command { bun install --frozen-lockfile } -FailureMessage "Frontend install failed"
Invoke-InDirectory -Path "$repositoryRoot\frontend" -Command { bun run format:check } -FailureMessage "Frontend format check failed"
Invoke-InDirectory -Path "$repositoryRoot\frontend" -Command { bun run lint } -FailureMessage "Frontend lint failed"
Invoke-InDirectory -Path "$repositoryRoot\frontend" -Command { bun run type-check } -FailureMessage "Frontend type check failed"
Invoke-InDirectory -Path "$repositoryRoot\frontend" -Command { bun run test } -FailureMessage "Frontend tests failed"
Invoke-InDirectory -Path "$repositoryRoot\frontend" -Command { bun run build:all } -FailureMessage "Frontend build failed"

Write-Host "[4/5] Verifying Python SDK"
Invoke-InDirectory -Path "$repositoryRoot\sdks\python" -Command { uv sync --all-groups --frozen } -FailureMessage "SDK install failed"
Invoke-InDirectory -Path "$repositoryRoot\sdks\python" -Command { uv run ruff check . } -FailureMessage "SDK lint failed"
Invoke-InDirectory -Path "$repositoryRoot\sdks\python" -Command { uv run ruff format --check . } -FailureMessage "SDK format check failed"
Invoke-InDirectory -Path "$repositoryRoot\sdks\python" -Command { uv run mypy } -FailureMessage "SDK type check failed"
Invoke-InDirectory -Path "$repositoryRoot\sdks\python" -Command { uv run pytest -q } -FailureMessage "SDK tests failed"
Invoke-InDirectory -Path "$repositoryRoot\sdks\python" -Command { uv build } -FailureMessage "SDK package build failed"

Write-Host "[5/5] Checking repository hygiene"
Invoke-InDirectory -Path $repositoryRoot -Command { git diff --check } -FailureMessage "Repository whitespace check failed"

Write-Host "Local verification completed successfully."

