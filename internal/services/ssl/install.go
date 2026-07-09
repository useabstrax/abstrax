package ssl

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	executil "abstrax/internal/exec"
	"abstrax/internal/globals"
	"abstrax/internal/platform"
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

// Install installs Certbot and the nginx plugin via the platform package manager.
func (s *Service) Install(ctx context.Context, opts InstallOptions) error {
	mgr, _, err := pkgmanager.NewFromPlatform(s.dryRun, false)
	if err != nil {
		return err
	}
	provider := platform.Resolve()

	repoOpts := platform.RepoOptions{
		EnableRequiredRepos: globals.Flags.EnableRequiredRepos,
		Yes:                 globals.Flags.Yes,
		DryRun:              opts.DryRun,
		Verbose:             globals.Flags.Verbose,
	}
	enabler := platform.DefaultRepoEnabler(
		func(ctx context.Context, name string) error {
			return mgr.Install(ctx, pkgmanager.InstallOptions{Name: name, DryRun: opts.DryRun})
		},
		func(ctx context.Context, name string, args ...string) error {
			_, err := s.runner.Run(ctx, name, args...)
			return err
		},
	)
	if err := platform.EnsureEPELWithOptions(ctx, provider, repoOpts, enabler); err != nil {
		return err
	}

	if err := mgr.Update(ctx); err != nil {
		return fmt.Errorf("updating package lists: %w", err)
	}

	for _, pkg := range provider.CertbotPackages() {
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
		pkgs := provider.CertbotPackages()
		hint := certbotNginxPackage
		if len(pkgs) > 1 {
			hint = pkgs[1]
		}
		return fmt.Errorf("certbot installed but nginx plugin is not available; check %s is installed", hint)
	}

	return nil
}

func certbotInstallPackages() []string {
	return platform.Resolve().CertbotPackages()
}

func nginxPluginInstalled() bool {
	for _, pkg := range platform.Resolve().CertbotPackages() {
		if pkg == certbotPackage {
			continue
		}
		if pkgmanager.PackageInstalled(pkg) {
			return true
		}
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
