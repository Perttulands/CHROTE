package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeSystemCollector struct {
	status SystemStatusResponse
	err    error
}

func (f fakeSystemCollector) Snapshot(context.Context) (SystemStatusResponse, error) {
	return f.status, f.err
}

func TestSystemHandlerStatusUsesSuccessEnvelope(t *testing.T) {
	handler := NewSystemHandlerWithCollector(fakeSystemCollector{
		status: SystemStatusResponse{
			Host: SystemHostStatus{
				Hostname:      "landmass",
				UptimeSeconds: 123.5,
				Load1:         0.25,
				Load5:         0.5,
				Load15:        0.75,
			},
			CPU: SystemCPUStatus{
				Cores:      8,
				TotalTicks: 1000,
				IdleTicks:  250,
			},
			Memory: SystemMemoryStatus{
				TotalBytes:     16,
				AvailableBytes: 4,
				UsedBytes:      12,
				UsedPercent:    75,
			},
			Disks: []SystemDiskStatus{{
				Mount:       "/",
				TotalBytes:  100,
				UsedBytes:   60,
				UsedPercent: 60,
			}},
			Network: []SystemNetworkStatus{{
				Name:    "eth0",
				RxBytes: 10,
				TxBytes: 20,
			}},
			GPUs: []SystemGPUStatus{{
				Available: true,
				Name:      "fixture gpu",
			}},
		},
	})

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/system/status", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var response struct {
		Success bool                 `json:"success"`
		Data    SystemStatusResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if !response.Success {
		t.Fatal("system status should use the success envelope")
	}
	if response.Data.Host.Hostname != "landmass" {
		t.Fatalf("hostname = %q, want fixture host", response.Data.Host.Hostname)
	}
	if response.Data.CPU.TotalTicks == 0 || response.Data.CPU.IdleTicks == 0 {
		t.Fatal("CPU tick counters should be present so the browser can compute deltas")
	}
}

func TestSystemHandlerStatusReportsCollectorFailure(t *testing.T) {
	handler := NewSystemHandlerWithCollector(fakeSystemCollector{err: errors.New("fixture failure")})

	req := httptest.NewRequest(http.MethodGet, "/api/system/status", nil)
	rec := httptest.NewRecorder()
	handler.Status(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "SYSTEM_STATUS_ERROR") {
		t.Fatalf("response should include stable error code, got %s", rec.Body.String())
	}
}

func TestParseCPUStatTotalsFirstAggregateLine(t *testing.T) {
	cpu, err := parseCPUStat("cpu  10 20 30 40 5 6 7 8 9 10\ncpu0 1 2 3 4\n", 12)
	if err != nil {
		t.Fatalf("parse CPU stat: %v", err)
	}
	if cpu.Cores != 12 {
		t.Fatalf("cores = %d, want injected runtime core count", cpu.Cores)
	}
	if cpu.TotalTicks != 145 {
		t.Fatalf("total ticks = %d, want sum of aggregate CPU fields", cpu.TotalTicks)
	}
	if cpu.IdleTicks != 45 {
		t.Fatalf("idle ticks = %d, want idle+iowait", cpu.IdleTicks)
	}
}

func TestParseMemInfoUsesAvailableMemoryForOperationalUsage(t *testing.T) {
	memory, err := parseMemInfo(`MemTotal:       16000 kB
MemFree:         1000 kB
MemAvailable:    4000 kB
Buffers:          500 kB
Cached:          2000 kB
SwapTotal:       8000 kB
SwapFree:        6000 kB
`)
	if err != nil {
		t.Fatalf("parse meminfo: %v", err)
	}
	if memory.TotalBytes != 16000*1024 {
		t.Fatalf("total bytes = %d, want kB converted to bytes", memory.TotalBytes)
	}
	if memory.AvailableBytes != 4000*1024 {
		t.Fatalf("available bytes = %d, want MemAvailable", memory.AvailableBytes)
	}
	if memory.UsedPercent != 75 {
		t.Fatalf("used percent = %.2f, want 75", memory.UsedPercent)
	}
	if memory.SwapUsedPercent != 25 {
		t.Fatalf("swap used percent = %.2f, want 25", memory.SwapUsedPercent)
	}
}

func TestParseNetworkDevicesSkipsLoopback(t *testing.T) {
	network, err := parseNetworkDevices(`Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 100 1 0 0 0 0 0 0 200 2 0 0 0 0 0 0
  eth0: 12345 3 0 0 0 0 0 0 67890 4 0 0 0 0 0 0
`)
	if err != nil {
		t.Fatalf("parse network devices: %v", err)
	}
	if len(network) != 1 {
		t.Fatalf("network interfaces = %d, want only non-loopback interface", len(network))
	}
	if network[0].Name != "eth0" || network[0].RxBytes != 12345 || network[0].TxBytes != 67890 {
		t.Fatalf("network[0] = %+v, want parsed eth0 byte counters", network[0])
	}
}

func TestParseNvidiaSMIOutputConvertsMemoryToBytes(t *testing.T) {
	gpus, err := parseNvidiaSMIOutput("RTX 4090, 42, 24576, 1024, 51, 120.5\n")
	if err != nil {
		t.Fatalf("parse nvidia-smi: %v", err)
	}
	if len(gpus) != 1 {
		t.Fatalf("gpus = %d, want 1", len(gpus))
	}
	if !gpus[0].Available {
		t.Fatal("parsed GPU should be available")
	}
	if gpus[0].MemoryTotalBytes != 24576*1024*1024 {
		t.Fatalf("memory total bytes = %d, want MiB converted to bytes", gpus[0].MemoryTotalBytes)
	}
}
