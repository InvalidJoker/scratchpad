<#
.SYNOPSIS
Install Scratchpad (sp) from a GitHub release, with completions and the
spo/spn wrappers wired into your PowerShell profile.

.DESCRIPTION
Quick install:

    irm https://raw.githubusercontent.com/InvalidJoker/scratchpad/main/scripts/install.ps1 | iex

With options — `iex` cannot take parameters, so build a scriptblock instead:

    & ([scriptblock]::Create((irm https://raw.githubusercontent.com/InvalidJoker/scratchpad/main/scripts/install.ps1))) -Dir C:\tools\sp

.PARAMETER Version
Release tag to install. Defaults to $env:SP_VERSION, then the latest release.

.PARAMETER Dir
Where sp.exe goes. Defaults to $env:SP_INSTALL_DIR, then
%LOCALAPPDATA%\Programs\Scratchpad.

.PARAMETER NoCompletions
Skip tab completion.

.PARAMETER NoShellInit
Skip the spo/spn wrappers.
#>
[CmdletBinding()]
param(
    [string]$Version = $env:SP_VERSION,
    [string]$Dir = $env:SP_INSTALL_DIR,
    [switch]$NoCompletions = [bool]$env:SP_NO_COMPLETIONS,
    [switch]$NoShellInit = [bool]$env:SP_NO_SHELL_INIT
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
# Invoke-WebRequest's progress bar costs more than the download on PS 5.1.
$ProgressPreference = 'SilentlyContinue'

$Repo = 'InvalidJoker/scratchpad'
$BlockStart = '# >>> scratchpad shell integration >>>'
$BlockEnd = '# <<< scratchpad shell integration <<<'

function Write-Step($Message) { Write-Host "  $Message" }
function Write-Note($Message) { Write-Warning $Message }

function Get-Arch {
    # PROCESSOR_ARCHITECTURE reports the *process* architecture, so a 32-bit
    # host would pick the wrong archive; OSArchitecture is what the release is
    # named after. It needs .NET 4.7.1, hence the environment fallback for old
    # Windows PowerShell hosts.
    $arch = $null
    try {
        $arch = [string][System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
    } catch {
        # Windows PowerShell on an older .NET: fall back to the environment.
        $arch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
    }
    switch -Regex ($arch) {
        '^(X64|AMD64)$' { return 'amd64' }
        '^ARM64$' { return 'arm64' }
        default { throw "unsupported architecture $arch: only amd64 and arm64 are published" }
    }
}

function Get-LatestVersion {
    # The API is the only place that answers this in one hop on both PS 5.1 and
    # 7; the HTML redirect is the fallback when it rate-limits.
    try {
        return (Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -UseBasicParsing).tag_name
    } catch {
        try {
            $response = Invoke-WebRequest -Uri "https://github.com/$Repo/releases/latest" -UseBasicParsing
            $final = if ($response.BaseResponse.PSObject.Properties['RequestMessage']) {
                $response.BaseResponse.RequestMessage.RequestUri.AbsoluteUri
            } else {
                $response.BaseResponse.ResponseUri.AbsoluteUri
            }
            if ($final -match '/tag/(?<tag>[^/]+)$') { return $Matches.tag }
        } catch {
            # Fall through to the shared error below.
        }
    }
    throw 'could not determine the latest release: pass -Version <tag>'
}

function Test-Checksum($Path, $Name, $SumsFile) {
    if (-not (Test-Path -LiteralPath $SumsFile)) {
        Write-Note "no checksums published, skipping verification"
        return
    }
    $line = Select-String -Path $SumsFile -Pattern ([regex]::Escape($Name) + '$') |
        Select-Object -First 1
    if (-not $line) {
        Write-Note "no checksum published for $Name, skipping verification"
        return
    }
    $expected = ($line.Line -split '\s+')[0]
    # goreleaser writes lowercase hex, Get-FileHash returns uppercase: -ine
    # states the case-insensitive comparison rather than relying on the default.
    $actual = (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash
    if ($expected -ine $actual) {
        throw "checksum mismatch for $Name"
    }
    Write-Step 'checksum ok'
}

# Windows PowerShell defaults to TLS 1.0, which github.com refuses. PowerShell 7
# negotiates on its own, and forcing 1.2 there would take 1.3 away from it.
if ($PSVersionTable.PSVersion.Major -lt 6) {
    [Net.ServicePointManager]::SecurityProtocol =
        [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
}

$arch = Get-Arch
if (-not $Version) { $Version = Get-LatestVersion }
if (-not $Dir) { $Dir = Join-Path $env:LOCALAPPDATA 'Programs\Scratchpad' }

# Release archives are named with the bare version; tags carry the v.
$number = $Version -replace '^v', ''
$archive = "sp_${number}_windows_${arch}.zip"
$base = "https://github.com/$Repo/releases/download/$Version"

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("scratchpad-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    Write-Host "Scratchpad $Version (windows/$arch)"
    Write-Step "downloading $archive"
    $zip = Join-Path $tmp $archive
    try {
        Invoke-WebRequest -Uri "$base/$archive" -OutFile $zip -UseBasicParsing
    } catch {
        throw "download failed: $base/$archive"
    }

    $sums = Join-Path $tmp 'checksums.txt'
    try {
        Invoke-WebRequest -Uri "$base/checksums.txt" -OutFile $sums -UseBasicParsing
    } catch {
        Remove-Item -LiteralPath $sums -ErrorAction SilentlyContinue
    }
    Test-Checksum -Path $zip -Name $archive -SumsFile $sums

    $extracted = Join-Path $tmp 'extracted'
    Expand-Archive -LiteralPath $zip -DestinationPath $extracted -Force
    $binary = Join-Path $extracted 'sp.exe'
    if (-not (Test-Path -LiteralPath $binary)) { throw "archive did not contain sp.exe" }

    if (-not (Test-Path -LiteralPath $Dir)) {
        New-Item -ItemType Directory -Path $Dir -Force | Out-Null
    }
    $target = Join-Path $Dir 'sp.exe'
    try {
        Move-Item -LiteralPath $binary -Destination $target -Force
    } catch {
        throw "could not write $target — close any running sp and try again, or pass -Dir <path>"
    }
    Write-Step "installed $target"

    # --- completions and the spo/spn wrappers --------------------------------

    # Windows has no drop-in completion directory, so everything the shell needs
    # lives in one file that the profile dot-sources. Reinstalling refreshes the
    # file without touching the profile again.
    $profileScript = Join-Path $Dir 'sp.profile.ps1'
    $parts = @()

    if ($NoCompletions) {
        Write-Step 'completions: skipped'
    } else {
        # Prefer the copy shipped in the archive, fall back to the binary we
        # just installed.
        $shipped = Join-Path $extracted 'completions\sp.ps1'
        if (Test-Path -LiteralPath $shipped) {
            $completion = Get-Content -LiteralPath $shipped -Raw
        } else {
            $completion = (& $target completion powershell) -join "`n"
        }
        $parts += $completion
        Write-Step "completions: $profileScript"
    }

    if ($NoShellInit) {
        Write-Step 'shell integration: skipped'
    } else {
        $parts += (& $target shell-init powershell) -join "`n"
        Write-Step 'shell integration: spo and spn'
    }

    if ($parts.Count -gt 0) {
        Set-Content -LiteralPath $profileScript -Value ($parts -join "`n`n") -Encoding UTF8

        $profilePath = $PROFILE.CurrentUserCurrentHost
        $profileDir = Split-Path -Parent $profilePath
        if (-not (Test-Path -LiteralPath $profileDir)) {
            New-Item -ItemType Directory -Path $profileDir -Force | Out-Null
        }
        $existing = if (Test-Path -LiteralPath $profilePath) {
            Get-Content -LiteralPath $profilePath -Raw
        } else {
            ''
        }
        if ($existing -like "*$BlockStart*") {
            Write-Step "profile: already loads sp, left alone ($profilePath)"
        } else {
            $block = @($BlockStart, ". `"$profileScript`"", $BlockEnd) -join "`n"
            Add-Content -LiteralPath $profilePath -Value "`n$block" -Encoding UTF8
            Write-Step "profile: added to $profilePath"
        }
    }

    # --- PATH ---------------------------------------------------------------

    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $onPath = @($userPath -split ';' | Where-Object { $_.TrimEnd('\') -eq $Dir.TrimEnd('\') }).Count -gt 0
    if ($onPath) {
        Write-Step 'PATH: already set'
    } else {
        $updated = if ([string]::IsNullOrEmpty($userPath)) { $Dir } else { "$userPath;$Dir" }
        [Environment]::SetEnvironmentVariable('Path', $updated, 'User')
        # The persisted value only reaches new processes, so patch this one too.
        $env:Path = "$env:Path;$Dir"
        Write-Step "PATH: added $Dir"
    }

    Write-Host ''
    Write-Host 'Done. Open a new PowerShell, then run: sp setup'
} finally {
    Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
}
