param(
    [Parameter(Mandatory = $true)]
    [ValidateSet(
        "contracts",
        "account-unit",
        "platform-unit",
        "account-integration",
        "platform-integration",
        "production-tracer",
        "sdk",
        "sdk-minimum",
        "paigram-compatibility",
        "frontend",
        "real-browser",
        "repository-hygiene"
    )]
    [string]$Task
)

$ErrorActionPreference = "Stop"
$repositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path

if (-not [string]::Equals($env:CI, "true", [StringComparison]::OrdinalIgnoreCase)) {
    throw "verify-ci.ps1 is a CI-only entry point and requires CI=true"
}

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

function Invoke-GoTestWithoutSkips {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [string[]]$Arguments,
        [string[]]$ExpectedTests = @(),
        [switch]$AllowNoTests,
        [Parameter(Mandatory = $true)]
        [string]$FailureMessage
    )

    $resultPath = Join-Path ([IO.Path]::GetTempPath()) "paigram-go-test-$([guid]::NewGuid().ToString('N')).json"
    Push-Location $Path
    try {
        & go test -json @Arguments 2>&1 | Tee-Object -FilePath $resultPath
        $testExitCode = $LASTEXITCODE
    }
    finally {
        Pop-Location
    }

    try {
        if ($testExitCode -ne 0) {
            throw "$FailureMessage (exit code $testExitCode)"
        }
        $events = @(Get-Content -LiteralPath $resultPath | ForEach-Object {
            if (-not $_.StartsWith("{")) {
                return
            }
            $_ | ConvertFrom-Json
        })
        $skippedTests = @($events | Where-Object {
            $_.Action -eq "skip" -and -not [string]::IsNullOrWhiteSpace($_.Test)
        } | ForEach-Object { "$($_.Package):$($_.Test)" })
        if ($skippedTests.Count -ne 0) {
            $skippedTests | Sort-Object -Unique | ForEach-Object { Write-Error "Skipped test: $_" }
            throw "Go tests must not silently skip in CI"
        }
        $executedTests = @($events | Where-Object {
            $_.Action -eq "run" -and -not [string]::IsNullOrWhiteSpace($_.Test)
        } | ForEach-Object { $_.Test } | Sort-Object -Unique)
        if (-not $AllowNoTests -and $executedTests.Count -eq 0) {
            throw "Go test selection did not execute any tests"
        }
        foreach ($expectedTest in $ExpectedTests) {
            if ($executedTests -notcontains $expectedTest) {
                throw "Expected Go test was not executed: $expectedTest"
            }
        }
    }
    finally {
        Remove-Item -LiteralPath $resultPath -Force -ErrorAction SilentlyContinue
    }
}

function Assert-TrackedPathsClean {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$Paths
    )

    $changes = @(git -C $repositoryRoot status --porcelain=v1 -- @Paths)
    if ($LASTEXITCODE -ne 0) {
        throw "Could not inspect generated artifacts (exit code $LASTEXITCODE)"
    }
    if ($changes.Count -ne 0) {
        $changes | ForEach-Object { Write-Error $_ }
        throw "Generated artifacts differ from the committed contract"
    }
}

function Assert-RepositoryClean {
    $changes = @(git -C $repositoryRoot status --porcelain=v1 --untracked-files=all)
    if ($LASTEXITCODE -ne 0) {
        throw "Could not inspect repository status (exit code $LASTEXITCODE)"
    }
    if ($changes.Count -ne 0) {
        $changes | ForEach-Object { Write-Error $_ }
        throw "CI task modified the checkout"
    }
}

function Assert-GoFormatted {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    $unformatted = @(Get-ChildItem -LiteralPath $Path -Recurse -File -Filter "*.go" | ForEach-Object {
        $output = @(gofmt -l $_.FullName)
        if ($LASTEXITCODE -ne 0) {
            throw "gofmt failed for $($_.FullName) (exit code $LASTEXITCODE)"
        }
        $output
    })
    if ($unformatted.Count -ne 0) {
        $unformatted | ForEach-Object { Write-Error $_ }
        throw "Go source files are not formatted"
    }
}

function Invoke-Contracts {
    Assert-GoFormatted -Path "$repositoryRoot/contracts/runtime/go"
    Assert-GoFormatted -Path "$repositoryRoot/contracts/gen/go"
    Invoke-Checked -Command { & "$repositoryRoot/contracts/generate.ps1" } -FailureMessage "Contract generation failed"
    Invoke-InDirectory -Path "$repositoryRoot/services/account-center" -Command {
        go run ./cmd/paigram openapi --out ../../contracts/openapi.json
    } -FailureMessage "OpenAPI generation failed"
    Invoke-InDirectory -Path "$repositoryRoot/frontend" -Command { bun install --frozen-lockfile } -FailureMessage "Frontend install failed"
    Invoke-InDirectory -Path "$repositoryRoot/frontend" -Command { bun run openapi:gen } -FailureMessage "Frontend OpenAPI type generation failed"
    Assert-TrackedPathsClean -Paths @(
        "contracts/gen/go",
        "contracts/openapi.json",
        "sdks/python/src/paigram_account_sdk/_generated",
        "frontend/packages/shared-components/src/api/generated/schema.ts"
    )
    Invoke-GoTestWithoutSkips -Path "$repositoryRoot/contracts/runtime/go" -Arguments @("./...") -FailureMessage "Runtime contract tests failed"
    Invoke-GoTestWithoutSkips -Path "$repositoryRoot/contracts/gen/go" -Arguments @("./...") -AllowNoTests -FailureMessage "Generated Go contracts failed to compile"
    Assert-RepositoryClean
}

function Invoke-AccountUnit {
    if ([string]::IsNullOrWhiteSpace($env:PAI_TEST_DATABASE_DSN) -and [string]::IsNullOrWhiteSpace($env:PAI_DATABASE_DSN)) {
        throw "Account Center database tests require PAI_TEST_DATABASE_DSN or PAI_DATABASE_DSN"
    }
    if (-not [string]::Equals($env:PAI_REQUIRE_DATABASE_TESTS, "true", [StringComparison]::OrdinalIgnoreCase)) {
        throw "Account Center CI must set PAI_REQUIRE_DATABASE_TESTS=true"
    }
    Assert-GoFormatted -Path "$repositoryRoot/services/account-center"
    Invoke-GoTestWithoutSkips -Path "$repositoryRoot/services/account-center" -Arguments @("-count=1", "./...") -FailureMessage "Account Center tests failed"
    Invoke-InDirectory -Path "$repositoryRoot/services/account-center" -Command { go vet ./... } -FailureMessage "Account Center vet failed"
    Invoke-InDirectory -Path "$repositoryRoot/services/account-center" -Command { go build ./... } -FailureMessage "Account Center build failed"
    Assert-RepositoryClean
}

function Invoke-PlatformUnit {
    Assert-GoFormatted -Path "$repositoryRoot/services/platform-mihomo"
    Invoke-GoTestWithoutSkips -Path "$repositoryRoot/services/platform-mihomo" -Arguments @("-count=1", "./...") -FailureMessage "Platform tests failed"
    Invoke-InDirectory -Path "$repositoryRoot/services/platform-mihomo" -Command { go vet ./... } -FailureMessage "Platform vet failed"
    Invoke-InDirectory -Path "$repositoryRoot/services/platform-mihomo" -Command { go build ./... } -FailureMessage "Platform build failed"
    Assert-RepositoryClean
}

function Assert-NoSkippedTestMarkers {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$Paths
    )

    $forbiddenSkippedTests = @(rg -n `
        "(test|it|describe)\.(skip|todo)\(|pytest\.(skip|xfail)|pytest\.mark\.(skip|skipif|xfail)" `
        @Paths
    )
    if ($LASTEXITCODE -notin @(0, 1)) {
        throw "Could not inspect skipped tests (exit code $LASTEXITCODE)"
    }
    if ($forbiddenSkippedTests.Count -ne 0) {
        $forbiddenSkippedTests | ForEach-Object { Write-Error $_ }
        throw "Committed tests must not be skipped or marked todo"
    }
}

function Assert-JUnitHasNoSkippedTests {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    [xml]$report = Get-Content -LiteralPath $Path -Raw
    $suites = @($report.SelectNodes("//testsuite"))
    if ($suites.Count -eq 0) {
        throw "Test runner did not produce any JUnit test suites"
    }
    $testCount = ($suites | Measure-Object -Property tests -Sum).Sum
    $skippedCount = ($suites | Measure-Object -Property skipped -Sum).Sum
    $disabledCount = ($suites | Measure-Object -Property disabled -Sum).Sum
    if ([int]$testCount -eq 0) {
        throw "Test runner did not execute any tests"
    }
    if ([int]$skippedCount -ne 0 -or [int]$disabledCount -ne 0) {
        throw "Test runner reported skipped or disabled tests"
    }
}

function Invoke-SDK {
    Assert-NoSkippedTestMarkers -Paths @("$repositoryRoot/sdks/python/tests")
    Invoke-InDirectory -Path "$repositoryRoot/sdks/python" -Command { uv lock --check } -FailureMessage "SDK lockfile is stale"
    Invoke-InDirectory -Path "$repositoryRoot/sdks/python" -Command { uv sync --all-groups --locked } -FailureMessage "SDK install failed"
    Invoke-InDirectory -Path "$repositoryRoot/sdks/python" -Command { uv run ruff check . } -FailureMessage "SDK lint failed"
    Invoke-InDirectory -Path "$repositoryRoot/sdks/python" -Command { uv run ruff format --check . } -FailureMessage "SDK format failed"
    Invoke-InDirectory -Path "$repositoryRoot/sdks/python" -Command { uv run mypy } -FailureMessage "SDK type check failed"
    $pytestResult = Join-Path ([IO.Path]::GetTempPath()) "paigram-sdk-tests-$([guid]::NewGuid().ToString('N')).xml"
    try {
        Invoke-InDirectory -Path "$repositoryRoot/sdks/python" -Command {
            uv run pytest -q --junitxml=$pytestResult
        } -FailureMessage "SDK tests failed"
        Assert-JUnitHasNoSkippedTests -Path $pytestResult
    }
    finally {
        Remove-Item -LiteralPath $pytestResult -Force -ErrorAction SilentlyContinue
    }
    Invoke-InDirectory -Path "$repositoryRoot/sdks/python" -Command { uv build } -FailureMessage "SDK package build failed"
    Assert-RepositoryClean
}

function Invoke-SDKMinimum {
    $environmentPath = Join-Path ([IO.Path]::GetTempPath()) "paigram-sdk-min-$([guid]::NewGuid().ToString('N'))"
    try {
        Invoke-InDirectory -Path "$repositoryRoot/sdks/python" -Command {
            uv venv --python 3.10 $environmentPath
        } -FailureMessage "SDK minimum-version environment creation failed"
        $pythonPath = if ($IsWindows) {
            Join-Path $environmentPath "Scripts/python.exe"
        }
        else {
            Join-Path $environmentPath "bin/python"
        }
        Invoke-InDirectory -Path "$repositoryRoot/sdks/python" -Command {
            uv pip install --python $pythonPath --no-deps .
        } -FailureMessage "SDK package is not installable at minimum versions"
        Invoke-InDirectory -Path "$repositoryRoot/sdks/python" -Command {
            uv pip install --python $pythonPath `
                "grpcio==1.81.1" "httpx==0.28.0" "protobuf==6.33.5" `
                "pytest==9.0.0" "pytest-asyncio==1.3.0"
        } -FailureMessage "SDK minimum test dependencies are not installable"
        $pytestResult = Join-Path ([IO.Path]::GetTempPath()) "paigram-sdk-min-tests-$([guid]::NewGuid().ToString('N')).xml"
        try {
            Invoke-InDirectory -Path "$repositoryRoot/sdks/python" -Command {
                uv run --no-project --python $pythonPath python -m pytest -q --junitxml=$pytestResult
            } -FailureMessage "SDK tests failed at minimum dependency versions"
            Assert-JUnitHasNoSkippedTests -Path $pytestResult
        }
        finally {
            Remove-Item -LiteralPath $pytestResult -Force -ErrorAction SilentlyContinue
        }
        Invoke-Checked -Command {
            uv run --no-project --python $pythonPath python -c "import asyncio; from paigram_account_sdk import PaiGramAccountClient; client = PaiGramAccountClient(account_http_url='https://account.invalid', account_grpc_target='localhost:50051', client_id='smoke', client_secret='smoke'); asyncio.run(client.close())"
        } -FailureMessage "SDK cannot construct and close at minimum versions"
    }
    finally {
        if (Test-Path -LiteralPath $environmentPath) {
            Remove-Item -LiteralPath $environmentPath -Recurse -Force
        }
    }
}

switch ($Task) {
    "contracts" { Invoke-Contracts }
    "account-unit" { Invoke-AccountUnit }
    "platform-unit" { Invoke-PlatformUnit }
    "account-integration" {
        Invoke-GoTestWithoutSkips -Path "$repositoryRoot/services/account-center" -Arguments @(
            "-count=1", "-tags=integration",
            "-skip=^(TestPythonSDKCallsProductionPlatformWithAccountIssuedTicket|TestPythonSDKDiscoversRuntimeRouteAcrossTLSListeners)$",
            "./integration"
        ) -FailureMessage "Account Center integration tests failed"
        Assert-RepositoryClean
    }
    "platform-integration" {
        Invoke-GoTestWithoutSkips -Path "$repositoryRoot/services/platform-mihomo" -Arguments @(
            "-count=1", "-tags=integration", "./integration"
        ) -FailureMessage "Platform integration tests failed"
        Assert-RepositoryClean
    }
    "production-tracer" {
        Invoke-GoTestWithoutSkips -Path "$repositoryRoot/services/account-center" -Arguments @(
            "-count=1", "-tags=integration",
            "-run=^(TestPythonSDKCallsProductionPlatformWithAccountIssuedTicket|TestPythonSDKDiscoversRuntimeRouteAcrossTLSListeners)$",
            "./integration"
        ) -ExpectedTests @(
            "TestPythonSDKCallsProductionPlatformWithAccountIssuedTicket",
            "TestPythonSDKDiscoversRuntimeRouteAcrossTLSListeners"
        ) -FailureMessage "Production TLS tracer failed"
        Assert-RepositoryClean
    }
    "sdk" { Invoke-SDK }
    "sdk-minimum" { Invoke-SDKMinimum }
    "paigram-compatibility" {
        Invoke-Checked -Command { & "$repositoryRoot/scripts/verify-paigram-sdk.ps1" } -FailureMessage "PaiGram SDK compatibility failed"
        Assert-RepositoryClean
    }
    "frontend" {
        Assert-NoSkippedTestMarkers -Paths @("$repositoryRoot/frontend/tests")
        Invoke-InDirectory -Path "$repositoryRoot/frontend" -Command { bun install --frozen-lockfile } -FailureMessage "Frontend install failed"
        Invoke-InDirectory -Path "$repositoryRoot/frontend" -Command { bun run format:check } -FailureMessage "Frontend format check failed"
        Invoke-InDirectory -Path "$repositoryRoot/frontend" -Command { bun run lint } -FailureMessage "Frontend lint failed"
        Invoke-InDirectory -Path "$repositoryRoot/frontend" -Command { bun run type-check } -FailureMessage "Frontend type check failed"
        $bunTestResult = Join-Path ([IO.Path]::GetTempPath()) "paigram-frontend-tests-$([guid]::NewGuid().ToString('N')).xml"
        try {
            Invoke-InDirectory -Path "$repositoryRoot/frontend" -Command {
                bun test tests/unit --reporter=junit --reporter-outfile=$bunTestResult
            } -FailureMessage "Frontend tests failed"
            Assert-JUnitHasNoSkippedTests -Path $bunTestResult
        }
        finally {
            Remove-Item -LiteralPath $bunTestResult -Force -ErrorAction SilentlyContinue
        }
        Invoke-InDirectory -Path "$repositoryRoot/frontend" -Command { bun run build:all } -FailureMessage "Frontend builds failed"
        Assert-RepositoryClean
    }
    "real-browser" {
        Assert-NoSkippedTestMarkers -Paths @("$repositoryRoot/frontend/tests/e2e-real")
        $forbiddenDoubles = @(rg -n `
            'page\.route|route\.fulfill|setupWorker|setupServer|\bmsw\b' `
            "$repositoryRoot/frontend/tests/e2e-real" `
            "$repositoryRoot/frontend/playwright.real.config.ts"
        )
        if ($LASTEXITCODE -notin @(0, 1)) {
            throw "Could not inspect real-browser test doubles (exit code $LASTEXITCODE)"
        }
        if ($forbiddenDoubles.Count -ne 0) {
            $forbiddenDoubles | ForEach-Object { Write-Error $_ }
            throw "Real-browser acceptance must not intercept or mock application requests"
        }
        Invoke-InDirectory -Path "$repositoryRoot/frontend" -Command { bun install --frozen-lockfile } -FailureMessage "Frontend install failed"
        Invoke-InDirectory -Path "$repositoryRoot/frontend" -Command { bunx playwright install --with-deps chromium } -FailureMessage "Chromium install failed"
        $playwrightResult = Join-Path ([IO.Path]::GetTempPath()) "paigram-real-browser-$([guid]::NewGuid().ToString('N')).xml"
        try {
            $env:PAI_E2E_JUNIT_PATH = $playwrightResult
            Invoke-InDirectory -Path "$repositoryRoot/frontend" -Command {
                bun run e2e:real
            } -FailureMessage "Real-browser system acceptance failed"
            Assert-JUnitHasNoSkippedTests -Path $playwrightResult
        }
        finally {
            Remove-Item Env:PAI_E2E_JUNIT_PATH -ErrorAction SilentlyContinue
            Remove-Item -LiteralPath $playwrightResult -Force -ErrorAction SilentlyContinue
        }
        Assert-RepositoryClean
    }
    "repository-hygiene" {
        $diffBase = $env:PAI_CI_DIFF_BASE
        if ([string]::IsNullOrWhiteSpace($diffBase) -or $diffBase -match "^0+$") {
            Invoke-InDirectory -Path $repositoryRoot -Command {
                git diff-tree --check --root -r HEAD
            } -FailureMessage "Repository whitespace check failed"
        }
        else {
            Invoke-InDirectory -Path $repositoryRoot -Command {
                git rev-parse --verify "$diffBase^{commit}" | Out-Null
            } -FailureMessage "CI diff base is not available"
            Invoke-InDirectory -Path $repositoryRoot -Command {
                git diff --check "$diffBase...HEAD"
            } -FailureMessage "Repository whitespace check failed"
        }
        Assert-RepositoryClean
    }
}

Write-Host "CI task '$Task' completed successfully."
