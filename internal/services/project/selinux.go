package project

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	selinuxContentType   = "httpd_sys_content_t"
	selinuxRWContentType = "httpd_sys_rw_content_t"
)

// Common app-writable trees. fcontext rules are added even when the directory
// does not exist yet so Laravel/WordPress can create them later.
var defaultWritableRelPaths = []string{
	"storage",
	"bootstrap/cache",
	"wp-content/uploads",
}

var (
	selinuxEnabledFn = projectSELinuxEnabled
	runSELinuxCmdFn  = func(name string, args ...string) error {
		return exec.Command(name, args...).Run()
	}
)

func projectSELinuxEnabled() bool {
	out, err := exec.Command("getenforce").Output()
	if err != nil {
		return selinuxEnforcing()
	}
	switch strings.ToLower(strings.TrimSpace(string(out))) {
	case "enforcing", "permissive":
		return true
	default:
		return false
	}
}

func fcontextPattern(path string) string {
	cleaned := filepath.Clean(path)
	if cleaned == "" || cleaned == "." {
		return ""
	}
	return cleaned + "(/.*)?"
}

// applyProjectSELinuxLabels adds persistent httpd file contexts for a project
// path and restores labels. It returns a short note of what was done, or empty
// when SELinux is off.
func applyProjectSELinuxLabels(projectPath string) (string, error) {
	if !selinuxEnabledFn() {
		return "", nil
	}
	projectPath = filepath.Clean(projectPath)
	if projectPath == "" || projectPath == "." {
		return "", nil
	}

	patterns := []struct {
		pattern string
		typeName string
	}{
		{fcontextPattern(projectPath), selinuxContentType},
	}
	for _, rel := range defaultWritableRelPaths {
		patterns = append(patterns, struct {
			pattern  string
			typeName string
		}{
			fcontextPattern(filepath.Join(projectPath, rel)),
			selinuxRWContentType,
		})
	}

	for _, p := range patterns {
		if p.pattern == "" {
			continue
		}
		if err := ensureFContext(p.pattern, p.typeName); err != nil {
			return "", err
		}
	}

	if err := runSELinuxCmdFn("restorecon", "-RF", projectPath); err != nil {
		return "", fmt.Errorf("restoring SELinux context on %s: %w", projectPath, err)
	}

	note := fmt.Sprintf("SELinux: labelled %s as %s (writable trees as %s) and ran restorecon",
		projectPath, selinuxContentType, selinuxRWContentType)
	return note, nil
}

// removeProjectSELinuxLabels drops fcontext rules Abstrax added for a project.
func removeProjectSELinuxLabels(projectPath string) error {
	if !selinuxEnabledFn() {
		return nil
	}
	projectPath = filepath.Clean(projectPath)
	if projectPath == "" || projectPath == "." {
		return nil
	}

	paths := []string{projectPath}
	for _, rel := range defaultWritableRelPaths {
		paths = append(paths, filepath.Join(projectPath, rel))
	}
	for _, path := range paths {
		pattern := fcontextPattern(path)
		if pattern == "" {
			continue
		}
		_ = runSELinuxCmdFn("semanage", "fcontext", "-d", pattern)
	}
	return nil
}

func ensureFContext(pattern, typeName string) error {
	if err := runSELinuxCmdFn("semanage", "fcontext", "-a", "-t", typeName, pattern); err == nil {
		return nil
	}
	if err := runSELinuxCmdFn("semanage", "fcontext", "-m", "-t", typeName, pattern); err != nil {
		return fmt.Errorf("setting SELinux fcontext %s -> %s: %w", pattern, typeName, err)
	}
	return nil
}
