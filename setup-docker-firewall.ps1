# Docker Firewall Setup Script
# Run this script as Administrator to allow Docker ports through Windows Firewall

Write-Host "=== Docker Port Firewall Configuration ===" -ForegroundColor Cyan
Write-Host ""

# Check for admin privileges
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)

if (-not $isAdmin) {
    Write-Host "ERROR: This script must be run as Administrator!" -ForegroundColor Red
    Write-Host "Right-click PowerShell and select 'Run as Administrator'" -ForegroundColor Yellow
    Write-Host ""
    Read-Host "Press Enter to exit"
    exit 1
}

Write-Host "✓ Running with Administrator privileges" -ForegroundColor Green
Write-Host ""

# Define all Docker ports with descriptions
$ports = @(
    @{Port=2222; Name="Docker SSH Access"; Description="SSH access to container"},
    @{Port=3000; Name="Docker Frontend Dev"; Description="Frontend dev servers"},
    @{Port=5000; Name="Docker Python Apps"; Description="Python applications"},
    @{Port=8000; Name="Docker Node.js Apps"; Description="Node.js applications"},
    @{Port=8080; Name="Docker Web Apps"; Description="General web apps"},
    @{Port=5500; Name="Docker Static Servers"; Description="Static file servers"},
    @{Port=6000; Name="Docker Custom Services"; Description="Custom Node.js services"},
    @{Port=9000; Name="Docker Registry"; Description="Docker registry"},
    @{Port=9090; Name="Docker Prometheus"; Description="Prometheus monitoring"},
    @{Port=9100; Name="Docker Node Exporter"; Description="Node exporter"},
    @{Port=9200; Name="Docker Elasticsearch"; Description="Elasticsearch"},
    @{Port=9300; Name="Docker Elasticsearch Transport"; Description="Elasticsearch transport"},
    @{Port=11434; Name="Docker Ollama API"; Description="Ollama LLM API"},
    @{Port=7000; Name="Docker AI Services 1"; Description="AI/ML services"},
    @{Port=8001; Name="Docker AI Services 2"; Description="AI/ML services"},
    @{Port=8002; Name="Docker AI Services 3"; Description="AI/ML services"},
    @{Port=9400; Name="Docker Reserved 1"; Description="Reserved for future use"},
    @{Port=9500; Name="Docker Reserved 2"; Description="Reserved for future use"},
    @{Port=9600; Name="Docker Reserved 3"; Description="Reserved for future use"},
    @{Port=9700; Name="Docker Reserved 4"; Description="Reserved for future use"},
    @{Port=9800; Name="Docker Reserved 5"; Description="Reserved for future use"},
    @{Port=9900; Name="Docker Reserved 6"; Description="Reserved for future use"}
)

Write-Host "Creating firewall rules for $($ports.Count) Docker ports..." -ForegroundColor Yellow
Write-Host ""

$successCount = 0
$failCount = 0

foreach ($item in $ports) {
    try {
        # Check if rule already exists
        $existingRule = Get-NetFirewallRule -DisplayName $item.Name -ErrorAction SilentlyContinue
        
        if ($existingRule) {
            Write-Host "  Port $($item.Port) - Rule already exists, skipping" -ForegroundColor Yellow
            $successCount++
        } else {
            # Create the rule
            New-NetFirewallRule -DisplayName $item.Name `
                -Description $item.Description `
                -Direction Inbound `
                -Protocol TCP `
                -LocalPort $item.Port `
                -Action Allow `
                -Profile Any `
                -Enabled True | Out-Null
            
            Write-Host "✓ Port $($item.Port) - $($item.Name)" -ForegroundColor Green
            $successCount++
        }
    }
    catch {
        Write-Host "✗ Port $($item.Port) - Failed: $($_.Exception.Message)" -ForegroundColor Red
        $failCount++
    }
}

Write-Host ""
Write-Host "=== Summary ===" -ForegroundColor Cyan
Write-Host "Success: $successCount rules" -ForegroundColor Green
Write-Host "Failed: $failCount rules" -ForegroundColor $(if($failCount -gt 0){"Red"}else{"Green"})
Write-Host ""

# Verify the rules
Write-Host "Verifying created rules..." -ForegroundColor Yellow
$dockerRules = Get-NetFirewallRule | Where-Object { $_.DisplayName -like "Docker*" -and $_.DisplayName -notlike "*Backend*" }
Write-Host "Found $($dockerRules.Count) Docker port rules in firewall" -ForegroundColor Cyan
Write-Host ""

# Show currently listening ports
Write-Host "Currently active Docker ports:" -ForegroundColor Yellow
$listening = netstat -an | Select-String "LISTENING"
foreach ($item in $ports) {
    if ($listening -match ":$($item.Port)\s") {
        Write-Host "  Port $($item.Port) : ACTIVE" -ForegroundColor Green
    }
}

Write-Host ""
Write-Host "Firewall configuration complete!" -ForegroundColor Green
Write-Host "You can now access your Docker services from localhost on these ports." -ForegroundColor Cyan
Write-Host ""
Read-Host "Press Enter to exit"
