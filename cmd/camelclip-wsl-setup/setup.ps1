param([string]$VersionTag,[string]$Arch,[string]$Digest,[string]$Launcher)
$ErrorActionPreference = 'Stop'
$wsl = Join-Path $env:SystemRoot 'System32\wsl.exe'
function Get-Distros {
    if (!(Test-Path $wsl)) { return }
    $lines = & $wsl --list --verbose 2>$null
    if ($LASTEXITCODE -ne 0) { return }
    foreach ($line in $lines) {
        if (($line -replace "\x00",'') -match '^\s*\*?\s*([\w.-]+)\s+.+?\s+2\s*$' -and $Matches[1] -notmatch '^(docker-desktop|podman-machine|rancher-desktop)') { $Matches[1] }
    }
}
$distros = @(Get-Distros)
if (!$distros.Count) {
    if (!(Test-Path $wsl)) { throw 'Install or update WSL 2 using Microsoft instructions, then reopen setup.' }
    if ((Read-Host 'Enable WSL 2 and install Ubuntu? A restart may be required [y/N]') -ne 'y') { exit 1 }
    $resume = Join-Path $env:LOCALAPPDATA 'CamelClip\Setup'
    New-Item -Force -ItemType Directory $resume | Out-Null
    $copy = Join-Path $resume 'CamelClip-Setup.exe'
    if ($Launcher -ne $copy) { Copy-Item -LiteralPath $Launcher -Destination $copy -Force }
    $runOnce = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\RunOnce'
    if (!(Test-Path $runOnce)) { New-Item $runOnce | Out-Null }
    New-ItemProperty -Path $runOnce -Name CamelClipSetup -Value ('"'+$copy+'"') -PropertyType String -Force | Out-Null
    $result = Start-Process $wsl -ArgumentList '--install -d Ubuntu --no-launch' -Verb RunAs -Wait -PassThru
    if ($result.ExitCode -notin 0,3010) {
        Remove-ItemProperty $runOnce CamelClipSetup
        throw "WSL setup failed ($($result.ExitCode)). Check virtualization and Windows Update."
    }
    $distros = @(Get-Distros)
    if (!$distros.Count) { Read-Host 'Restart Windows to continue setup after signing in. Press Enter to close'; exit 0 }
    Remove-ItemProperty $runOnce CamelClipSetup
}
$distro = $distros[0]
if ($distros.Count -gt 1) {
    Write-Host ('Available WSL 2 distributions: ' + ($distros -join ', '))
    $choice = Read-Host "Distribution [$distro]"
    if ($choice) { if ($choice -notin $distros) { throw 'Unknown distribution' }; $distro = $choice }
}
Write-Host "Installing inside $distro. Complete its Linux user setup if prompted."
$guestArch = (& $wsl -d $distro --exec uname -m | Out-String).Trim()
if ($LASTEXITCODE -ne 0 -or $guestArch -ne @{'amd64'='x86_64';'arm64'='aarch64'}[$Arch]) { throw 'Choose the installer matching your Linux architecture.' }
$name = "camelclip_setup_${VersionTag}_linux_${Arch}"
$url = "https://github.com/bfcamel/CamelClip/releases/download/v$VersionTag/$name.tar.gz"
$temp = Join-Path ([IO.Path]::GetTempPath()) ([guid]::NewGuid().ToString()+'.tar.gz')
try {
    Invoke-WebRequest -UseBasicParsing $url -OutFile $temp
    if ((Get-FileHash $temp -Algorithm SHA256).Hash -ne $Digest) { throw 'Linux payload checksum mismatch.' }
    $path = (& $wsl -d $distro --exec wslpath -u $temp | Out-String).Trim()
    if ($LASTEXITCODE -ne 0) { throw 'Cannot map the downloaded archive into WSL.' }
    # Feed the shell program by file to preserve quoting through Windows PowerShell.
    $install = Join-Path $PSScriptRoot 'install.sh'
    $body = @'
set -eu
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
tar -xzf "$1" -C "$tmp"
"$tmp/$2/setup.sh"
'@
    [IO.File]::WriteAllText($install,($body -replace "`r",""),(New-Object Text.UTF8Encoding($false)))
    $guestScript = (& $wsl -d $distro --exec wslpath -u $install | Out-String).Trim()
    & $wsl -d $distro --exec sh $guestScript $path $name
    if ($LASTEXITCODE -ne 0) { throw 'Linux component setup did not complete.' }
    $menu = [Environment]::GetFolderPath('Programs')
    foreach ($entry in @(@('CamelClip','camelclip-gui'),@('CamelClip Terminal','camelclip-tui'))) {
        & $wsl -d $distro --exec sh -c ('test -x ~/.local/bin/'+$entry[1])
        if ($LASTEXITCODE -eq 0) {
            $link = (New-Object -ComObject WScript.Shell).CreateShortcut((Join-Path $menu ($entry[0]+'.lnk')))
            $link.TargetPath = $wsl
            $link.Arguments = '-d "'+$distro+'" --cd ~ --exec sh -c "exec ~/.local/bin/'+$entry[1]+'"'
            $link.Save()
        }
    }
} finally { Remove-Item $temp -Force -ErrorAction SilentlyContinue }
