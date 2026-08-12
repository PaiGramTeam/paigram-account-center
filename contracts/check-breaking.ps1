$ErrorActionPreference = "Stop"

$repositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$temporaryRoot = [IO.Path]::GetFullPath((Join-Path $repositoryRoot "tmp"))
$checkRoot = [IO.Path]::GetFullPath((Join-Path $temporaryRoot "contract-breaking-$([guid]::NewGuid().ToString('N'))"))
$temporaryPrefix = $temporaryRoot.TrimEnd([IO.Path]::DirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar
if (-not $checkRoot.StartsWith($temporaryPrefix, [StringComparison]::OrdinalIgnoreCase)) {
    throw "Breaking-check directory escaped the repository temporary directory"
}

$protoContractsSource = "https://github.com/PaiGramTeam/proto-contracts.git#format=git,commit=355561643fb141dd8067bd1b98db43f71627d004"
$accountCenterSource = "https://github.com/PaiGramTeam/account-center.git#format=git,commit=fde72872d19acb10817eed6262453afc55b7d1dc"

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

New-Item -ItemType Directory -Path $checkRoot -Force | Out-Null
Push-Location $PSScriptRoot
try {
    $sourceShared = Join-Path $checkRoot "source-shared.binpb"
    $currentShared = Join-Path $checkRoot "current-shared.binpb"
    $sourceAccount = Join-Path $checkRoot "source-account.binpb"
    $currentAccount = Join-Path $checkRoot "current-account.binpb"

    Invoke-Checked -Command {
        buf build $protoContractsSource --path platform/v1 -o $sourceShared
    } -FailureMessage "Could not build the source shared-contract baseline"
    Invoke-Checked -Command {
        buf build . --path proto/platform/v1 -o $currentShared
    } -FailureMessage "Could not build the current shared contracts"
    Invoke-Checked -Command {
        buf breaking $currentShared --against $sourceShared --config buf.breaking.yaml
    } -FailureMessage "Shared contracts contain a wire-breaking change"

    Invoke-Checked -Command {
        buf build $accountCenterSource --path proto/paigram/v1/bot_access.proto -o $sourceAccount
    } -FailureMessage "Could not build the source Account Center contract baseline"
    Invoke-Checked -Command {
        buf build . --path proto/account/v1/bot_access.proto -o $currentAccount
    } -FailureMessage "Could not build the current Account Center bot contract"
    Invoke-Checked -Command {
        buf breaking $currentAccount --against $sourceAccount --config buf.breaking.yaml
    } -FailureMessage "Account Center bot contract contains a wire-breaking change"
}
finally {
    Pop-Location
    if (Test-Path -LiteralPath $checkRoot) {
        $resolvedCheckRoot = [IO.Path]::GetFullPath($checkRoot)
        if (-not $resolvedCheckRoot.StartsWith($temporaryPrefix, [StringComparison]::OrdinalIgnoreCase)) {
            throw "Refusing to remove a breaking-check directory outside the temporary directory"
        }
        Remove-Item -LiteralPath $resolvedCheckRoot -Recurse -Force
    }
}
