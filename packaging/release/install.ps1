$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$targetDir = Join-Path $HOME "AppData\Local\Programs\Nebula"

New-Item -ItemType Directory -Force -Path $targetDir | Out-Null
Move-Item -Force (Join-Path $scriptDir "nb.exe") (Join-Path $targetDir "nb.exe")
Move-Item -Force (Join-Path $scriptDir "nbtui.exe") (Join-Path $targetDir "nbtui.exe")

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
$escapedTarget = [Regex]::Escape($targetDir)

if ($userPath -notmatch "(^|;)$escapedTarget($|;)") {
  $newPath = if ([string]::IsNullOrWhiteSpace($userPath)) { $targetDir } else { "$userPath;$targetDir" }
  [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
  Write-Host "Added $targetDir to PATH"
}

Write-Host "Nebula installed to $targetDir"
Write-Host "Restart your shell if PATH changes do not appear immediately."
