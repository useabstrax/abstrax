// Package serverinfo collects system metrics from /proc and standard tools.
package serverinfo

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"

	executil "abstrax/internal/exec"
)

// Service collects server information.
type Service struct {
	runner *executil.Runner
}

// New creates a Service.
func New(verbose bool) *Service {
	return &Service{runner: executil.New(false, verbose)}
}

// Status returns a comprehensive server status.
func (s *Service) Status(ctx context.Context) (*ServerStatus, error) {
	status := &ServerStatus{}

	if h, err := os.Hostname(); err == nil {
		status.Hostname = h
	}

	status.Uptime = s.uptime(ctx)
	status.LoadAverage = s.loadAverage()
	status.CPU = s.cpuInfo(ctx)
	status.Memory = s.memoryInfo()
	status.Swap = s.swapInfo()
	status.Disks = s.diskInfo(ctx)
	status.OS = s.osInfo()
	status.KernelVersion = s.kernelVersion(ctx)
	status.PrivateIPs = s.privateIPs()

	return status, nil
}

// CPU returns CPU information.
func (s *Service) CPU(ctx context.Context) CPUInfo {
	return s.cpuInfo(ctx)
}

// Memory returns memory information.
func (s *Service) Memory() MemoryInfo {
	return s.memoryInfo()
}

// Disk returns disk information.
func (s *Service) Disk(ctx context.Context, path string) []DiskInfo {
	if path != "" {
		info := s.diskUsage(ctx, path)
		if info != nil {
			return []DiskInfo{*info}
		}
	}
	return s.diskInfo(ctx)
}

// Load returns the load average.
func (s *Service) Load() [3]float64 {
	return s.loadAverage()
}

func (s *Service) uptime(ctx context.Context) string {
	res, err := s.runner.RunSilent(ctx, "uptime", "-p")
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(res.Stdout)
}

func (s *Service) loadAverage() [3]float64 {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return [3]float64{}
	}
	parts := strings.Fields(string(data))
	if len(parts) < 3 {
		return [3]float64{}
	}
	var avg [3]float64
	for i := 0; i < 3; i++ {
		avg[i], _ = strconv.ParseFloat(parts[i], 64)
	}
	return avg
}

func (s *Service) cpuInfo(_ context.Context) CPUInfo {
	return CPUInfo{Cores: runtime.NumCPU()}
}

func (s *Service) memoryInfo() MemoryInfo {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return MemoryInfo{}
	}

	kv := parseMeminfo(data)
	total := kv["MemTotal"]
	free := kv["MemFree"]
	buffers := kv["Buffers"]
	cached := kv["Cached"]
	sreclaimable := kv["SReclaimable"]

	available := free + buffers + cached + sreclaimable
	used := total - available

	info := MemoryInfo{
		TotalMB: kbToMB(total),
		UsedMB:  kbToMB(used),
		FreeMB:  kbToMB(available),
	}
	if total > 0 {
		info.UsagePct = math.Round(float64(used)/float64(total)*100*10) / 10
	}
	return info
}

func (s *Service) swapInfo() SwapInfo {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return SwapInfo{}
	}

	kv := parseMeminfo(data)
	total := kv["SwapTotal"]
	free := kv["SwapFree"]
	used := total - free

	info := SwapInfo{
		TotalMB: kbToMB(total),
		UsedMB:  kbToMB(used),
	}
	if total > 0 {
		info.UsagePct = math.Round(float64(used)/float64(total)*100*10) / 10
	}
	return info
}

func (s *Service) diskInfo(ctx context.Context) []DiskInfo {
	res, err := s.runner.RunSilent(ctx, "df", "-BG", "--output=source,target,size,used,avail,pcent")
	if err != nil {
		return nil
	}

	var disks []DiskInfo
	scanner := bufio.NewScanner(strings.NewReader(res.Stdout))
	first := true
	for scanner.Scan() {
		if first {
			first = false
			continue
		}
		parts := strings.Fields(scanner.Text())
		if len(parts) < 6 {
			continue
		}
		if strings.HasPrefix(parts[0], "tmpfs") || strings.HasPrefix(parts[0], "devtmpfs") {
			continue
		}
		d := DiskInfo{
			Device:     parts[0],
			MountPoint: parts[1],
			TotalGB:    parseGB(parts[2]),
			UsedGB:     parseGB(parts[3]),
			FreeGB:     parseGB(parts[4]),
		}
		pct := strings.TrimSuffix(parts[5], "%")
		d.UsagePct, _ = strconv.ParseFloat(pct, 64)
		disks = append(disks, d)
	}
	return disks
}

func (s *Service) diskUsage(ctx context.Context, path string) *DiskInfo {
	res, err := s.runner.RunSilent(ctx, "df", "-BG", "--output=source,target,size,used,avail,pcent", path)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(res.Stdout), "\n")
	if len(lines) < 2 {
		return nil
	}
	parts := strings.Fields(lines[1])
	if len(parts) < 6 {
		return nil
	}
	d := &DiskInfo{
		Device:     parts[0],
		MountPoint: parts[1],
		TotalGB:    parseGB(parts[2]),
		UsedGB:     parseGB(parts[3]),
		FreeGB:     parseGB(parts[4]),
	}
	pct := strings.TrimSuffix(parts[5], "%")
	d.UsagePct, _ = strconv.ParseFloat(pct, 64)
	return d
}

func (s *Service) osInfo() OSInfo {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return OSInfo{}
	}
	kv := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			kv[parts[0]] = strings.Trim(parts[1], `"`)
		}
	}
	return OSInfo{
		Name:    kv["ID"],
		Version: kv["VERSION_ID"],
		Pretty:  kv["PRETTY_NAME"],
	}
}

func (s *Service) kernelVersion(ctx context.Context) string {
	res, err := s.runner.RunSilent(ctx, "uname", "-r")
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(res.Stdout)
}

func (s *Service) privateIPs() []string {
	var ips []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return ips
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && !ip.IsLoopback() {
				ips = append(ips, ip.String())
			}
		}
	}
	return ips
}

func parseMeminfo(data []byte) map[string]int64 {
	kv := make(map[string]int64)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) < 2 {
			continue
		}
		key := strings.TrimSuffix(parts[0], ":")
		val, _ := strconv.ParseInt(parts[1], 10, 64)
		kv[key] = val
	}
	return kv
}

func kbToMB(kb int64) int64 {
	return kb / 1024
}

func parseGB(s string) float64 {
	s = strings.TrimSuffix(s, "G")
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// Services returns running/failed services.
func (s *Service) Services(ctx context.Context, failed bool) ([]string, error) {
	if executil.SystemctlWorks() {
		return s.servicesSystemctl(ctx, failed)
	}
	return s.servicesLegacy(ctx, failed)
}

func (s *Service) servicesSystemctl(ctx context.Context, failed bool) ([]string, error) {
	args := []string{"list-units", "--type=service", "--no-pager", "--no-legend"}
	if failed {
		args = append(args, "--state=failed")
	} else {
		args = append(args, "--state=running,failed")
	}

	res, err := s.runner.RunSilent(ctx, "systemctl", args...)
	if err != nil {
		return nil, fmt.Errorf("listing services: %w", err)
	}

	var services []string
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		services = append(services, line)
	}
	return services, nil
}

func (s *Service) servicesLegacy(ctx context.Context, failed bool) ([]string, error) {
	if !executil.Exists("service") {
		return nil, fmt.Errorf("listing services: no supported init system found")
	}
	res, err := s.runner.RunSilent(ctx, "service", "--status-all")
	if err != nil {
		return nil, fmt.Errorf("listing services: %w", err)
	}

	var services []string
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// service --status-all output: " [ + ]  servicename" or " [ - ]  servicename"
		if failed {
			if strings.Contains(line, "[ - ]") {
				services = append(services, line)
			}
		} else {
			services = append(services, line)
		}
	}
	return services, nil
}
