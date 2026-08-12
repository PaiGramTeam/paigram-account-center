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

function Get-FileManifest {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$Paths
    )

    $files = foreach ($path in $Paths) {
        $item = Get-Item -LiteralPath $path
        if ($item.PSIsContainer) {
            Get-ChildItem -LiteralPath $item.FullName -Recurse -File
        }
        else {
            $item
        }
    }

    @($files |
        Where-Object { $_.Extension -ne ".pyc" -and $_.FullName -notmatch "[\\/]__pycache__[\\/]" } |
        Sort-Object FullName |
        ForEach-Object {
        $relativePath = [IO.Path]::GetRelativePath($repositoryRoot, $_.FullName)
        "$relativePath`t$((Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash)"
    })
}

function Assert-ManifestUnchanged {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$Before,
        [Parameter(Mandatory = $true)]
        [string[]]$After,
        [Parameter(Mandatory = $true)]
        [string]$FailureMessage
    )

    $changes = @(Compare-Object -ReferenceObject $Before -DifferenceObject $After)
    if ($changes.Count -ne 0) {
        $changes | ForEach-Object { Write-Host $_ }
        throw $FailureMessage
    }
}

Write-Host "[1/6] Regenerating shared contracts"
$contractManifest = Get-FileManifest -Paths @(
    "$repositoryRoot\contracts\gen\go",
    "$repositoryRoot\sdks\python\src\paigram_account_sdk\_generated"
)
Invoke-Checked -Command { & "$repositoryRoot\contracts\generate.ps1" } -FailureMessage "Contract generation failed"
$regeneratedContractManifest = Get-FileManifest -Paths @(
    "$repositoryRoot\contracts\gen\go",
    "$repositoryRoot\sdks\python\src\paigram_account_sdk\_generated"
)
Assert-ManifestUnchanged -Before $contractManifest -After $regeneratedContractManifest -FailureMessage "Generated contracts are not reproducible"
$openAPIManifest = Get-FileManifest -Paths @(
    "$repositoryRoot\contracts\openapi.json",
    "$repositoryRoot\frontend\packages\shared-components\src\api\generated\schema.ts"
)
Invoke-InDirectory -Path "$repositoryRoot\services\account-center" -Command {
    go run ./cmd/paigram openapi --out ../../contracts/openapi.json
} -FailureMessage "OpenAPI generation failed"
Invoke-InDirectory -Path "$repositoryRoot\frontend" -Command { bun install --frozen-lockfile } -FailureMessage "Frontend install failed"
Invoke-InDirectory -Path "$repositoryRoot\frontend" -Command { bun run openapi:gen } -FailureMessage "Frontend OpenAPI type generation failed"
$regeneratedOpenAPIManifest = Get-FileManifest -Paths @(
    "$repositoryRoot\contracts\openapi.json",
    "$repositoryRoot\frontend\packages\shared-components\src\api\generated\schema.ts"
)
Assert-ManifestUnchanged -Before $openAPIManifest -After $regeneratedOpenAPIManifest -FailureMessage "OpenAPI artifacts are not reproducible"

Write-Host "[2/6] Verifying Go modules"
if ([string]::IsNullOrWhiteSpace($env:PAI_TEST_DATABASE_DSN) -and [string]::IsNullOrWhiteSpace($env:PAI_DATABASE_DSN)) {
    throw "Database-backed tests require PAI_TEST_DATABASE_DSN or PAI_DATABASE_DSN; verification will not silently skip them"
}
$env:PAI_REQUIRE_DATABASE_TESTS = "true"
Invoke-InDirectory -Path "$repositoryRoot\contracts\runtime\go" -Command { go test ./... } -FailureMessage "Service ticket runtime tests failed"
Invoke-InDirectory -Path "$repositoryRoot\contracts\gen\go" -Command { go test ./... } -FailureMessage "Go contract tests failed"
Invoke-InDirectory -Path "$repositoryRoot\services\account-center" -Command { go test ./... } -FailureMessage "Account Center tests failed"
Invoke-InDirectory -Path "$repositoryRoot\services\account-center" -Command {
    go test -count=1 -tags=integration ./integration
} -FailureMessage "Account Center integration tests failed"
Invoke-InDirectory -Path "$repositoryRoot\services\account-center" -Command { go build ./... } -FailureMessage "Account Center build failed"
Invoke-InDirectory -Path "$repositoryRoot\services\platform-mihomo" -Command { go test ./... } -FailureMessage "Mihomo service tests failed"
Invoke-InDirectory -Path "$repositoryRoot\services\platform-mihomo" -Command {
    go test -count=1 -tags=integration ./integration
} -FailureMessage "Mihomo integration tests failed"
Invoke-InDirectory -Path "$repositoryRoot\services\platform-mihomo" -Command { go build ./... } -FailureMessage "Mihomo service build failed"

Write-Host "[3/6] Verifying frontend workspace"
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
$minimumSDKEnvironment = Join-Path ([IO.Path]::GetTempPath()) "paigram-sdk-min-$([guid]::NewGuid().ToString('N'))"
try {
    Invoke-InDirectory -Path "$repositoryRoot\sdks\python" -Command {
        uv venv --python 3.10 $minimumSDKEnvironment
    } -FailureMessage "SDK minimum-version environment creation failed"
    $minimumSDKPython = if ($IsWindows) {
        Join-Path $minimumSDKEnvironment "Scripts\python.exe"
    }
    else {
        Join-Path $minimumSDKEnvironment "bin/python"
    }
    Invoke-InDirectory -Path "$repositoryRoot\sdks\python" -Command {
        uv pip install --python $minimumSDKPython --no-deps .
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
        uv pip install --python $minimumSDKPython "grpcio==1.81.1" "httpx==0.28.0" "protobuf==6.33.5"
    } -FailureMessage "SDK declared minimum dependencies are not installable"
    Invoke-Checked -Command {
        uv run --no-project --python $minimumSDKPython python -c "import asyncio; from paigram_account_sdk import PaiGramAccountClient, PlatformEndpoint; client = PaiGramAccountClient(account_http_url='https://account.invalid', account_grpc_target='localhost:50051', client_id='smoke', client_secret='smoke', platform_endpoints={'mihomo': PlatformEndpoint(target='localhost:9000')}); asyncio.run(client.close())"
    } -FailureMessage "SDK cannot construct and close at its declared minimum dependency versions"
}
finally {
    if (Test-Path -LiteralPath $minimumSDKEnvironment) {
        Remove-Item -LiteralPath $minimumSDKEnvironment -Recurse -Force
    }
}

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
