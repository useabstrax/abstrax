package serverinfo

// ServerStatus holds a comprehensive view of server health.
type ServerStatus struct {
	Hostname      string     `json:"hostname"`
	Uptime        string     `json:"uptime"`
	LoadAverage   [3]float64 `json:"load_average"`
	CPU           CPUInfo    `json:"cpu"`
	Memory        MemoryInfo `json:"memory"`
	Swap          SwapInfo   `json:"swap"`
	Disks         []DiskInfo `json:"disks"`
	OS            OSInfo     `json:"os"`
	KernelVersion string     `json:"kernel_version"`
	PrivateIPs    []string   `json:"private_ips"`
}

// CPUInfo describes CPU usage.
type CPUInfo struct {
	Cores    int     `json:"cores"`
	UsagePct float64 `json:"usage_pct,omitempty"`
}

// MemoryInfo describes memory usage.
type MemoryInfo struct {
	TotalMB  int64   `json:"total_mb"`
	UsedMB   int64   `json:"used_mb"`
	FreeMB   int64   `json:"free_mb"`
	UsagePct float64 `json:"usage_pct"`
}

// SwapInfo describes swap usage.
type SwapInfo struct {
	TotalMB  int64   `json:"total_mb"`
	UsedMB   int64   `json:"used_mb"`
	UsagePct float64 `json:"usage_pct"`
}

// DiskInfo describes disk usage for a mount point.
type DiskInfo struct {
	MountPoint string  `json:"mount_point"`
	Device     string  `json:"device"`
	TotalGB    float64 `json:"total_gb"`
	UsedGB     float64 `json:"used_gb"`
	FreeGB     float64 `json:"free_gb"`
	UsagePct   float64 `json:"usage_pct"`
}

// OSInfo describes the OS.
type OSInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Pretty  string `json:"pretty"`
}
