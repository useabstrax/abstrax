package pkgmanager

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"abstrax/internal/platform"
)

// Backend is the shared package-manager surface used by Abstrax commands.
type Backend interface {
	Install(ctx context.Context, opts InstallOptions) error
	Remove(ctx context.Context, opts RemoveOptions) error
	Update(ctx context.Context) error
	Upgrade(ctx context.Context, securityOnly bool) error
	Search(ctx context.Context, query string) ([]PackageInfo, error)
	Info(ctx context.Context, name string) (*PackageInfo, error)
	List(ctx context.Context) ([]PackageInfo, error)
}

// NewForFamily returns the package manager backend for a distro family.
func NewForFamily(family string, dryRun, verbose bool) (Backend, error) {
	switch family {
	case "debian":
		return NewApt(dryRun, verbose), nil
	case "rhel":
		return NewDnf(dryRun, verbose), nil
	default:
		return nil, fmt.Errorf("no package manager backend for family %q", family)
	}
}

// NewFromPlatform detects the platform and returns the matching backend.
func NewFromPlatform(dryRun, verbose bool) (Backend, *platform.Info, error) {
	info, _, err := platform.Detect()
	if err != nil {
		return nil, nil, err
	}
	backend, err := NewForFamily(info.Family, dryRun, verbose)
	if err != nil {
		return nil, info, err
	}
	return backend, info, nil
}

// PackageInstalled reports whether a package is installed using the appropriate
// query tool for the detected package manager.
func PackageInstalled(name string) bool {
	info, _, err := platform.Detect()
	if err != nil {
		return packageInstalledApt(name) || packageInstalledRPM(name)
	}
	switch info.PackageManager {
	case "dnf", "yum":
		return packageInstalledRPM(name)
	default:
		return packageInstalledApt(name)
	}
}

func packageInstalledApt(name string) bool {
	cmd := exec.Command("dpkg-query", "-W", "-f=${Status}", name)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "install ok installed")
}

func packageInstalledRPM(name string) bool {
	cmd := exec.Command("rpm", "-q", name)
	return cmd.Run() == nil
}
