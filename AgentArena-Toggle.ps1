# AgentArena Control Script
# Interactive menu for Docker operations

$ErrorActionPreference = "Continue"
$ArenaPath = "e:\Docker\AgentArena"
$ContainerName = "agentarena-dev"

# Set console appearance
$Host.UI.RawUI.WindowTitle = "AgentArena Control"
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

function Write-Status {
    param([string]$Message, [string]$Color = "White")
    Write-Host $Message -ForegroundColor $Color
}

function Test-DockerRunning {
    try {
        $null = docker info 2>&1
        return $LASTEXITCODE -eq 0
    } catch {
        return $false
    }
}

function Test-ArenaRunning {
    try {
        $status = docker inspect -f '{{.State.Running}}' $ContainerName 2>&1
        return $status -eq "true"
    } catch {
        return $false
    }
}

function Ensure-Docker {
    if (-not (Test-DockerRunning)) {
        Write-Status "  Docker not running. Starting Docker Desktop..." "Yellow"
        Start-Process "C:\Program Files\Docker\Docker\Docker Desktop.exe"

        $timeout = 60
        $elapsed = 0
        while (-not (Test-DockerRunning) -and $elapsed -lt $timeout) {
            Start-Sleep -Seconds 2
            $elapsed += 2
            Write-Host "." -NoNewline -ForegroundColor Gray
        }
        Write-Host ""

        if (-not (Test-DockerRunning)) {
            Write-Status "  ERROR: Docker failed to start!" "Red"
            return $false
        }
    }
    return $true
}

function Start-Arena {
    Write-Status "`n  Starting AgentArena..." "Cyan"
    Write-Status "  ======================" "Cyan"

    if (-not (Ensure-Docker)) { return $false }
    Write-Status "  Docker is ready" "Green"

    Set-Location $ArenaPath
    docker compose up -d 2>&1 | ForEach-Object { Write-Status "  $_" "Gray" }

    if ($LASTEXITCODE -ne 0) {
        Write-Status "  ERROR: Failed to start containers!" "Red"
        return $false
    }

    # Wait for services
    Write-Status "`n  Waiting for services..." "Yellow"
    $timeout = 30
    $elapsed = 0
    while ($elapsed -lt $timeout) {
        try {
            $response = Invoke-WebRequest -Uri "http://localhost:8080" -TimeoutSec 2 -UseBasicParsing -ErrorAction SilentlyContinue
            if ($response.StatusCode -eq 200) {
                Write-Status "  Services are ready!" "Green"
                return $true
            }
        } catch { }
        Start-Sleep -Seconds 2
        $elapsed += 2
        Write-Host "." -NoNewline -ForegroundColor Gray
    }
    Write-Host ""
    Write-Status "  Services started (may still be initializing)" "Yellow"
    return $true
}

function Stop-Arena {
    Write-Status "`n  Stopping AgentArena..." "Cyan"

    Set-Location $ArenaPath
    docker compose down 2>&1 | ForEach-Object { Write-Status "  $_" "Gray" }

    if ($LASTEXITCODE -eq 0) {
        Write-Status "  AgentArena stopped" "Green"
        return $true
    } else {
        Write-Status "  ERROR: Failed to stop containers!" "Red"
        return $false
    }
}

function Restart-Arena {
    Write-Status "`n  Restarting AgentArena..." "Cyan"

    Set-Location $ArenaPath
    docker compose restart 2>&1 | ForEach-Object { Write-Status "  $_" "Gray" }

    if ($LASTEXITCODE -eq 0) {
        Write-Status "  AgentArena restarted" "Green"
    } else {
        Write-Status "  ERROR: Restart failed!" "Red"
    }
}

function Rebuild-Arena {
    param([switch]$NoCache)

    $cacheMsg = if ($NoCache) { " (no cache)" } else { "" }
    Write-Status "`n  Rebuilding agent-arena$cacheMsg..." "Cyan"

    if (-not (Ensure-Docker)) { return }

    Set-Location $ArenaPath

    # Build dashboard
    Write-Status "  Building dashboard..." "Yellow"
    Push-Location "$ArenaPath\dashboard"
    & npm run build
    Pop-Location

    # Rebuild container
    Write-Status "  Building container..." "Yellow"
    if ($NoCache) {
        & docker compose build --no-cache agent-arena
    } else {
        & docker compose build agent-arena
    }

    # Recreate container
    Write-Status "  Recreating container..." "Yellow"
    & docker compose up -d --force-recreate agent-arena

    Write-Status "  Done." "Green"
}

function Show-Logs {
    param([string]$Service = "agent-arena", [switch]$Follow)

    Write-Status "`n  Showing logs for $Service..." "Cyan"
    Write-Status "  (Press Ctrl+C to exit)`n" "Gray"

    Set-Location $ArenaPath
    if ($Follow) {
        docker compose logs -f --tail 100 $Service
    } else {
        docker compose logs --tail 50 $Service 2>&1 | ForEach-Object { Write-Host $_ }
    }
}

function Enter-Shell {
    param(
        [string]$Service = "agent-arena",
        [switch]$AsRoot
    )

    $userLabel = if ($AsRoot) { "root" } else { "dev" }
    Write-Status "`n  Opening shell in $Service as $userLabel..." "Cyan"
    Write-Status "  (Type 'exit' to return)`n" "Gray"

    Set-Location $ArenaPath
    if ($AsRoot) {
        docker compose exec -u root $Service bash
    } else {
        docker compose exec -u dev $Service bash
    }
}

function Show-Status {
    Write-Status "`n  Container Status" "Cyan"
    Write-Status "  ================" "Cyan"

    Set-Location $ArenaPath
    docker compose ps --format "table {{.Name}}\t{{.Status}}\t{{.Ports}}" 2>&1 | ForEach-Object { Write-Status "  $_" "White" }
}

function Prune-Docker {
    Write-Status "`n  Cleaning up Docker resources..." "Cyan"

    $confirm = Read-Host "  This removes unused images, containers, volumes. Continue? (y/N)"
    if ($confirm -ne "y") {
        Write-Status "  Cancelled." "Yellow"
        return
    }

    docker system prune -f 2>&1 | ForEach-Object { Write-Status "  $_" "Gray" }
    Write-Status "  Cleanup complete" "Green"
}

function Pull-Images {
    Write-Status "`n  Pulling latest images..." "Cyan"

    Set-Location $ArenaPath
    docker compose pull 2>&1 | ForEach-Object { Write-Status "  $_" "Gray" }

    if ($LASTEXITCODE -eq 0) {
        Write-Status "  Images updated" "Green"
    }
}

function Show-Menu {
    $running = Test-ArenaRunning
    $statusText = if ($running) { "RUNNING" } else { "STOPPED" }
    $statusColor = if ($running) { "Green" } else { "Red" }

    Clear-Host
    Write-Status ""
    Write-Status "  ================================" "Magenta"
    Write-Status "       AGENT ARENA CONTROL        " "Magenta"
    Write-Status "  ================================" "Magenta"
    Write-Status "  Status: $statusText" $statusColor
    Write-Status ""
    Write-Status "  --- Quick Actions ---" "Cyan"
    if ($running) {
        Write-Status "  1. Stop"
        Write-Status "  2. Restart"
    } else {
        Write-Status "  1. Start"
    }
    Write-Status ""
    Write-Status "  --- Build ---" "Cyan"
    Write-Status "  3. Rebuild            - uses cached layers, fast"
    Write-Status "  4. Rebuild (no cache) - full rebuild, use after dependency changes"
    Write-Status ""
    Write-Status "  --- Diagnostics ---" "Cyan"
    Write-Status "  5. View logs"
    Write-Status "  6. Follow logs (live)"
    Write-Status "  7. Container status"
    Write-Status "  8. Shell into container (dev)"
    Write-Status "  9. Shell into container (root)"
    Write-Status ""
    Write-Status "  --- Maintenance ---" "Cyan"
    Write-Status "  10. Pull latest images"
    Write-Status "  0. Docker cleanup (prune)"
    Write-Status ""
    Write-Status "  q. Quit" "Gray"
    Write-Status ""
}

# Main loop
while ($true) {
    Show-Menu
    $choice = Read-Host "  Select option"

    $running = Test-ArenaRunning

    switch ($choice) {
        "1" {
            if ($running) { Stop-Arena } else { Start-Arena }
        }
        "2" {
            if ($running) { Restart-Arena } else { Write-Status "`n  Arena is not running" "Yellow" }
        }
        "3" { Rebuild-Arena }
        "4" { Rebuild-Arena -NoCache }
        "5" { Show-Logs }
        "6" { Show-Logs -Follow }
        "7" { Show-Status }
        "8" {
            if ($running) { Enter-Shell } else { Write-Status "`n  Arena is not running" "Yellow" }
        }
        "9" {
            if ($running) { Enter-Shell -AsRoot } else { Write-Status "`n  Arena is not running" "Yellow" }
        }
        "10" { Pull-Images }
        "0" { Prune-Docker }
        "q" {
            Write-Status "`n  Goodbye!" "Cyan"
            exit
        }
        default { Write-Status "`n  Invalid option" "Yellow" }
    }

    if ($choice -ne "6" -and $choice -ne "8" -and $choice -ne "9") {
        Write-Status "`n  Press any key to continue..." "Gray"
        $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
    }
}
