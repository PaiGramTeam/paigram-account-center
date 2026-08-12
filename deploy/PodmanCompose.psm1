Set-StrictMode -Version Latest

function Assert-PodmanComposeAvailable {
    param([version]$MinimumVersion = [version]"1.6.0")

    $command = Get-Command podman-compose -ErrorAction SilentlyContinue
    if (-not $command) {
        throw "podman-compose $MinimumVersion or later is required for Podman external secrets"
    }
    $output = ((@(& $command.Source version)) -join "`n").Trim()
    if ($LASTEXITCODE -ne 0 -or $output -notmatch 'podman-compose version (?<version>\d+\.\d+\.\d+)') {
        throw "Could not determine the podman-compose version"
    }
    $installed = [version]$Matches.version
    if ($installed -lt $MinimumVersion) {
        throw "podman-compose $MinimumVersion or later is required; found $installed"
    }
    return $command.Source
}

Export-ModuleMember -Function Assert-PodmanComposeAvailable
