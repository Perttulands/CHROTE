# Chrote-Toggle.ps1
# Windows launcher for CHROTE WSL environment
# Place shortcut to this script on desktop

param(
    [switch]$Stop,
    [switch]$Status,
    [switch]$Logs
)

$WSL_DISTRO = "Ubuntu"
$CHROTE_URL = "http://chrote:8080"
$LOCAL_URL = "http://localhost:8080"

function Write-Color($Message, $Color = "White") {
    Write-Host $Message -ForegroundColor $Color
}

function Test-WSLRunning {
    $running = wsl -l --running 2>$null | Select-String $WSL_DISTRO
    return $null -ne $running
}

function Start-Chrote {
    Write-Color "Starting CHROTE..." "Cyan"

    # Start WSL (services auto-start via systemd)
    wsl -d $WSL_DISTRO echo "CHROTE services starting..."

    # Wait for server to be ready
    Write-Color "Waiting for server..." "Yellow"
    $maxAttempts = 30
    $attempt = 0

    while ($attempt -lt $maxAttempts) {
        try {
            $response = Invoke-WebRequest -Uri "$LOCAL_URL/api/health" -TimeoutSec 2 -UseBasicParsing -ErrorAction SilentlyContinue
            if ($response.StatusCode -eq 200) {
                Write-Color "Server ready!" "Green"
                break
            }
        } catch {
            # Server not ready yet
        }
        Start-Sleep -Milliseconds 500
        $attempt++
    }

    if ($attempt -ge $maxAttempts) {
        Write-Color "Warning: Server may not be fully ready" "Yellow"
        Write-Color "Check logs with: .\Chrote-Toggle.ps1 -Logs" "Yellow"
    }

    # Open browser
    Write-Color "Opening browser..." "Cyan"
    Start-Process $CHROTE_URL

    Write-Color "CHROTE is running at $CHROTE_URL" "Green"
}

function Stop-Chrote {
    Write-Color "Stopping CHROTE (shutting down WSL)..." "Yellow"
    wsl --shutdown
    Write-Color "WSL shutdown complete" "Green"
}

function Get-ChroteStatus {
    Write-Color "CHROTE Status" "Cyan"
    Write-Color "=============" "Cyan"

    if (Test-WSLRunning) {
        Write-Color "WSL: Running" "Green"

        # Check services
        Write-Color "`nServices:" "Cyan"
        wsl -d $WSL_DISTRO systemctl status chrote-server --no-pager -l 2>$null | Select-Object -First 5
        wsl -d $WSL_DISTRO systemctl status chrote-ttyd --no-pager -l 2>$null | Select-Object -First 5

        # Check Tailscale
        Write-Color "`nTailscale:" "Cyan"
        wsl -d $WSL_DISTRO tailscale status 2>$null | Select-Object -First 3

        # Test API
        Write-Color "`nAPI Health:" "Cyan"
        try {
            $health = Invoke-RestMethod -Uri "$LOCAL_URL/api/health" -TimeoutSec 2 -ErrorAction SilentlyContinue
            Write-Color "API: OK" "Green"
        } catch {
            Write-Color "API: Not responding" "Red"
        }
    } else {
        Write-Color "WSL: Not running" "Yellow"
        Write-Color "Run .\Chrote-Toggle.ps1 to start" "Yellow"
    }
}

function Get-ChroteLogs {
    Write-Color "CHROTE Logs (Ctrl+C to exit)" "Cyan"
    Write-Color "=============================" "Cyan"
    wsl -d $WSL_DISTRO journalctl -u chrote-server -u chrote-ttyd -f --no-pager
}

# Main logic
if ($Stop) {
    Stop-Chrote
} elseif ($Status) {
    Get-ChroteStatus
} elseif ($Logs) {
    Get-ChroteLogs
} else {
    # Default: Toggle behavior
    if (Test-WSLRunning) {
        # Check if API is responding
        try {
            $health = Invoke-RestMethod -Uri "$LOCAL_URL/api/health" -TimeoutSec 2 -ErrorAction SilentlyContinue
            # Already running, just open browser
            Write-Color "CHROTE already running, opening browser..." "Green"
            Start-Process $CHROTE_URL
        } catch {
            # WSL running but services may not be
            Write-Color "WSL running but services not responding, restarting..." "Yellow"
            wsl -d $WSL_DISTRO sudo systemctl restart chrote-server chrote-ttyd
            Start-Sleep -Seconds 2
            Start-Process $CHROTE_URL
        }
    } else {
        Start-Chrote
    }
}
