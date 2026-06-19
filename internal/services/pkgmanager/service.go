// Package pkgmanager provides a package manager abstraction with an initial
// apt backend.
package pkgmanager

import (
	"context"
	"fmt"
	"strings"

	executil "abstrax/internal/exec"
)

// AptManager implements Manager for apt-based systems.
type AptManager struct {
	runner *executil.Runner
}

// NewApt creates an AptManager.
func NewApt(dryRun, verbose bool) *AptManager {
	return &AptManager{runner: executil.New(dryRun, verbose)}
}

// Install installs a package.
func (a *AptManager) Install(ctx context.Context, opts InstallOptions) error {
	env := []string{"DEBIAN_FRONTEND=noninteractive"}
	pkg := opts.Name
	if opts.Version != "" {
		pkg = fmt.Sprintf("%s=%s", opts.Name, opts.Version)
	}

	args := append(env, "apt-get", "install", "-y", pkg)
	res, err := a.runner.Run(ctx, "env", args...)
	if err != nil {
		if res.Stderr != "" {
			return fmt.Errorf("apt install %s: %s", pkg, strings.TrimSpace(res.Stderr))
		}
		return fmt.Errorf("apt install %s: %w", pkg, err)
	}
	return nil
}

// Remove removes a package.
func (a *AptManager) Remove(ctx context.Context, opts RemoveOptions) error {
	cmd := "remove"
	if opts.Purge {
		cmd = "purge"
	}
	env := []string{"DEBIAN_FRONTEND=noninteractive"}
	args := append(env, "apt-get", cmd, "-y", opts.Name)
	_, err := a.runner.Run(ctx, "env", args...)
	if err != nil {
		return fmt.Errorf("apt %s %s: %w", cmd, opts.Name, err)
	}
	return nil
}

// Update runs apt-get update.
func (a *AptManager) Update(ctx context.Context) error {
	_, err := a.runner.Run(ctx, "apt-get", "update")
	return err
}

// Upgrade runs apt-get upgrade.
func (a *AptManager) Upgrade(ctx context.Context, securityOnly bool) error {
	env := []string{"DEBIAN_FRONTEND=noninteractive"}
	if securityOnly {
		// Use unattended-upgrades for security-only on Debian/Ubuntu.
		args := append(env, "unattended-upgrade", "-d")
		_, err := a.runner.Run(ctx, "env", args...)
		return err
	}
	args := append(env, "apt-get", "upgrade", "-y")
	_, err := a.runner.Run(ctx, "env", args...)
	return err
}

// Search searches for packages matching a query.
func (a *AptManager) Search(ctx context.Context, query string) ([]PackageInfo, error) {
	res, err := a.runner.RunSilent(ctx, "apt-cache", "search", query)
	if err != nil {
		return nil, fmt.Errorf("apt-cache search: %w", err)
	}

	var pkgs []PackageInfo
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " - ", 2)
		p := PackageInfo{Name: parts[0]}
		if len(parts) == 2 {
			p.Description = parts[1]
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

// Info returns information about a specific package.
func (a *AptManager) Info(ctx context.Context, name string) (*PackageInfo, error) {
	res, err := a.runner.RunSilent(ctx, "apt-cache", "show", name)
	if err != nil {
		return nil, fmt.Errorf("package %s not found", name)
	}

	p := &PackageInfo{Name: name}
	for _, line := range strings.Split(res.Stdout, "\n") {
		if strings.HasPrefix(line, "Version:") {
			p.Version = strings.TrimSpace(strings.TrimPrefix(line, "Version:"))
		} else if strings.HasPrefix(line, "Description:") {
			p.Description = strings.TrimSpace(strings.TrimPrefix(line, "Description:"))
		} else if strings.HasPrefix(line, "Architecture:") {
			p.Architecture = strings.TrimSpace(strings.TrimPrefix(line, "Architecture:"))
		}
	}

	// Check install status.
	if statusRes, err := a.runner.RunSilent(ctx, "dpkg-query", "-W", "-f=${Status}", name); err == nil {
		if strings.Contains(statusRes.Stdout, "install ok installed") {
			p.Status = "installed"
		} else {
			p.Status = "not installed"
		}
	}

	return p, nil
}

// List lists installed packages.
func (a *AptManager) List(ctx context.Context) ([]PackageInfo, error) {
	res, err := a.runner.RunSilent(ctx, "dpkg-query", "-W", "-f=${Package}|${Version}|${Architecture}|${Status}\n")
	if err != nil {
		return nil, fmt.Errorf("listing packages: %w", err)
	}

	var pkgs []PackageInfo
	for _, line := range strings.Split(res.Stdout, "\n") {
		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}
		if !strings.Contains(parts[3], "install ok installed") {
			continue
		}
		pkgs = append(pkgs, PackageInfo{
			Name:         parts[0],
			Version:      parts[1],
			Architecture: parts[2],
			Status:       "installed",
		})
	}
	return pkgs, nil
}
