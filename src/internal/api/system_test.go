package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestSystemHandlerHistoryUsesSuccessEnvelopeWithTelemetryShape(t *testing.T) {
	handler := NewSystemHandlerWithCollector(fakeSystemCollector{})
	utilization := 17.0
	handler.history.add(SystemStatusResponse{
		Timestamp: "2026-06-28T01:00:00Z",
		Memory: SystemMemoryStatus{
			TotalBytes:      100,
			UsedBytes:       50,
			UsedPercent:     50,
			SwapTotalBytes:  40,
			SwapUsedBytes:   10,
			SwapUsedPercent: 25,
		},
		Disks: []SystemDiskStatus{{
			Mount:       "/srv",
			TotalBytes:  200,
			UsedBytes:   125,
			UsedPercent: 62.5,
		}},
		GPUs: []SystemGPUStatus{{
			Available:          true,
			Source:             "/usr/lib/wsl/lib/nvidia-smi",
			Name:               "NVIDIA RTX",
			UtilizationPercent: &utilization,
			MemoryTotalBytes:   16376 * 1024 * 1024,
			MemoryUsedBytes:    2291 * 1024 * 1024,
		}},
	})

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/system/history", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var response struct {
		Success bool                  `json:"success"`
		Data    SystemHistoryResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode history response: %v", err)
	}
	if !response.Success {
		t.Fatal("system history should use the success envelope")
	}
	if len(response.Data.Samples) != 1 {
		t.Fatalf("samples = %d, want 1", len(response.Data.Samples))
	}
	sample := response.Data.Samples[0]
	if sample.Timestamp != "2026-06-28T01:00:00Z" {
		t.Fatalf("timestamp = %q, want fixture timestamp", sample.Timestamp)
	}
	if sample.Memory.SwapUsedPercent != 25 {
		t.Fatalf("swap used percent = %.2f, want separate swap usage", sample.Memory.SwapUsedPercent)
	}
	if len(sample.Disks) != 1 || sample.Disks[0].Mount != "/srv" || sample.Disks[0].UsedPercent != 62.5 {
		t.Fatalf("disks = %+v, want storage state fields", sample.Disks)
	}
	if len(sample.GPUs) != 1 {
		t.Fatalf("gpus = %d, want 1", len(sample.GPUs))
	}
	gpu := sample.GPUs[0]
	if gpu.Source != "/usr/lib/wsl/lib/nvidia-smi" || gpu.Name != "NVIDIA RTX" {
		t.Fatalf("gpu source/name = %q/%q, want WSL source and name preserved", gpu.Source, gpu.Name)
	}
	if gpu.UtilizationPercent == nil || *gpu.UtilizationPercent != 17 {
		t.Fatalf("gpu utilization = %v, want 17", gpu.UtilizationPercent)
	}
	if gpu.MemoryTotalBytes == 0 || gpu.MemoryUsedBytes == 0 {
		t.Fatalf("gpu VRAM fields should be present, got %+v", gpu)
	}
}

func TestSystemHistoryStoreBoundsRetention(t *testing.T) {
	store := newSystemHistoryStore(2)
	store.add(SystemStatusResponse{Timestamp: "first"})
	store.add(SystemStatusResponse{Timestamp: "second"})
	store.add(SystemStatusResponse{Timestamp: "third"})

	samples := store.snapshot()
	if len(samples) != 2 {
		t.Fatalf("samples = %d, want bounded history length 2", len(samples))
	}
	if samples[0].Timestamp != "second" || samples[1].Timestamp != "third" {
		t.Fatalf("samples = %+v, want oldest entries dropped and chronological order preserved", samples)
	}
}

func TestSystemHandlerHistorySamplerRecordsBackendSample(t *testing.T) {
	handler := NewSystemHandlerWithCollector(fakeSystemCollector{
		status: SystemStatusResponse{Timestamp: "sampled-by-backend"},
	})
	stop := handler.StartHistorySampler(context.Background(), time.Hour)
	defer stop()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if samples := handler.history.snapshot(); len(samples) == 1 && samples[0].Timestamp == "sampled-by-backend" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("history sampler did not record backend sample, got %+v", handler.history.snapshot())
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
SwapCached:       125 kB
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
	if memory.SwapCachedBytes != 125*1024 {
		t.Fatalf("swap cached bytes = %d, want kB converted to bytes", memory.SwapCachedBytes)
	}
}

func TestParseVMStatSwapCounters(t *testing.T) {
	counters, err := parseVMStatSwapCounters(`pgfault 10
pswpin 123
pswpout 456
pgmajfault 7
`)
	if err != nil {
		t.Fatalf("parse vmstat swap counters: %v", err)
	}
	if counters.InPages != 123 || counters.OutPages != 456 {
		t.Fatalf("swap counters = %+v, want pswpin/pswpout pages", counters)
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

func TestCollectGPUsUsesConfiguredWSLNvidiaSMIWhenPathLookupFails(t *testing.T) {
	fakeWSLNvidiaSMI := filepath.Join(t.TempDir(), "usr", "lib", "wsl", "lib", "nvidia-smi")
	if err := os.MkdirAll(filepath.Dir(fakeWSLNvidiaSMI), 0o755); err != nil {
		t.Fatalf("create fake WSL nvidia-smi dir: %v", err)
	}
	if err := os.WriteFile(fakeWSLNvidiaSMI, []byte("fixture"), 0o755); err != nil {
		t.Fatalf("write fake WSL nvidia-smi: %v", err)
	}

	collector := &LocalSystemCollector{
		lookPath: func(string) (string, error) {
			return "", exec.ErrNotFound
		},
		nvidiaSMIPaths: []string{fakeWSLNvidiaSMI},
		runNvidiaSMI: func(_ context.Context, path string) ([]byte, error) {
			if path != fakeWSLNvidiaSMI {
				t.Fatalf("nvidia-smi path = %q, want WSL fallback", path)
			}
			return []byte("NVIDIA RTX, 17, 16376, 2291, 39, 58.32\n"), nil
		},
	}

	gpus := collector.collectGPUs(context.Background())
	if len(gpus) != 1 || !gpus[0].Available {
		t.Fatalf("gpus = %+v, want available WSL GPU", gpus)
	}
	if gpus[0].Source != fakeWSLNvidiaSMI {
		t.Fatalf("source = %q, want WSL fallback path", gpus[0].Source)
	}
}
