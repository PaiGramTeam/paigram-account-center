#requires -Version 7.4

Describe "Production database bootstrap" {
    BeforeAll {
        $accountCompose = Get-Content -LiteralPath (Join-Path $PSScriptRoot "podman/compose.yaml") -Raw
        $platformCompose = Get-Content -LiteralPath (Join-Path $PSScriptRoot "podman-platform-mihomo/compose.yaml") -Raw
        $accountConfig = Get-Content -LiteralPath (Join-Path $PSScriptRoot "podman/config.yaml") -Raw
        $accountGRPCTLS = Get-Content -LiteralPath (Join-Path $PSScriptRoot "podman/compose.grpc-tls.yaml") -Raw
        $accountControlTLS = Get-Content -LiteralPath (Join-Path $PSScriptRoot "podman/compose.platform-control-tls.yaml") -Raw
        $platformControlTLS = Get-Content -LiteralPath (Join-Path $PSScriptRoot "podman-platform-mihomo/compose.control-tls.yaml") -Raw
        $platformRuntimeTLS = Get-Content -LiteralPath (Join-Path $PSScriptRoot "podman-platform-mihomo/compose.runtime-tls.yaml") -Raw
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

    It "does not inherit ambient host proxy settings into application services" {
        foreach ($name in @("HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy")) {
            ([regex]::Matches($accountCompose, "(?m)^      ${name}: `"`"$")).Count | Should Be 2
            ([regex]::Matches($platformCompose, "(?m)^      ${name}: `"`"$")).Count | Should Be 1
        }
    }

    It "disables proxy use in container-local health probes" {
        $accountCompose | Should Match ([regex]::Escape('wget", "-Y", "off", "-q", "-O", "/dev/null", "http://127.0.0.1:8080/readyz'))
        $accountCompose | Should Match ([regex]::Escape('wget", "-Y", "off", "-q", "-O", "/dev/null", "http://127.0.0.1:8080/nginx-health'))
    }

    It "keeps gRPC TLS out of the default compositions" {
        $accountCompose | Should Not Match 'PAI_GRPC_CERTIFICATE_FILE'
        $accountCompose | Should Not Match 'PAI_PLATFORM_CONTROL_ROOT_CA_FILE'
        $platformCompose | Should Not Match 'PAI_SERVER_CONTROL_TLS_CERTIFICATE_FILE'
        $platformCompose | Should Not Match 'PAI_SERVER_RUNTIME_TLS_CERTIFICATE_FILE'
        $platformCompose | Should Match ([regex]::Escape('"-target", "127.0.0.1:9001"]'))
    }

    It "provides independent opt-in TLS compositions" {
        $accountGRPCTLS | Should Match 'PAI_GRPC_CERTIFICATE_FILE'
        $accountControlTLS | Should Match 'PAI_PLATFORM_CONTROL_ROOT_CA_FILE'
        $platformControlTLS | Should Match 'PAI_SERVER_CONTROL_TLS_CERTIFICATE_FILE'
        $platformRuntimeTLS | Should Match 'PAI_SERVER_RUNTIME_TLS_CERTIFICATE_FILE'
    }
}
