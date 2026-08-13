Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Assert-OperationalPathHasNoLinks {
    param([Parameter(Mandatory)][string]$Path)
    $currentPath = [System.IO.Path]::GetFullPath($Path)
    while ($currentPath) {
        $current = Get-Item -LiteralPath $currentPath -Force
        if ($current.LinkType -or ($current.Attributes -band [System.IO.FileAttributes]::ReparsePoint)) {
            throw "Operational paths must not contain symbolic links or junctions"
        }
        $parentPath = [System.IO.Path]::GetDirectoryName($currentPath)
        if (-not $parentPath -or $parentPath -eq $currentPath) {
            break
        }
        $currentPath = $parentPath
    }
}

function Assert-OperationalPrivateFile {
    param(
        [Parameter(Mandatory)][string]$Path,
        [Parameter(Mandatory)][string]$RepositoryRoot
    )
    Assert-OperationalPathHasNoLinks -Path $Path
    $item = Get-Item -LiteralPath $Path -Force
    $resolvedPath = $item.FullName
    $resolvedRepository = (Resolve-Path -LiteralPath $RepositoryRoot).Path.TrimEnd(
        [System.IO.Path]::DirectorySeparatorChar,
        [System.IO.Path]::AltDirectorySeparatorChar
    )
    $comparison = if ($IsWindows) {
        [System.StringComparison]::OrdinalIgnoreCase
    } else {
        [System.StringComparison]::Ordinal
    }
    if ($resolvedPath.Equals($resolvedRepository, $comparison) -or
        $resolvedPath.StartsWith($resolvedRepository + [System.IO.Path]::DirectorySeparatorChar, $comparison)) {
        throw "Private operational files must be outside the repository"
    }
    if (-not $IsWindows) {
        $mode = [System.IO.File]::GetUnixFileMode($resolvedPath)
        $readOnly = [System.IO.UnixFileMode]::UserRead
        $readWrite = $readOnly -bor [System.IO.UnixFileMode]::UserWrite
        if ($mode -ne $readOnly -and $mode -ne $readWrite) {
            throw "Private operational files must have owner-only read or read/write permissions"
        }
        return $resolvedPath
    }
    $currentSID = [System.Security.Principal.WindowsIdentity]::GetCurrent().User
    $acl = Get-Acl -LiteralPath $resolvedPath
    $ownerSID = ([System.Security.Principal.NTAccount]$acl.Owner).Translate(
        [System.Security.Principal.SecurityIdentifier]
    )
    if ($ownerSID -ne $currentSID) {
        throw "Private operational files must be owned by the current user"
    }
    $rules = $acl.GetAccessRules(
        $true,
        $true,
        [System.Security.Principal.SecurityIdentifier]
    )
    $allowedCurrentUser = $false
    foreach ($rule in $rules) {
        if ($rule.AccessControlType -ne [System.Security.AccessControl.AccessControlType]::Allow) {
            continue
        }
        if ($rule.IdentityReference -ne $currentSID) {
            throw "Private operational files must grant access only to the current user"
        }
        $allowedCurrentUser = $true
    }
    if (-not $allowedCurrentUser) {
        throw "Private operational files must grant access to the current user"
    }
    return $resolvedPath
}

function Resolve-OperationalServiceURL {
    param(
        [Parameter(Mandatory)][string]$URL,
        [switch]$AllowLoopbackHTTP
    )
    $uri = [Uri]$URL
    $isHTTPS = $uri.Scheme -eq "https"
    $isLoopbackHTTP = $AllowLoopbackHTTP -and
        $uri.Scheme -eq "http" -and
        $uri.Host -in @("127.0.0.1", "localhost", "[::1]", "::1")
    if ((-not $isHTTPS -and -not $isLoopbackHTTP) -or
        $uri.UserInfo -or
        $uri.Query -or
        $uri.Fragment -or
        $uri.AbsolutePath -ne "/") {
        throw "Service URL must use HTTPS; isolated operations may opt into loopback HTTP"
    }
    return $uri
}

Export-ModuleMember -Function @(
    "Assert-OperationalPathHasNoLinks",
    "Assert-OperationalPrivateFile",
    "Resolve-OperationalServiceURL"
)
