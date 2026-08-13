Set-StrictMode -Version Latest

function Invoke-ImmutableComposeDeployment {
    param(
        [Parameter(Mandatory)][string]$PodmanCompose,
        [Parameter(Mandatory)][string[]]$ComposeArguments,
        [Parameter(Mandatory)][string]$FailureMessage
    )
    $global:LASTEXITCODE = 0
    & $PodmanCompose @ComposeArguments up --no-build -d --force-recreate
    if ($LASTEXITCODE -ne 0) {
        throw $FailureMessage
    }
}

Export-ModuleMember -Function Invoke-ImmutableComposeDeployment
