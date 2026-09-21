package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	nvidiaSMITimeout     = 2 * time.Second
	nvidiaSMICacheTTL    = 3 * time.Second
	nvidiaSMIOutputLimit = 64 * 1024
)

// GPUInfo is the host NVIDIA telemetry reported by nvidia-smi.
type GPUInfo struct {
	Available bool        `json:"available"`
	Error     string      `json:"error,omitempty"`
	Devices   []GPUDevice `json:"devices"`
}

type GPUDevice struct {
	Index              int      `json:"index"`
	UUID               string   `json:"uuid"`
	Name               string   `json:"name"`
	DriverVersion      string   `json:"driver_version"`
	UtilizationPercent *float64 `json:"utilization_percent,omitempty"`
	MemoryUsedBytes    *uint64  `json:"memory_used_bytes,omitempty"`
	MemoryTotalBytes   *uint64  `json:"memory_total_bytes,omitempty"`
	TemperatureC       *float64 `json:"temperature_c,omitempty"`
	PowerWatts         *float64 `json:"power_watts,omitempty"`
}

type gpuCache struct {
	mu      sync.Mutex
	updated time.Time
	value   GPUInfo
}

func (c *gpuCache) read(ctx context.Context) GPUInfo {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	if !c.updated.IsZero() && now.Sub(c.updated) < nvidiaSMICacheTTL {
		return cloneGPUInfo(c.value)
	}

	value := queryNvidiaSMI(ctx)
	c.value = value
	c.updated = time.Now()
	return cloneGPUInfo(value)
}

func queryNvidiaSMI(ctx context.Context) GPUInfo {
	if ctx == nil {
		ctx = context.Background()
	}
	path, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return unavailableGPU("NVIDIA telemetry unavailable: nvidia-smi was not found")
	}

	queryCtx, cancel := context.WithTimeout(ctx, nvidiaSMITimeout)
	defer cancel()
	cmd := exec.CommandContext(queryCtx, path,
		"--query-gpu=index,uuid,name,driver_version,utilization.gpu,memory.used,memory.total,temperature.gpu,power.draw",
		"--format=csv,noheader,nounits",
	)
	hideCommandWindow(cmd)
	stdout := &limitedBuffer{limit: nvidiaSMIOutputLimit}
	stderr := &limitedBuffer{limit: nvidiaSMIOutputLimit}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return unavailableGPU(nvidiaSMIError(queryCtx, err, stderr.String(), stdout.truncated || stderr.truncated))
	}
	if stdout.truncated || stderr.truncated {
		return unavailableGPU("NVIDIA telemetry unavailable: nvidia-smi output exceeded the safety limit")
	}

	devices, parseErr := parseNvidiaSMIOutput(stdout.String())
	if len(devices) == 0 {
		if parseErr != nil {
			return unavailableGPU("NVIDIA telemetry unavailable: " + parseErr.Error())
		}
		return unavailableGPU("NVIDIA telemetry unavailable: nvidia-smi returned no GPU data")
	}
	info := GPUInfo{Available: true, Devices: devices}
	if parseErr != nil {
		info.Error = "NVIDIA telemetry partially unavailable: " + parseErr.Error()
	}
	return info
}

func parseNvidiaSMIOutput(output string) ([]GPUDevice, error) {
	reader := csv.NewReader(strings.NewReader(output))
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	reader.LazyQuotes = true

	devices := make([]GPUDevice, 0)
	invalidRows := 0
	for {
		fields, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return devices, fmt.Errorf("invalid nvidia-smi output: %w", err)
		}
		if len(fields) == 0 || allCSVFieldsEmpty(fields) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(fields[0]), "index") {
			continue
		}
		if len(fields) < 9 {
			invalidRows++
			continue
		}
		index, err := strconv.Atoi(strings.TrimSpace(fields[0]))
		if err != nil || index < 0 {
			invalidRows++
			continue
		}
		devices = append(devices, GPUDevice{
			Index:              index,
			UUID:               requiredGPUString(fields[1]),
			Name:               requiredGPUString(fields[2]),
			DriverVersion:      requiredGPUString(fields[3]),
			UtilizationPercent: optionalFloat(fields[4]),
			MemoryUsedBytes:    optionalMiBBytes(fields[5]),
			MemoryTotalBytes:   optionalMiBBytes(fields[6]),
			TemperatureC:       optionalFloat(fields[7]),
			PowerWatts:         optionalFloat(fields[8]),
		})
	}
	if invalidRows > 0 {
		return devices, fmt.Errorf("ignored %d malformed GPU row(s)", invalidRows)
	}
	if len(devices) == 0 {
		return devices, errors.New("nvidia-smi returned no GPU data")
	}
	return devices, nil
}

func allCSVFieldsEmpty(fields []string) bool {
	for _, field := range fields {
		if strings.TrimSpace(field) != "" {
			return false
		}
	}
	return true
}

func requiredGPUString(value string) string {
	value = strings.TrimSpace(value)
	if unavailableValue(value) {
		return ""
	}
	return value
}

func optionalFloat(value string) *float64 {
	value = strings.TrimSpace(value)
	if unavailableValue(value) {
		return nil
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return nil
	}
	return &n
}

func optionalMiBBytes(value string) *uint64 {
	value = strings.TrimSpace(value)
	if unavailableValue(value) {
		return nil
	}
	mib, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(mib) || math.IsInf(mib, 0) || mib < 0 {
		return nil
	}
	bytesValue := mib * 1024 * 1024
	if bytesValue > float64(^uint64(0)) {
		return nil
	}
	converted := uint64(math.Round(bytesValue))
	return &converted
}

func unavailableValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "n/a", "na", "not supported", "[not supported]", "-":
		return true
	default:
		return false
	}
}

func unavailableGPU(message string) GPUInfo {
	return GPUInfo{Error: message, Devices: []GPUDevice{}}
}

func cloneGPUInfo(info GPUInfo) GPUInfo {
	devices := make([]GPUDevice, len(info.Devices))
	for i, device := range info.Devices {
		devices[i] = device
		if device.UtilizationPercent != nil {
			value := *device.UtilizationPercent
			devices[i].UtilizationPercent = &value
		}
		if device.MemoryUsedBytes != nil {
			value := *device.MemoryUsedBytes
			devices[i].MemoryUsedBytes = &value
		}
		if device.MemoryTotalBytes != nil {
			value := *device.MemoryTotalBytes
			devices[i].MemoryTotalBytes = &value
		}
		if device.TemperatureC != nil {
			value := *device.TemperatureC
			devices[i].TemperatureC = &value
		}
		if device.PowerWatts != nil {
			value := *device.PowerWatts
			devices[i].PowerWatts = &value
		}
	}
	info.Devices = devices
	return info
}

func nvidiaSMIError(ctx context.Context, err error, stderr string, outputLimited bool) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "NVIDIA telemetry unavailable: nvidia-smi timed out after 2 seconds"
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return "NVIDIA telemetry unavailable: nvidia-smi query canceled"
	}
	if outputLimited {
		return "NVIDIA telemetry unavailable: nvidia-smi output exceeded the safety limit"
	}
	if detail := strings.TrimSpace(stderr); detail != "" {
		return "NVIDIA telemetry unavailable: nvidia-smi failed: " + firstErrorLine(detail)
	}
	return "NVIDIA telemetry unavailable: nvidia-smi failed: " + err.Error()
}

func firstErrorLine(value string) string {
	if index := strings.IndexByte(value, '\n'); index >= 0 {
		value = value[:index]
	}
	return strings.TrimSpace(value)
}

type limitedBuffer struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.limit <= b.Len() {
		b.truncated = true
		return 0, errors.New("output limit exceeded")
	}
	remaining := b.limit - b.Len()
	if len(p) > remaining {
		_, _ = b.Buffer.Write(p[:remaining])
		b.truncated = true
		return remaining, errors.New("output limit exceeded")
	}
	return b.Buffer.Write(p)
}
