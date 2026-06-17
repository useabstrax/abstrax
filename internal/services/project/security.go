package project

import (
	"fmt"
	"os"
	"strings"

	executil "abstrax/internal/exec"
)

// SecurityWarning describes a potential access restriction on the project path.
type SecurityWarning struct {
	Message string
}

// CheckSecurityWarnings returns actionable warnings for SELinux/AppArmor when relevant.
func CheckSecurityWarnings(projectPath string) []SecurityWarning {
	var warnings []SecurityWarning
	if selinuxEnforcing() {
		if strings.HasPrefix(projectPath, "/home/") {
			warnings = append(warnings, SecurityWarning{
				Message: "SELinux is enforcing and the project is under /home; you may need to adjust file contexts (for example semanage fcontext and restorecon) if nginx or PHP-FPM cannot access the site",
			})
		}
	}
	if appArmorEnabled() {
		warnings = append(warnings, SecurityWarning{
			Message: "AppArmor is enabled; ensure nginx and PHP-FPM profiles allow access to the project path if you see permission denied errors",
		})
	}
	return warnings
}

func selinuxEnforcing() bool {
	data, err := os.ReadFile("/sys/fs/selinux/enforce")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) == "1"
}

func appArmorEnabled() bool {
	if !executil.Exists("aa-status") {
		return false
	}
	data, err := os.ReadFile("/sys/module/apparmor/parameters/enabled")
	if err != nil {
		return executil.Exists("apparmor_status")
	}
	return strings.Contains(string(data), "Y")
}

func formatWarnings(warnings []SecurityWarning) string {
	if len(warnings) == 0 {
		return ""
	}
	var lines []string
	for _, w := range warnings {
		lines = append(lines, w.Message)
	}
	return fmt.Sprintf("Security note: %s", strings.Join(lines, " "))
}
