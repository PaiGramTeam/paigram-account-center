param(
    [switch]$AllowDirty,
    [switch]$SkipPaiGramCompatibility
)

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

Write-Host "[1/6] Regenerating shared contracts"
Invoke-Checked -Command { & "$repositoryRoot\contracts\generate.ps1" } -FailureMessage "Contract generation failed"
Invoke-InDirectory -Path $repositoryRoot -Command {
    git diff --exit-code -- contracts/gen/go sdks/python/src/paigram_account_sdk/_generated
} -FailureMessage "Generated contracts are not reproducible"

Write-Host "[2/6] Verifying Go modules"
Invoke-InDirectory -Path "$repositoryRoot\contracts\gen\go" -Command { go test ./... } -FailureMessage "Go contract tests failed"
Invoke-InDirectory -Path "$repositoryRoot\services\account-center" -Command { go test ./... } -FailureMessage "Account Center tests failed"
Invoke-InDirectory -Path "$repositoryRoot\services\account-center" -Command { go build ./... } -FailureMessage "Account Center build failed"
Invoke-InDirectory -Path "$repositoryRoot\services\platform-mihomo" -Command { go test ./... } -FailureMessage "Mihomo service tests failed"
Invoke-InDirectory -Path "$repositoryRoot\services\platform-mihomo" -Command { go build ./... } -FailureMessage "Mihomo service build failed"

Write-Host "[3/6] Verifying frontend workspace"
Invoke-InDirectory -Path "$repositoryRoot\frontend" -Command { bun install --frozen-lockfile } -FailureMessage "Frontend install failed"
Invoke-InDirectory -Path "$repositoryRoot\frontend" -Command { bun run format:check } -FailureMessage "Frontend format check failed"
Invoke-InDirectory -Path "$repositoryRoot\frontend" -Command { bun run lint } -FailureMessage "Frontend lint failed"
Invoke-InDirectory -Path "$repositoryRoot\frontend" -Command { bun run type-check } -FailureMessage "Frontend type check failed"
Invoke-InDirectory -Path "$repositoryRoot\frontend" -Command { bun run test } -FailureMessage "Frontend tests failed"
Invoke-InDirectory -Path "$repositoryRoot\frontend" -Command { bun run build:all } -FailureMessage "Frontend build failed"

Write-Host "[4/6] Verifying Python SDK"
Invoke-InDirectory -Path "$repositoryRoot\sdks\python" -Command { uv sync --all-groups --frozen } -FailureMessage "SDK install failed"
Invoke-InDirectory -Path "$repositoryRoot\sdks\python" -Command { uv run ruff check . } -FailureMessage "SDK lint failed"
Invoke-InDirectory -Path "$repositoryRoot\sdks\python" -Command { uv run ruff format --check . } -FailureMessage "SDK format check failed"
Invoke-InDirectory -Path "$repositoryRoot\sdks\python" -Command { uv run mypy } -FailureMessage "SDK type check failed"
Invoke-InDirectory -Path "$repositoryRoot\sdks\python" -Command { uv run pytest -q } -FailureMessage "SDK tests failed"
Invoke-InDirectory -Path "$repositoryRoot\sdks\python" -Command { uv build } -FailureMessage "SDK package build failed"

Write-Host "[5/6] Verifying PaiGram SDK compatibility"
if ($SkipPaiGramCompatibility) {
    Write-Host "PaiGram compatibility verification skipped by request."
}
else {
    Invoke-Checked -Command {
        & "$repositoryRoot\scripts\verify-paigram-sdk.ps1"
    } -FailureMessage "PaiGram SDK compatibility verification failed"
}

Write-Host "[6/6] Checking repository hygiene"
Invoke-InDirectory -Path $repositoryRoot -Command { git diff --check } -FailureMessage "Repository whitespace check failed"
if (-not $AllowDirty) {
    $repositoryChanges = @(git status --porcelain --untracked-files=all)
    if ($LASTEXITCODE -ne 0) {
        throw "Could not inspect repository status (exit code $LASTEXITCODE)"
    }
    if ($repositoryChanges.Count -ne 0) {
        $repositoryChanges | ForEach-Object { Write-Host $_ }
        throw "Repository must be clean after local verification"
    }
}

Write-Host "Local verification completed successfully."
