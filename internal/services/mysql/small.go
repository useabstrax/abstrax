package mysql

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// smallServerMemoryKB is 1 GiB. Hosts at or below this get a smaller
	// InnoDB buffer pool so MySQL 8.4 does not OOM-restart.
	smallServerMemoryKB = 1024 * 1024
	smallDropInName     = "zz-abstrax-small.cnf"
)

func needsSmallServerConfig(memKB int) bool {
	return memKB > 0 && memKB <= smallServerMemoryKB
}

func smallServerConfig() string {
	return `# Managed by Abstrax. Sized for a server with 1 GiB of memory or less.
[mysqld]
innodb_buffer_pool_size = 64M
performance_schema = OFF
max_connections = 50
`
}

func parseMemTotalKB(meminfo string) (int, error) {
	for _, line := range strings.Split(meminfo, "\n") {
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, fmt.Errorf("meminfo MemTotal has no value")
		}
		kb, err := strconv.Atoi(fields[1])
		if err != nil {
			return 0, fmt.Errorf("meminfo MemTotal: %w", err)
		}
		return kb, nil
	}
	return 0, fmt.Errorf("meminfo has no MemTotal")
}

func (s *Service) totalMemoryKB() (int, error) {
	if s.memTotalFn != nil {
		return s.memTotalFn()
	}
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	return parseMemTotalKB(string(data))
}

// ensureSmallServerConfig writes the small-server drop-in when this host has
// 1 GiB of memory or less. It returns true when that drop-in is in place.
// A host with more memory, or one whose memory cannot be read, is left unchanged.
func (s *Service) ensureSmallServerConfig() (bool, error) {
	kb, err := s.totalMemoryKB()
	if err != nil || !needsSmallServerConfig(kb) {
		return false, nil
	}
	path, err := s.smallConfigPath()
	if err != nil {
		return false, err
	}
	body := smallServerConfig()
	existing, readErr := os.ReadFile(path)
	if readErr == nil && string(existing) == body {
		return true, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("creating mysql config directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return false, fmt.Errorf("writing small-server mysql config: %w", err)
	}
	return true, nil
}

func (s *Service) smallConfigPath() (string, error) {
	dir, err := s.remoteDropInPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(dir), smallDropInName), nil
}
