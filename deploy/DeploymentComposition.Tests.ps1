#requires -Version 7.4

Import-Module (Join-Path $PSScriptRoot "DeploymentComposition.psm1") -Force

Describe "Immutable Compose deployment" {
    BeforeEach {
        $binRoot = Join-Path $TestDrive $([Guid]::NewGuid().ToString("N"))
        New-Item -ItemType Directory -Path $binRoot | Out-Null
        $composeCommand = Join-Path $binRoot "podman-compose.ps1"
        @'
$args -join " " | Set-Content -LiteralPath $env:PAI_DEPLOYMENT_COMPOSE_ARGS
if ($env:PAI_DEPLOYMENT_COMPOSE_FAIL -eq "1") { exit 23 }
'@ | Set-Content -LiteralPath $composeCommand
        $env:PAI_DEPLOYMENT_COMPOSE_ARGS = Join-Path $binRoot "arguments.txt"
    }

    AfterEach {
        Remove-Item Env:PAI_DEPLOYMENT_COMPOSE_ARGS, Env:PAI_DEPLOYMENT_COMPOSE_FAIL -ErrorAction SilentlyContinue
    }

    It "always force-recreates immutable services" {
        Invoke-ImmutableComposeDeployment `
            -PodmanCompose $composeCommand `
            -ComposeArguments @("--env-file", ".env", "-p", "recovery", "-f", "compose.yaml") `
            -FailureMessage "deployment failed"

        (Get-Content -LiteralPath $env:PAI_DEPLOYMENT_COMPOSE_ARGS -Raw).Trim() | Should Be "--env-file .env -p recovery -f compose.yaml up --no-build -d --force-recreate"
    }

    It "fails when the Compose provider fails" {
        $env:PAI_DEPLOYMENT_COMPOSE_FAIL = "1"
        try {
            Invoke-ImmutableComposeDeployment `
                -PodmanCompose $composeCommand `
                -ComposeArguments @("-p", "recovery") `
                -FailureMessage "deployment failed"
            throw "Expected deployment failure"
        } catch {
            $_.Exception.Message | Should Be "deployment failed"
        }
    }
}
