package pkgmanager

import (
	"context"
	"fmt"
	"strings"

	executil "abstrax/internal/exec"
)

// DnfManager implements Manager for dnf-based systems.
type DnfManager struct {
	runner *executil.Runner
}

// NewDnf creates a DnfManager.
func NewDnf(dryRun, verbose bool) *DnfManager {
	return &DnfManager{runner: executil.New(dryRun, verbose)}
}

// Install installs a package with dnf.
func (d *DnfManager) Install(ctx context.Context, opts InstallOptions) error {
	pkg := opts.Name
	if opts.Version != "" {
		pkg = fmt.Sprintf("%s-%s", opts.Name, opts.Version)
	}
	res, err := d.runner.Run(ctx, "dnf", "install", "-y", pkg)
	if err != nil {
		if res.Stderr != "" {
			return fmt.Errorf("dnf install %s: %s", pkg, strings.TrimSpace(res.Stderr))
		}
		return fmt.Errorf("dnf install %s: %w", pkg, err)
	}
	return nil
}

// Remove removes a package with dnf.
func (d *DnfManager) Remove(ctx context.Context, opts RemoveOptions) error {
	res, err := d.runner.Run(ctx, "dnf", "remove", "-y", opts.Name)
	if err != nil {
		if res.Stderr != "" {
			return fmt.Errorf("dnf remove %s: %s", opts.Name, strings.TrimSpace(res.Stderr))
		}
		return fmt.Errorf("dnf remove %s: %w", opts.Name, err)
	}
	return nil
}

// Update refreshes package metadata.
func (d *DnfManager) Update(ctx context.Context) error {
	_, err := d.runner.Run(ctx, "dnf", "makecache")
	return err
}

// Upgrade upgrades installed packages.
func (d *DnfManager) Upgrade(ctx context.Context, securityOnly bool) error {
	args := []string{"upgrade", "-y"}
	if securityOnly {
		args = []string{"upgrade", "-y", "--security"}
	}
	_, err := d.runner.Run(ctx, "dnf", args...)
	return err
}

// Search searches for packages matching a query.
func (d *DnfManager) Search(ctx context.Context, query string) ([]PackageInfo, error) {
	res, err := d.runner.RunSilent(ctx, "dnf", "search", query)
	if err != nil {
		return nil, fmt.Errorf("dnf search: %w", err)
	}

	var pkgs []PackageInfo
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "=") || strings.HasPrefix(line, "Last metadata") {
			continue
		}
		// Example: nginx.x86_64 : A high performance web server
		parts := strings.SplitN(line, " : ", 2)
		name := strings.Fields(parts[0])
		if len(name) == 0 {
			continue
		}
		p := PackageInfo{Name: strings.Split(name[0], ".")[0]}
		if len(parts) == 2 {
			p.Description = parts[1]
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

// Info returns information about a specific package.
func (d *DnfManager) Info(ctx context.Context, name string) (*PackageInfo, error) {
	res, err := d.runner.RunSilent(ctx, "dnf", "info", name)
	if err != nil {
		return nil, fmt.Errorf("package %s not found", name)
	}

	p := &PackageInfo{Name: name}
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Version") {
			fields := strings.SplitN(line, ":", 2)
			if len(fields) == 2 {
				p.Version = strings.TrimSpace(fields[1])
			}
		} else if strings.HasPrefix(line, "Summary") {
			fields := strings.SplitN(line, ":", 2)
			if len(fields) == 2 {
				p.Description = strings.TrimSpace(fields[1])
			}
		} else if strings.HasPrefix(line, "Architecture") {
			fields := strings.SplitN(line, ":", 2)
			if len(fields) == 2 {
				p.Architecture = strings.TrimSpace(fields[1])
			}
		}
	}

	if statusRes, err := d.runner.RunSilent(ctx, "rpm", "-q", name); err == nil && statusRes.ExitCode == 0 {
		p.Status = "installed"
	} else {
		p.Status = "not installed"
	}

	return p, nil
}

// List lists installed packages.
func (d *DnfManager) List(ctx context.Context) ([]PackageInfo, error) {
	res, err := d.runner.RunSilent(ctx, "rpm", "-qa", "--qf", "%{NAME}|%{VERSION}|%{ARCH}\n")
	if err != nil {
		return nil, fmt.Errorf("listing packages: %w", err)
	}

	var pkgs []PackageInfo
	for _, line := range strings.Split(res.Stdout, "\n") {
		parts := strings.Split(line, "|")
		if len(parts) < 3 {
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
