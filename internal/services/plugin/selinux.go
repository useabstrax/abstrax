package plugin

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

const pluginBinarySELinuxType = "bin_t"

// Test seams for SELinux labeling.
var (
	selinuxEnabledFn = selinuxEnabled
	runSELinuxCmdFn  = runSELinuxCmd
)

func selinuxEnabled() bool {
	out, err := exec.Command("getenforce").Output()
	if err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(string(out))) {
	case "enforcing", "permissive":
		return true
	default:
		return false
	}
}

func runSELinuxCmd(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

func pluginFContextPattern(installDir string) string {
	dir := filepath.Clean(installDir)
	if dir == "" || dir == "." {
		return ""
	}
	return dir + "(/.*)?"
}

// ensurePluginSELinuxLabels gives installed plugin binaries a persistent bin_t
// file context so systemd can execute them. Downloads land in /tmp as tmp_t;
// renaming into the plugin directory keeps that label unless restorecon runs
// after a matching fcontext rule.
func (s *Service) ensurePluginSELinuxLabels(binaryPath string) error {
	if s == nil || s.paths == nil || !selinuxEnabledFn() {
		return nil
	}
	installDir := filepath.Clean(s.paths.InstallDir)
	pattern := pluginFContextPattern(installDir)
	if pattern == "" {
		return nil
	}

	if err := ensurePluginFContext(pattern); err != nil {
		return err
	}

	if binaryPath != "" {
		if err := runSELinuxCmdFn("restorecon", "-F", binaryPath); err != nil {
			return fmt.Errorf("restoring SELinux context on %s: %w", binaryPath, err)
		}
	}
	if err := runSELinuxCmdFn("restorecon", "-RF", installDir); err != nil {
		return fmt.Errorf("restoring SELinux context on %s: %w", installDir, err)
	}
	return nil
}

func ensurePluginFContext(pattern string) error {
	// -a fails when the rule already exists; -m updates it.
	if err := runSELinuxCmdFn("semanage", "fcontext", "-a", "-t", pluginBinarySELinuxType, pattern); err == nil {
		return nil
	}
	if err := runSELinuxCmdFn("semanage", "fcontext", "-m", "-t", pluginBinarySELinuxType, pattern); err != nil {
		return fmt.Errorf("setting SELinux fcontext %s -> %s: %w", pattern, pluginBinarySELinuxType, err)
	}
	return nil
}

func preferCopyInstall() bool {
	return selinuxEnabledFn()
}
