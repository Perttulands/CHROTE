# Creates a desktop shortcut for AgentArena Toggle
# Run this once to create the shortcut

$WshShell = New-Object -ComObject WScript.Shell
$DesktopPath = [Environment]::GetFolderPath("Desktop")
$ShortcutPath = Join-Path $DesktopPath "AgentArena.lnk"

$Shortcut = $WshShell.CreateShortcut($ShortcutPath)
$Shortcut.TargetPath = "powershell.exe"
$Shortcut.Arguments = "-ExecutionPolicy Bypass -File `"e:\Docker\AgentArena\AgentArena-Toggle.ps1`""
$Shortcut.WorkingDirectory = "e:\Docker\AgentArena"
$Shortcut.Description = "Start or Stop AgentArena"
$Shortcut.IconLocation = "e:\Docker\AgentArena\arena.ico"
$Shortcut.Save()

Write-Host "Desktop shortcut created: $ShortcutPath" -ForegroundColor Green
Write-Host "Double-click 'AgentArena' on your desktop to toggle start/stop" -ForegroundColor Cyan
