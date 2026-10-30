package sshcfg

import (
	"path/filepath"
	"strconv"
	"strings"

	"abstrax/internal/platform"
	"abstrax/internal/validate"
)

const defaultSSHPort = 22

// SSHPort returns the configured SSH listening port. It reads the Abstrax
// managed include file first, then a legacy 99-abstrax.conf, then the main
// sshd_config, and defaults to 22.
func SSHPort() (int, error) {
	portStr := portFromConfigFiles()
	if portStr == "" {
		return defaultSSHPort, nil
	}

	if err := validate.PortString(portStr); err != nil {
		return defaultSSHPort, nil
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return defaultSSHPort, nil
	}
	return port, nil
}

func portFromConfigFiles() string {
	paths := platform.Resolve().Paths()
	if port := portFromEntries(readConfigFileSafe(paths.AbstraxSSHConfig)); port != "" {
		return port
	}
	legacy := filepath.Join(paths.SSHConfigDir, legacyManagedFile)
	if filepath.Clean(legacy) != filepath.Clean(paths.AbstraxSSHConfig) {
		if port := portFromEntries(readConfigFileSafe(legacy)); port != "" {
			return port
		}
	}
	return portFromEntries(readConfigFileSafe(sshdConfigPath))
}

func readConfigFileSafe(path string) []ConfigEntry {
	entries, err := readConfigFile(path)
	if err != nil {
		return nil
	}
	return entries
}

func portFromEntries(entries []ConfigEntry) string {
	for _, e := range entries {
		if strings.EqualFold(e.Key, "port") {
			return e.Value
		}
	}
	return ""
}
