param(
    [string]$PaiGramCommit = "6e5ded51240754bf3521fbb54f1df3b294f1c616"
)

$ErrorActionPreference = "Stop"

$repositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$sdkRoot = Join-Path $repositoryRoot "sdks\python"
$temporaryRoot = [IO.Path]::GetFullPath((Join-Path $repositoryRoot "tmp"))
$checkoutPath = [IO.Path]::GetFullPath((Join-Path $temporaryRoot "paigram-sdk-compat-$([guid]::NewGuid().ToString('N'))"))
$temporaryPrefix = $temporaryRoot.TrimEnd([IO.Path]::DirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar
if (-not $checkoutPath.StartsWith($temporaryPrefix, [StringComparison]::OrdinalIgnoreCase)) {
    throw "Compatibility checkout escaped the repository temporary directory"
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

New-Item -ItemType Directory -Path $temporaryRoot -Force | Out-Null
try {
    Invoke-Checked -Command {
        gh repo clone PaiGramTeam/PaiGram $checkoutPath -- --filter=blob:none --no-checkout
    } -FailureMessage "PaiGram compatibility checkout failed"
    Invoke-Checked -Command {
        git -C $checkoutPath checkout --detach $PaiGramCommit
    } -FailureMessage "PaiGram compatibility commit checkout failed"

    Push-Location $checkoutPath
    try {
        Invoke-Checked -Command { uv lock --check } -FailureMessage "PaiGram lock validation failed"
        Invoke-Checked -Command {
            uv add --editable $sdkRoot --no-sync
        } -FailureMessage "PaiGram and SDK dependency resolution failed"
    }
    finally {
        Pop-Location
    }

    Invoke-Checked -Command {
        uv run --isolated --no-project --python 3.10 --with-editable $sdkRoot --with "httpx<1.0.0,>=0.28.0" python -c "from paigram_account_sdk import PaiGramAccountClient, PlatformEndpoint; assert PaiGramAccountClient and PlatformEndpoint"
    } -FailureMessage "PaiGram Python 3.10 SDK import failed"
}
finally {
    if (Test-Path -LiteralPath $checkoutPath) {
        $resolvedCheckout = [IO.Path]::GetFullPath($checkoutPath)
        if (-not $resolvedCheckout.StartsWith($temporaryPrefix, [StringComparison]::OrdinalIgnoreCase)) {
            throw "Refusing to remove a compatibility checkout outside the temporary directory"
        }
        Remove-Item -LiteralPath $resolvedCheckout -Recurse -Force
    }
}

