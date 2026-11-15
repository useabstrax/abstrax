package cache

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"abstrax/internal/platform"
	"abstrax/internal/services/pkgmanager"
)

var (
	redisVersionPattern     = regexp.MustCompile(`v=(\d+\.\d+\.\d+)`)
	memcachedVersionPattern = regexp.MustCompile(`memcached\s+(\d+\.\d+\.\d+)`)
	versionNumberPattern    = regexp.MustCompile(`\d+\.\d+\.\d+`)
)

// packageInstalledFn and lookPathFn are seams for tests.
var (
	packageInstalledFn = pkgmanager.PackageInstalled
	lookPathFn         = exec.LookPath
	readFileFn         = os.ReadFile
	runVersionFn       = func(ctx context.Context, binary string, args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, binary, args...)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
)

func (s *Service) fillStatus(ctx context.Context, provider platform.Provider, d Driver, info *StatusInfo) {
	pkg, _, err := driverNames(provider, d)
	if err != nil {
		return
	}

	installed := packageInstalledFn(pkg)
	binary, binErr := driverBinary(d)
	if binErr == nil {
		installed = true
	}
	info.Installed = installed
	if !installed {
		return
	}

	if binErr == nil {
		if version, ok := s.readDriverVersion(ctx, d, binary); ok {
			info.Version = &version
		}
	}

	port, bind := readDriverEndpoint(provider, d)
	info.Port = port
	info.Bind = bind
}

func driverBinary(d Driver) (string, error) {
	switch d {
	case DriverRedis:
		if path, err := lookPathFn("redis-server"); err == nil {
			return path, nil
		}
		return lookPathFn("redis")
	case DriverMemcached:
		return lookPathFn("memcached")
	default:
		return "", fmt.Errorf("unsupported cache driver %q", d)
	}
}

func (s *Service) readDriverVersion(ctx context.Context, d Driver, binary string) (string, bool) {
	switch d {
	case DriverRedis:
		out, err := runVersionFn(ctx, binary, "--version")
		if err != nil && out == "" {
			return "", false
		}
		return parseRedisVersion(out)
	case DriverMemcached:
		out, err := runVersionFn(ctx, binary, "-V")
		if err != nil && out == "" {
			out, err = runVersionFn(ctx, binary, "-h")
		}
		if err != nil && out == "" {
			return "", false
		}
		return parseMemcachedVersion(out)
	default:
		return "", false
	}
}

func parseRedisVersion(raw string) (string, bool) {
	if m := redisVersionPattern.FindStringSubmatch(raw); len(m) == 2 {
		return m[1], true
	}
	if v := versionNumberPattern.FindString(raw); v != "" {
		return v, true
	}
	return "", false
}

func parseMemcachedVersion(raw string) (string, bool) {
	if m := memcachedVersionPattern.FindStringSubmatch(strings.ToLower(raw)); len(m) == 2 {
		return m[1], true
	}
	// First line of `memcached -h` is often "memcached 1.6.18".
	first := strings.TrimSpace(strings.Split(raw, "\n")[0])
	if m := memcachedVersionPattern.FindStringSubmatch(strings.ToLower(first)); len(m) == 2 {
		return m[1], true
	}
	if v := versionNumberPattern.FindString(raw); v != "" {
		return v, true
	}
	return "", false
}

func readDriverEndpoint(provider platform.Provider, d Driver) (port, bind string) {
	switch d {
	case DriverRedis:
		return readRedisEndpoint(provider.RedisConfigPath())
	case DriverMemcached:
		return readMemcachedEndpoint(provider.MemcachedConfigPath())
	default:
		return "", ""
	}
}

func readRedisEndpoint(path string) (port, bind string) {
	data, err := readFileFn(path)
	if err != nil {
		return "", ""
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
			port = fields[1]
		case "bind":
			bind = fields[1]
		}
	}
	return port, bind
}

func readMemcachedEndpoint(path string) (port, bind string) {
	data, err := readFileFn(path)
	if err != nil {
		return "", ""
	}
	body := string(data)
	// Debian: lines like "-p 11211" / "-l 127.0.0.1"
	// RHEL sysconfig: PORT="11211" / USER=... / OPTIONS="-l 127.0.0.1"
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "-") {
			fields := strings.Fields(trimmed)
			for i := 0; i < len(fields)-1; i++ {
				switch fields[i] {
				case "-p":
					port = fields[i+1]
				case "-l":
					bind = fields[i+1]
				}
			}
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		value = strings.Trim(value, `"'`)
		switch strings.ToUpper(strings.TrimSpace(key)) {
		case "PORT":
			port = value
		case "OPTIONS":
			fields := strings.Fields(value)
			for i := 0; i < len(fields)-1; i++ {
				if fields[i] == "-l" {
					bind = fields[i+1]
				}
				if fields[i] == "-p" && port == "" {
					port = fields[i+1]
				}
			}
		}
	}
	return port, bind
}
