package project

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	executil "abstrax/internal/exec"
)

// ManagedACL records an ACL entry applied by Abstrax.
type ManagedACL struct {
	Path        string `json:"path"`
	Permissions string `json:"permissions"`
	Recurse     bool   `json:"recurse,omitempty"`
	Default     bool   `json:"default,omitempty"`
}

// ACLManager applies filesystem ACLs for nginx access to isolated projects.
type ACLManager struct {
	runner *executil.Runner
}

func newACLManager(runner *executil.Runner) *ACLManager {
	return &ACLManager{runner: runner}
}

// RequiredACLs returns ACL entries needed for nginx to serve an isolated project.
func RequiredACLs(paths *ValidatedPaths, id RuntimeIdentity) []ManagedACL {
	if paths == nil {
		return nil
	}

	var entries []ManagedACL
	for _, p := range traversalPaths(paths, id.Home) {
		entries = append(entries, ManagedACL{
			Path:        p,
			Permissions: "x",
		})
	}

	if paths.PublicPath != "" {
		entries = append(entries,
			ManagedACL{Path: paths.PublicPath, Permissions: "rX", Recurse: true},
			ManagedACL{Path: paths.PublicPath, Permissions: "rX", Recurse: true, Default: true},
		)
	}
	return entries
}

func traversalPaths(paths *ValidatedPaths, userHome string) []string {
	project := filepath.Clean(paths.ProjectPath)
	end := filepath.Clean(paths.PublicPath)
	if end != project {
		end = filepath.Dir(end)
	} else {
		end = project
	}

	var chain []string
	current := end
	for {
		chain = append([]string{current}, chain...)
		if current == "/" {
			break
		}
		current = filepath.Dir(current)
	}

	if userHome != "" && isStrictChild(userHome, project) {
		var filtered []string
		home := filepath.Clean(userHome)
		for _, p := range chain {
			if p == home || isStrictChild(home, p) {
				filtered = append(filtered, p)
			}
		}
		return filtered
	}

	if paths.ApprovedRoot != "" {
		root := filepath.Clean(paths.ApprovedRoot)
		var filtered []string
		for _, p := range chain {
			if p == root || isStrictChild(root, p) {
				filtered = append(filtered, p)
			}
		}
		parents := parentsFromRoot(root)
		return append(parents, filtered...)
	}

	return chain
}

func parentsFromRoot(root string) []string {
	var parents []string
	current := filepath.Dir(root)
	for current != "/" && current != "." {
		parents = append([]string{current}, parents...)
		current = filepath.Dir(current)
	}
	return parents
}

// Apply installs ACL entries for the web server user.
func (m *ACLManager) Apply(ctx context.Context, entries []ManagedACL, webUser string) error {
	if len(entries) == 0 {
		return nil
	}
	if !executil.Exists("setfacl") {
		return fmt.Errorf(
			"setfacl is not installed; install the acl package (for example: apt install acl) to create user-owned projects",
		)
	}

	for _, entry := range entries {
		args := []string{"-m"}
		if entry.Default {
			args = append(args, "-d")
		}
		if entry.Recurse {
			args = append(args, "-R")
		}
		args = append(args, fmt.Sprintf("u:%s:%s", webUser, entry.Permissions), entry.Path)
		if _, err := m.runner.Run(ctx, "setfacl", args...); err != nil {
			return fmt.Errorf("setting ACL on %q: %w", entry.Path, err)
		}
	}
	return nil
}

// Remove reverses managed ACL entries without touching unrelated ACLs.
func (m *ACLManager) Remove(ctx context.Context, entries []ManagedACL, webUser string) error {
	if len(entries) == 0 {
		return nil
	}
	if !executil.Exists("setfacl") {
		return nil
	}

	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		args := []string{"-x"}
		if entry.Default {
			args = append(args, "-d")
		}
		if entry.Recurse {
			args = append(args, "-R")
		}
		args = append(args, fmt.Sprintf("u:%s", webUser), entry.Path)
		_, _ = m.runner.Run(ctx, "setfacl", args...)
	}
	return nil
}

// ensureACLPackage installs the acl package when automatic installation is used elsewhere.
func ensureACLPackage(ctx context.Context, runner *executil.Runner) error {
	if executil.Exists("setfacl") {
		return nil
	}
	if !executil.Exists("apt-get") {
		return fmt.Errorf("setfacl is not installed and automatic package installation is unavailable")
	}
	_, err := runner.Run(ctx, "apt-get", "install", "-y", "acl")
	return err
}

func aclEntriesEqual(a, b []ManagedACL) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func formatACLDebug(entries []ManagedACL, webUser string) string {
	var lines []string
	for _, e := range entries {
		prefix := "setfacl"
		if e.Default {
			prefix += " -d"
		}
		if e.Recurse {
			prefix += " -R"
		}
		lines = append(lines, fmt.Sprintf("%s -m u:%s:%s %s", prefix, webUser, e.Permissions, e.Path))
	}
	return strings.Join(lines, "\n")
}
