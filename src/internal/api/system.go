package api

import (
	"bufio"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chrote/server/internal/core"
)

var defaultSystemDiskMounts = []string{"/", "/home", "/srv"}

const (
	defaultSystemHistoryLimit          = 288
	defaultSystemHistorySampleInterval = 5 * time.Minute
)

// SystemHandler handles lightweight local host status endpoints.
type SystemHandler struct {
	collector   systemStatusCollector
	history     *systemHistoryStore
	samplerMu   sync.Mutex
	samplerSeq  uint64
	stopSampler context.CancelFunc
}

type systemStatusCollector interface {
	Snapshot(context.Context) (SystemStatusResponse, error)
}

// SystemStatusResponse is safe to return to browser clients.
type SystemStatusResponse struct {
	Timestamp string                `json:"timestamp"`
	Host      SystemHostStatus      `json:"host"`
	CPU       SystemCPUStatus       `json:"cpu"`
	Memory    SystemMemoryStatus    `json:"memory"`
	Disks     []SystemDiskStatus    `json:"disks"`
	Network   []SystemNetworkStatus `json:"network"`
	GPUs      []SystemGPUStatus     `json:"gpus"`
	Warnings  []SystemWarning       `json:"warnings,omitempty"`
}

type SystemHistoryResponse struct {
	Limit   int                    `json:"limit"`
	Samples []SystemStatusResponse `json:"samples"`
}

type SystemHostStatus struct {
	Hostname      string  `json:"hostname"`
	UptimeSeconds float64 `json:"uptimeSeconds"`
	Load1         float64 `json:"load1"`
	Load5         float64 `json:"load5"`
	Load15        float64 `json:"load15"`
}

type SystemCPUStatus struct {
	Cores      int    `json:"cores"`
	TotalTicks uint64 `json:"totalTicks"`
	IdleTicks  uint64 `json:"idleTicks"`
}

type SystemMemoryStatus struct {
	TotalBytes      uint64  `json:"totalBytes"`
	FreeBytes       uint64  `json:"freeBytes"`
	AvailableBytes  uint64  `json:"availableBytes"`
	UsedBytes       uint64  `json:"usedBytes"`
	UsedPercent     float64 `json:"usedPercent"`
	SwapTotalBytes  uint64  `json:"swapTotalBytes"`
	SwapUsedBytes   uint64  `json:"swapUsedBytes"`
	SwapUsedPercent float64 `json:"swapUsedPercent"`
	SwapCachedBytes uint64  `json:"swapCachedBytes"`
	SwapInPages     uint64  `json:"swapInPages"`
	SwapOutPages    uint64  `json:"swapOutPages"`
	PageSizeBytes   uint64  `json:"pageSizeBytes"`
}

type systemSwapCounters struct {
	InPages  uint64
	OutPages uint64
}

type SystemDiskStatus struct {
	Mount          string  `json:"mount"`
	TotalBytes     uint64  `json:"totalBytes"`
	AvailableBytes uint64  `json:"availableBytes"`
	UsedBytes      uint64  `json:"usedBytes"`
	UsedPercent    float64 `json:"usedPercent"`
}

type SystemNetworkStatus struct {
	Name    string `json:"name"`
	RxBytes uint64 `json:"rxBytes"`
	TxBytes uint64 `json:"txBytes"`
}

type SystemGPUStatus struct {
	Available          bool     `json:"available"`
	Name               string   `json:"name,omitempty"`
	UtilizationPercent *float64 `json:"utilizationPercent,omitempty"`
	MemoryTotalBytes   uint64   `json:"memoryTotalBytes,omitempty"`
	MemoryUsedBytes    uint64   `json:"memoryUsedBytes,omitempty"`
	TemperatureCelsius *float64 `json:"temperatureCelsius,omitempty"`
	PowerWatts         *float64 `json:"powerWatts,omitempty"`
	Source             string   `json:"source,omitempty"`
	Message            string   `json:"message,omitempty"`
}

type SystemWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// LocalSystemCollector reads lightweight host metrics from local Linux files.
type LocalSystemCollector struct {
	procRoot       string
	diskMounts     []string
	nvidiaSMIPaths []string
	lookPath       func(string) (string, error)
	runNvidiaSMI   func(context.Context, string) ([]byte, error)
}

// NewSystemHandler creates a system status handler.
func NewSystemHandler() *SystemHandler {
	return NewSystemHandlerWithCollector(NewLocalSystemCollector())
}

func NewSystemHandlerWithCollector(collector systemStatusCollector) *SystemHandler {
	return &SystemHandler{
		collector: collector,
		history:   newSystemHistoryStore(defaultSystemHistoryLimit),
	}
}

// NewLocalSystemCollector creates a collector for the current host.
func NewLocalSystemCollector() *LocalSystemCollector {
	return &LocalSystemCollector{
		procRoot:   "/proc",
		diskMounts: defaultSystemDiskMounts,
		nvidiaSMIPaths: []string{
			"nvidia-smi",
			"/usr/lib/wsl/lib/nvidia-smi",
			"/mnt/c/Windows/System32/lxss/lib/nvidia-smi",
		},
		lookPath: exec.LookPath,
		runNvidiaSMI: func(ctx context.Context, path string) ([]byte, error) {
			cmd := exec.CommandContext(
				ctx,
				path,
				"--query-gpu=name,utilization.gpu,memory.total,memory.used,temperature.gpu,power.draw",
				"--format=csv,noheader,nounits",
			)
			return cmd.Output()
		},
	}
}

// RegisterRoutes registers system status routes.
func (h *SystemHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/system/status", h.Status)
	mux.HandleFunc("GET /api/system/history", h.History)
}

// Status handles GET /api/system/status.
func (h *SystemHandler) Status(w http.ResponseWriter, r *http.Request) {
	status, err := h.collector.Snapshot(r.Context())
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "SYSTEM_STATUS_ERROR", err.Error())
		return
	}
	core.WriteSuccess(w, status)
}

// History handles GET /api/system/history.
func (h *SystemHandler) History(w http.ResponseWriter, r *http.Request) {
	if h.history == nil {
		core.WriteSuccess(w, SystemHistoryResponse{})
		return
	}
	core.WriteSuccess(w, SystemHistoryResponse{
		Limit:   h.history.limit,
		Samples: h.history.snapshot(),
	})
}

func (h *SystemHandler) StartDefaultHistorySampler(ctx context.Context) context.CancelFunc {
	return h.StartHistorySampler(ctx, defaultSystemHistorySampleInterval)
}

// StartHistorySampler records bounded server-side telemetry without requiring a browser poller.
func (h *SystemHandler) StartHistorySampler(ctx context.Context, interval time.Duration) context.CancelFunc {
	if interval <= 0 {
		return func() {}
	}
	samplerCtx, cancel := context.WithCancel(ctx)

	h.samplerMu.Lock()
	if h.stopSampler != nil {
		h.stopSampler()
	}
	h.samplerSeq++
	samplerID := h.samplerSeq
	h.stopSampler = cancel
	h.samplerMu.Unlock()

	go func() {
		defer func() {
			h.samplerMu.Lock()
			if h.samplerSeq == samplerID {
				h.stopSampler = nil
			}
			h.samplerMu.Unlock()
		}()

		select {
		case <-samplerCtx.Done():
			return
		default:
		}
		h.recordHistorySnapshot(samplerCtx)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-samplerCtx.Done():
				return
			case <-ticker.C:
				h.recordHistorySnapshot(samplerCtx)
			}
		}
	}()
	return cancel
}

func (h *SystemHandler) StopHistorySampler() {
	h.samplerMu.Lock()
	defer h.samplerMu.Unlock()
	if h.stopSampler != nil {
		h.stopSampler()
		h.stopSampler = nil
	}
}

func (h *SystemHandler) recordHistorySnapshot(ctx context.Context) {
	status, err := h.collector.Snapshot(ctx)
	if err != nil {
		return
	}
	h.history.add(status)
}

type systemHistoryStore struct {
	mu      sync.RWMutex
	limit   int
	samples []SystemStatusResponse
	next    int
}

func newSystemHistoryStore(limit int) *systemHistoryStore {
	if limit < 1 {
		limit = 1
	}
	return &systemHistoryStore{
		limit:   limit,
		samples: make([]SystemStatusResponse, 0, limit),
	}
}

func (s *systemHistoryStore) add(sample SystemStatusResponse) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.samples) < s.limit {
		s.samples = append(s.samples, sample)
		return
	}
	s.samples[s.next] = sample
	s.next = (s.next + 1) % s.limit
}

func (s *systemHistoryStore) snapshot() []SystemStatusResponse {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.samples) < s.limit {
		out := make([]SystemStatusResponse, len(s.samples))
		copy(out, s.samples)
		return out
	}
	out := make([]SystemStatusResponse, 0, len(s.samples))
	out = append(out, s.samples[s.next:]...)
	out = append(out, s.samples[:s.next]...)
	return out
}

// Snapshot returns a current local host status snapshot.
func (c *LocalSystemCollector) Snapshot(ctx context.Context) (SystemStatusResponse, error) {
	var warnings []SystemWarning

	cpuRaw, err := os.ReadFile(filepath.Join(c.procRoot, "stat"))
	if err != nil {
		return SystemStatusResponse{}, fmt.Errorf("read cpu stat: %w", err)
	}
	cpu, err := parseCPUStat(string(cpuRaw), runtime.NumCPU())
	if err != nil {
		return SystemStatusResponse{}, err
	}

	memRaw, err := os.ReadFile(filepath.Join(c.procRoot, "meminfo"))
	if err != nil {
		return SystemStatusResponse{}, fmt.Errorf("read meminfo: %w", err)
	}
	memory, err := parseMemInfo(string(memRaw))
	if err != nil {
		return SystemStatusResponse{}, err
	}
	memory.PageSizeBytes = uint64(os.Getpagesize())
	vmstatRaw, err := os.ReadFile(filepath.Join(c.procRoot, "vmstat"))
	if err != nil {
		warnings = append(warnings, SystemWarning{Code: "VMSTAT_UNAVAILABLE", Message: err.Error()})
	} else if counters, err := parseVMStatSwapCounters(string(vmstatRaw)); err != nil {
		warnings = append(warnings, SystemWarning{Code: "VMSTAT_PARSE_ERROR", Message: err.Error()})
	} else {
		memory.SwapInPages = counters.InPages
		memory.SwapOutPages = counters.OutPages
	}

	loadRaw, err := os.ReadFile(filepath.Join(c.procRoot, "loadavg"))
	if err != nil {
		return SystemStatusResponse{}, fmt.Errorf("read loadavg: %w", err)
	}
	load1, load5, load15, err := parseLoadAvg(string(loadRaw))
	if err != nil {
		return SystemStatusResponse{}, err
	}

	uptimeRaw, err := os.ReadFile(filepath.Join(c.procRoot, "uptime"))
	if err != nil {
		return SystemStatusResponse{}, fmt.Errorf("read uptime: %w", err)
	}
	uptime, err := parseUptime(string(uptimeRaw))
	if err != nil {
		return SystemStatusResponse{}, err
	}

	networkRaw, err := os.ReadFile(filepath.Join(c.procRoot, "net", "dev"))
	network := []SystemNetworkStatus{}
	if err != nil {
		warnings = append(warnings, SystemWarning{Code: "NETWORK_UNAVAILABLE", Message: err.Error()})
	} else if network, err = parseNetworkDevices(string(networkRaw)); err != nil {
		warnings = append(warnings, SystemWarning{Code: "NETWORK_PARSE_ERROR", Message: err.Error()})
	}

	disks, diskWarnings := c.collectDisks()
	warnings = append(warnings, diskWarnings...)

	hostname, _ := os.Hostname()
	gpus := c.collectGPUs(ctx)

	return SystemStatusResponse{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Host: SystemHostStatus{
			Hostname:      hostname,
			UptimeSeconds: uptime,
			Load1:         load1,
			Load5:         load5,
			Load15:        load15,
		},
		CPU:      cpu,
		Memory:   memory,
		Disks:    disks,
		Network:  network,
		GPUs:     gpus,
		Warnings: warnings,
	}, nil
}

func (c *LocalSystemCollector) collectDisks() ([]SystemDiskStatus, []SystemWarning) {
	var disks []SystemDiskStatus
	var warnings []SystemWarning
	seen := map[string]bool{}

	for _, mount := range c.diskMounts {
		if mount == "" || seen[mount] {
			continue
		}
		seen[mount] = true
		if _, err := os.Stat(mount); err != nil {
			continue
		}

		var stat syscall.Statfs_t
		if err := syscall.Statfs(mount, &stat); err != nil {
			warnings = append(warnings, SystemWarning{
				Code:    "DISK_STAT_ERROR",
				Message: fmt.Sprintf("%s: %v", mount, err),
			})
			continue
		}

		total := stat.Blocks * uint64(stat.Bsize)
		available := stat.Bavail * uint64(stat.Bsize)
		free := stat.Bfree * uint64(stat.Bsize)
		used := total - free
		disks = append(disks, SystemDiskStatus{
			Mount:          mount,
			TotalBytes:     total,
			AvailableBytes: available,
			UsedBytes:      used,
			UsedPercent:    percent(used, total),
		})
	}

	return disks, warnings
}

func (c *LocalSystemCollector) collectGPUs(ctx context.Context) []SystemGPUStatus {
	path, err := c.resolveNvidiaSMIPath()
	if err != nil {
		return []SystemGPUStatus{{
			Available: false,
			Message:   err.Error(),
		}}
	}

	runCtx, cancel := context.WithTimeout(ctx, 1200*time.Millisecond)
	defer cancel()

	out, err := c.runNvidiaSMI(runCtx, path)
	if err != nil {
		return []SystemGPUStatus{{
			Available: false,
			Source:    path,
			Message:   "nvidia-smi failed: " + err.Error(),
		}}
	}

	gpus, err := parseNvidiaSMIOutput(string(out))
	if err != nil {
		return []SystemGPUStatus{{
			Available: false,
			Source:    path,
			Message:   "nvidia-smi output could not be parsed: " + err.Error(),
		}}
	}
	if len(gpus) == 0 {
		return []SystemGPUStatus{{
			Available: false,
			Source:    path,
			Message:   "nvidia-smi returned no GPUs",
		}}
	}
	for i := range gpus {
		gpus[i].Source = path
	}
	return gpus
}

func (c *LocalSystemCollector) resolveNvidiaSMIPath() (string, error) {
	candidates := c.nvidiaSMIPaths
	if len(candidates) == 0 {
		candidates = []string{"nvidia-smi"}
	}
	var misses []string
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if strings.ContainsRune(candidate, os.PathSeparator) {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			} else if err != nil {
				misses = append(misses, candidate)
			}
			continue
		}
		path, err := c.lookPath(candidate)
		if err == nil {
			return path, nil
		}
		misses = append(misses, candidate)
	}
	if len(misses) == 0 {
		return "", errors.New("nvidia-smi not configured")
	}
	return "", fmt.Errorf("nvidia-smi not found; checked %s", strings.Join(misses, ", "))
}

func parseCPUStat(raw string, cores int) (SystemCPUStatus, error) {
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 5 {
			return SystemCPUStatus{}, errors.New("cpu stat aggregate line is incomplete")
		}

		var total uint64
		for _, field := range fields[1:] {
			value, err := strconv.ParseUint(field, 10, 64)
			if err != nil {
				return SystemCPUStatus{}, fmt.Errorf("parse cpu stat field %q: %w", field, err)
			}
			total += value
		}

		idle, err := strconv.ParseUint(fields[4], 10, 64)
		if err != nil {
			return SystemCPUStatus{}, fmt.Errorf("parse cpu idle: %w", err)
		}
		if len(fields) > 5 {
			iowait, err := strconv.ParseUint(fields[5], 10, 64)
			if err != nil {
				return SystemCPUStatus{}, fmt.Errorf("parse cpu iowait: %w", err)
			}
			idle += iowait
		}

		return SystemCPUStatus{Cores: cores, TotalTicks: total, IdleTicks: idle}, nil
	}
	if err := scanner.Err(); err != nil {
		return SystemCPUStatus{}, err
	}
	return SystemCPUStatus{}, errors.New("cpu aggregate line not found")
}

func parseMemInfo(raw string) (SystemMemoryStatus, error) {
	values := map[string]uint64{}
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return SystemMemoryStatus{}, fmt.Errorf("parse meminfo %s: %w", key, err)
		}
		values[key] = value * 1024
	}
	if err := scanner.Err(); err != nil {
		return SystemMemoryStatus{}, err
	}

	total := values["MemTotal"]
	if total == 0 {
		return SystemMemoryStatus{}, errors.New("MemTotal missing from meminfo")
	}

	available := values["MemAvailable"]
	if available == 0 {
		available = values["MemFree"] + values["Buffers"] + values["Cached"]
	}
	used := total - available

	swapTotal := values["SwapTotal"]
	swapFree := values["SwapFree"]
	swapCached := values["SwapCached"]
	swapUsed := uint64(0)
	if swapTotal > swapFree {
		swapUsed = swapTotal - swapFree
	}

	return SystemMemoryStatus{
		TotalBytes:      total,
		FreeBytes:       values["MemFree"],
		AvailableBytes:  available,
		UsedBytes:       used,
		UsedPercent:     percent(used, total),
		SwapTotalBytes:  swapTotal,
		SwapUsedBytes:   swapUsed,
		SwapUsedPercent: percent(swapUsed, swapTotal),
		SwapCachedBytes: swapCached,
	}, nil
}

func parseVMStatSwapCounters(raw string) (systemSwapCounters, error) {
	var counters systemSwapCounters
	seenIn := false
	seenOut := false
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		switch fields[0] {
		case "pswpin":
			value, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return systemSwapCounters{}, fmt.Errorf("parse pswpin: %w", err)
			}
			counters.InPages = value
			seenIn = true
		case "pswpout":
			value, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return systemSwapCounters{}, fmt.Errorf("parse pswpout: %w", err)
			}
			counters.OutPages = value
			seenOut = true
		}
	}
	if err := scanner.Err(); err != nil {
		return systemSwapCounters{}, err
	}
	if !seenIn || !seenOut {
		return systemSwapCounters{}, errors.New("vmstat missing pswpin/pswpout")
	}
	return counters, nil
}

func parseLoadAvg(raw string) (float64, float64, float64, error) {
	fields := strings.Fields(raw)
	if len(fields) < 3 {
		return 0, 0, 0, errors.New("loadavg is incomplete")
	}
	load1, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse load1: %w", err)
	}
	load5, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse load5: %w", err)
	}
	load15, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse load15: %w", err)
	}
	return load1, load5, load15, nil
}

func parseUptime(raw string) (float64, error) {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return 0, errors.New("uptime is empty")
	}
	uptime, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("parse uptime: %w", err)
	}
	return uptime, nil
}

func parseNetworkDevices(raw string) ([]SystemNetworkStatus, error) {
	var network []SystemNetworkStatus
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := scanner.Text()
		namePart, dataPart, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name := strings.TrimSpace(namePart)
		if name == "" || name == "lo" {
			continue
		}
		fields := strings.Fields(dataPart)
		if len(fields) < 9 {
			return nil, fmt.Errorf("network row for %s is incomplete", name)
		}
		rx, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse %s rx bytes: %w", name, err)
		}
		tx, err := strconv.ParseUint(fields[8], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse %s tx bytes: %w", name, err)
		}
		network = append(network, SystemNetworkStatus{Name: name, RxBytes: rx, TxBytes: tx})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return network, nil
}

func parseNvidiaSMIOutput(raw string) ([]SystemGPUStatus, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	reader := csv.NewReader(strings.NewReader(raw))
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	gpus := make([]SystemGPUStatus, 0, len(records))
	for _, record := range records {
		if len(record) < 6 {
			return nil, fmt.Errorf("expected 6 columns, got %d", len(record))
		}
		utilization, err := optionalFloat(record[1])
		if err != nil {
			return nil, fmt.Errorf("parse gpu utilization: %w", err)
		}
		memoryTotalMiB, err := optionalFloat(record[2])
		if err != nil {
			return nil, fmt.Errorf("parse gpu memory total: %w", err)
		}
		memoryUsedMiB, err := optionalFloat(record[3])
		if err != nil {
			return nil, fmt.Errorf("parse gpu memory used: %w", err)
		}
		temperature, err := optionalFloat(record[4])
		if err != nil {
			return nil, fmt.Errorf("parse gpu temperature: %w", err)
		}
		power, err := optionalFloat(record[5])
		if err != nil {
			return nil, fmt.Errorf("parse gpu power: %w", err)
		}

		gpu := SystemGPUStatus{
			Available:          true,
			Name:               strings.TrimSpace(record[0]),
			UtilizationPercent: utilization,
			TemperatureCelsius: temperature,
			PowerWatts:         power,
		}
		if memoryTotalMiB != nil {
			gpu.MemoryTotalBytes = uint64(*memoryTotalMiB * 1024 * 1024)
		}
		if memoryUsedMiB != nil {
			gpu.MemoryUsedBytes = uint64(*memoryUsedMiB * 1024 * 1024)
		}
		gpus = append(gpus, gpu)
	}

	return gpus, nil
}

func optionalFloat(raw string) (*float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "N/A") || strings.EqualFold(raw, "[N/A]") {
		return nil, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func percent(part, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) / float64(total) * 100
}
