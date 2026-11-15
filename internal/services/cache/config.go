package cache

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"abstrax/internal/platform"
)

// ConfigInfo is the effective cache driver configuration Abstrax manages.
type ConfigInfo struct {
	Driver          Driver `json:"driver"`
	Path            string `json:"path"`
	Port            string `json:"port,omitempty"`
	Bind            string `json:"bind,omitempty"`
	MaxMemory       string `json:"maxmemory,omitempty"`
	MaxMemoryPolicy string `json:"maxmemory_policy,omitempty"`
	RequirePassSet  bool   `json:"requirepass_set,omitempty"`
	MemoryMB        string `json:"memory_mb,omitempty"`
}

// ConfigOptions updates common cache settings.
type ConfigOptions struct {
	Driver          Driver
	Port            int
	Bind            string
	Memory          string
	MaxMemoryPolicy string
	RequirePass     string
	ClearRequirePass bool
	DryRun          bool
}

// Config returns the effective settings for a cache driver.
func (s *Service) Config(_ context.Context, driver Driver) (*ConfigInfo, error) {
	provider := platform.Resolve()
	switch driver {
	case DriverRedis:
		return readRedisConfigInfo(provider.RedisConfigPath(), driver)
	case DriverMemcached:
		return readMemcachedConfigInfo(provider.MemcachedConfigPath(), driver)
	default:
		return nil, fmt.Errorf("unsupported cache driver %q", driver)
	}
}

// ApplyConfig writes common settings and restarts the service when anything changed.
func (s *Service) ApplyConfig(ctx context.Context, opts ConfigOptions) (*ConfigInfo, error) {
	if opts.Driver == "" {
		return nil, fmt.Errorf("cache driver is required")
	}
	if !opts.hasChanges() {
		return s.Config(ctx, opts.Driver)
	}
	if opts.DryRun {
		info, err := s.Config(ctx, opts.Driver)
		if err != nil {
			return nil, err
		}
		preview := *info
		if opts.Port > 0 {
			preview.Port = strconv.Itoa(opts.Port)
		}
		if opts.Bind != "" {
			preview.Bind = opts.Bind
		}
		if opts.Memory != "" {
			preview.MaxMemory = opts.Memory
			preview.MemoryMB = opts.Memory
		}
		if opts.MaxMemoryPolicy != "" {
			preview.MaxMemoryPolicy = opts.MaxMemoryPolicy
		}
		if opts.RequirePass != "" {
			preview.RequirePassSet = true
		}
		if opts.ClearRequirePass {
			preview.RequirePassSet = false
		}
		return &preview, nil
	}

	provider := platform.Resolve()
	installOpts := InstallOptions{
		Driver:          opts.Driver,
		Port:            opts.Port,
		Bind:            opts.Bind,
		Memory:          opts.Memory,
		MaxMemoryPolicy: opts.MaxMemoryPolicy,
		Password:        opts.RequirePass,
	}

	switch opts.Driver {
	case DriverRedis:
		if err := applyRedisConfigSettings(provider.RedisConfigPath(), installOpts, opts.ClearRequirePass); err != nil {
			return nil, err
		}
	case DriverMemcached:
		if opts.MaxMemoryPolicy != "" || opts.RequirePass != "" || opts.ClearRequirePass {
			return nil, fmt.Errorf("memcached does not support maxmemory-policy or requirepass")
		}
		if err := applyMemcachedConfig(provider.MemcachedConfigPath(), installOpts); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported cache driver %q", opts.Driver)
	}

	_, svcName, err := driverNames(provider, opts.Driver)
	if err != nil {
		return nil, err
	}
	if err := s.svc.Restart(ctx, svcName); err != nil {
		return nil, fmt.Errorf("restarting %s after config change: %w", svcName, err)
	}
	return s.Config(ctx, opts.Driver)
}

func (o ConfigOptions) hasChanges() bool {
	return o.Port > 0 || o.Bind != "" || o.Memory != "" || o.MaxMemoryPolicy != "" || o.RequirePass != "" || o.ClearRequirePass
}

func readRedisConfigInfo(path string, driver Driver) (*ConfigInfo, error) {
	info := &ConfigInfo{Driver: driver, Path: path}
	data, err := readFileFn(path)
	if err != nil {
		if os.IsNotExist(err) {
			return info, nil
		}
		return nil, fmt.Errorf("reading redis config: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "port":
			info.Port = fields[1]
		case "bind":
			info.Bind = fields[1]
		case "maxmemory":
			info.MaxMemory = fields[1]
		case "maxmemory-policy":
			info.MaxMemoryPolicy = fields[1]
		case "requirepass":
			info.RequirePassSet = fields[1] != ""
		}
	}
	return info, nil
}

func readMemcachedConfigInfo(path string, driver Driver) (*ConfigInfo, error) {
	info := &ConfigInfo{Driver: driver, Path: path}
	port, bind := readMemcachedEndpoint(path)
	info.Port = port
	info.Bind = bind

	data, err := readFileFn(path)
	if err != nil {
		if os.IsNotExist(err) {
			return info, nil
		}
		return nil, fmt.Errorf("reading memcached config: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "-m") {
			fields := strings.Fields(trimmed)
			for i := 0; i < len(fields)-1; i++ {
				if fields[i] == "-m" {
					info.MemoryMB = fields[i+1]
				}
			}
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(key), "CACHESIZE") {
			info.MemoryMB = strings.Trim(value, `"'`)
		}
	}
	return info, nil
}

func applyRedisConfigSettings(configPath string, opts InstallOptions, clearPass bool) error {
	if opts.Port == 0 && opts.Bind == "" && opts.Memory == "" && opts.MaxMemoryPolicy == "" && opts.Password == "" && !clearPass {
		return nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("reading redis config: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	if opts.Port > 0 {
		lines = setConfigLine(lines, "port", strconv.Itoa(opts.Port))
	}
	if opts.Bind != "" {
		lines = setConfigLine(lines, "bind", opts.Bind)
	}
	if opts.Memory != "" {
		lines = setConfigLine(lines, "maxmemory", opts.Memory)
	}
	if opts.MaxMemoryPolicy != "" {
		lines = setConfigLine(lines, "maxmemory-policy", opts.MaxMemoryPolicy)
	}
	if opts.Password != "" {
		lines = setConfigLine(lines, "requirepass", opts.Password)
	}
	if clearPass {
		lines = removeConfigLine(lines, "requirepass")
	}

	content := strings.Join(lines, "\n")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing redis config: %w", err)
	}
	return nil
}

func removeConfigLine(lines []string, key string) []string {
	out := make([]string, 0, len(lines))
	prefix := key + " "
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			out = append(out, line)
			continue
		}
		if strings.HasPrefix(trimmed, prefix) || trimmed == key {
			continue
		}
		out = append(out, line)
	}
	return out
}
