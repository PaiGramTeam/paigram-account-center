#requires -Version 7.4

Describe "Production database bootstrap" {
    BeforeAll {
        $accountCompose = Get-Content -LiteralPath (Join-Path $PSScriptRoot "podman/compose.yaml") -Raw
        $platformCompose = Get-Content -LiteralPath (Join-Path $PSScriptRoot "podman-platform-mihomo/compose.yaml") -Raw
        $accountConfig = Get-Content -LiteralPath (Join-Path $PSScriptRoot "podman/config.yaml") -Raw
    }

    It "gates Account Center on explicit migration and seed jobs" {
        $accountCompose | Should Match '(?m)^  migrate:'
        $accountCompose | Should Match 'profiles: \[bootstrap\]'
        $accountCompose | Should Match 'command: \["migrate", "up"\]'
        $accountCompose | Should Match '(?m)^  seed:'
        $accountCompose | Should Match 'command: \["seed", "all", "--with-admin"\]'
        $accountConfig | Should Match '(?m)^  auto_migrate: false$'
        $accountConfig | Should Match '(?m)^  auto_seed: false$'
    }

    It "gates Platform Mihomo on an explicit migration job" {
        $platformCompose | Should Match '(?m)^  migrate:'
        $platformCompose | Should Match 'profiles: \[bootstrap\]'
        $platformCompose | Should Match 'command: \["-conf", "/opt/platform-mihomo/config/config.yaml", "migrate"\]'
    }
}
