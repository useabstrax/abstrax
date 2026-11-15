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
// When SELinux labels were applied successfully, pass labelledNote so the message
// describes what Abstrax did instead of only warning.
func CheckSecurityWarnings(projectPath string, labelledNote string) []SecurityWarning {
	var warnings []SecurityWarning
	if selinuxEnforcing() || (labelledNote == "" && selinuxEnabledFn()) {
		if labelledNote != "" {
			warnings = append(warnings, SecurityWarning{Message: labelledNote})
		} else if selinuxEnforcing() {
			msg := "SELinux is enforcing"
			if strings.HasPrefix(projectPath, "/home/") {
				msg += " and the project is under /home"
			}
			msg += "; file context labelling failed or was skipped. Abstrax will not disable SELinux automatically"
			warnings = append(warnings, SecurityWarning{Message: msg})
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
