package ssl

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	executil "abstrax/internal/exec"
	"abstrax/internal/services/pkgmanager"
)

const (
	certbotPackage      = "certbot"
	certbotNginxPackage = "python3-certbot-nginx"
)

// InstallOptions holds options for installing Certbot.
type InstallOptions struct {
	DryRun bool
}

// Installed reports whether Certbot and the nginx plugin are available.
func Installed() bool {
	return executil.Exists("certbot") && nginxPluginInstalled()
}

// InstallCommand returns the abstrax command to install Certbot.
func InstallCommand() string {
	return "abstrax ssl install"
}

// Install installs Certbot and the nginx plugin via apt.
func (s *Service) Install(ctx context.Context, opts InstallOptions) error {
	mgr := pkgmanager.NewApt(s.dryRun, false)

	if err := mgr.Update(ctx); err != nil {
		return fmt.Errorf("updating package lists: %w", err)
	}

	for _, pkg := range certbotInstallPackages() {
		if err := mgr.Install(ctx, pkgmanager.InstallOptions{
			Name:   pkg,
			DryRun: opts.DryRun,
		}); err != nil {
			return fmt.Errorf("installing %s: %w", pkg, err)
		}
	}

	if opts.DryRun {
		return nil
	}

	if !Installed() {
		return fmt.Errorf("certbot installed but nginx plugin is not available; check %s is installed", certbotNginxPackage)
	}

	return nil
}

func certbotInstallPackages() []string {
	return []string{certbotPackage, certbotNginxPackage}
}

func nginxPluginInstalled() bool {
	if packageInstalled(certbotNginxPackage) {
		return true
	}

	if !executil.Exists("certbot") {
		return false
	}

	res, err := exec.Command("certbot", "plugins", "--non-interactive").CombinedOutput()
	if err != nil {
		return false
	}

	return strings.Contains(string(res), "nginx")
}

func packageInstalled(name string) bool {
	cmd := exec.Command("dpkg-query", "-W", "-f=${Status}", name)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "install ok installed")
}
