# Installs pdx Designer for the current user, without administrator rights.
#
#   Program:    %LOCALAPPDATA%\Programs\pdx\pdx-designer.exe
#   Start menu: pdx Designer (current user)
#
# Settings and autosaves are kept in %APPDATA%\pdx-designer and are not
# touched by install or uninstall.
#
#   powershell -ExecutionPolicy Bypass -File install.ps1             install or update
#   powershell -ExecutionPolicy Bypass -File install.ps1 -Uninstall  remove
param([switch]$Uninstall)
$ErrorActionPreference = "Stop"

$dir = Join-Path $env:LOCALAPPDATA "Programs\pdx"
$exe = Join-Path $dir "pdx-designer.exe"
$lnk = Join-Path ([Environment]::GetFolderPath("Programs")) "pdx Designer.lnk"

if ($Uninstall) {
    if (Test-Path $lnk) { Remove-Item $lnk -Force }
    foreach ($f in @("pdx-designer.exe", "LICENSE", "NOTICE", "THIRD_PARTY_NOTICES.txt", "README.md")) {
        $p = Join-Path $dir $f
        if (Test-Path $p) { Remove-Item $p -Force }
    }
    if ((Test-Path $dir) -and -not (Get-ChildItem $dir)) { Remove-Item $dir -Force }
    Write-Output "removed $exe"
    exit 0
}

$src = Join-Path $PSScriptRoot "pdx-designer.exe"
if (-not (Test-Path $src)) { throw "pdx-designer.exe not found next to install.ps1" }
New-Item -ItemType Directory -Force -Path $dir | Out-Null
Copy-Item $src $exe -Force
foreach ($f in @("LICENSE", "NOTICE", "THIRD_PARTY_NOTICES.txt", "README.md")) {
    $p = Join-Path $PSScriptRoot $f
    if (Test-Path $p) { Copy-Item $p (Join-Path $dir $f) -Force }
}
$shell = New-Object -ComObject WScript.Shell
$shortcut = $shell.CreateShortcut($lnk)
$shortcut.TargetPath = $exe
$shortcut.WorkingDirectory = $dir
$shortcut.Description = "pdx Designer"
$shortcut.Save()
Write-Output "installed $exe"
