// Package web manages web servers (nginx initially; Apache is a stub).
package web

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	executil "abstrax/internal/exec"
	"abstrax/internal/platform"
	"abstrax/internal/services/pkgmanager"
	"abstrax/internal/services/svcmanager"
)

const defaultSiteConfigName = "default"

// Service manages web servers.
type Service struct {
	runner   *executil.Runner
	svc      *svcmanager.Service
	provider platform.Provider
	dryRun   bool
}

// New creates a Service.
func New(dryRun, verbose bool) *Service {
	return &Service{
		runner:   executil.New(dryRun, verbose),
		svc:      svcmanager.New(dryRun, verbose),
		provider: platform.Resolve(),
		dryRun:   dryRun,
	}
}

// Install installs and configures a web server.
func (s *Service) Install(ctx context.Context, opts InstallOptions) error {
	switch opts.Backend {
	case BackendApache:
		return fmt.Errorf("Apache support is not yet implemented")
	case BackendNginx, "":
		return s.installNginx(ctx, opts)
	default:
		return fmt.Errorf("unknown web server %q", opts.Backend)
	}
}

func (s *Service) installNginx(ctx context.Context, opts InstallOptions) error {
	mgr, _, err := pkgmanager.NewFromPlatform(s.dryRun, false)
	if err != nil {
		return err
	}

	if err := mgr.Update(ctx); err != nil {
		return err
	}
	if err := mgr.Install(ctx, pkgmanager.InstallOptions{Name: "nginx"}); err != nil {
		return fmt.Errorf("installing nginx: %w", err)
	}

	if err := s.configureNginx(ctx); err != nil {
		return err
	}

	if note := platform.SELinuxWarning(s.provider.Profile().SELinuxStatus, "nginx configuration"); note != "" {
		fmt.Println("Security note: " + note)
	}

	if opts.Enable || opts.Start {
		if err := s.svc.Enable(ctx, "nginx"); err != nil {
			return err
		}
	}
	if opts.Start {
		if err := s.svc.Start(ctx, "nginx"); err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) configureNginx(ctx context.Context) error {
	layout := s.provider.NginxLayout()
	if layout == platform.NginxConfD {
		confDir := s.provider.NginxConfigDir()
		if s.dryRun {
			fmt.Printf("[dry-run] would ensure %s exists\n", confDir)
		} else if err := os.MkdirAll(confDir, 0755); err != nil {
			return fmt.Errorf("creating %s: %w", confDir, err)
		}
		if err := s.disableDefaultConfDSite(); err != nil {
			return err
		}
	} else {
		avail := s.provider.NginxSitesAvailable()
		enabled := s.provider.NginxSitesEnabled()
		if s.dryRun {
			fmt.Printf("[dry-run] would create %s and %s\n", avail, enabled)
		} else {
			if err := os.MkdirAll(avail, 0755); err != nil {
				return fmt.Errorf("creating %s: %w", avail, err)
			}
			if err := os.MkdirAll(enabled, 0755); err != nil {
				return fmt.Errorf("creating %s: %w", enabled, err)
			}
		}
		if err := s.ensureNginxSitesInclude(); err != nil {
			return err
		}
		if err := s.disableDefaultSite(); err != nil {
			return err
		}
	}

	res, err := s.runner.RunSilent(ctx, "nginx", "-t")
	if err != nil || res.ExitCode != 0 {
		return fmt.Errorf("nginx config test failed: %s", res.Stderr+res.Stdout)
	}

	return nil
}

func (s *Service) ensureNginxSitesInclude() error {
	confPath := s.provider.NginxConfPath()
	includeLine := s.provider.NginxSitesEnabledInclude()
	if s.dryRun {
		fmt.Printf("[dry-run] would ensure %s includes sites-enabled\n", confPath)
		return nil
	}

	data, err := os.ReadFile(confPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", confPath, err)
	}

	content := string(data)
	if strings.Contains(content, includeLine) || strings.Contains(content, "sites-enabled") {
		return nil
	}

	confDInclude := "include /etc/nginx/conf.d/*.conf;"
	if idx := strings.Index(content, confDInclude); idx != -1 {
		insertPos := idx + len(confDInclude)
		newContent := content[:insertPos] + "\n\t" + includeLine + content[insertPos:]
		return os.WriteFile(confPath, []byte(newContent), 0644)
	}

	httpIdx := strings.Index(content, "http {")
	if httpIdx == -1 {
		return fmt.Errorf("%s: no http block found; cannot add sites-enabled include", confPath)
	}
	insertPos := httpIdx + len("http {")
	newContent := content[:insertPos] + "\n\t" + includeLine + content[insertPos:]
	return os.WriteFile(confPath, []byte(newContent), 0644)
}

func (s *Service) disableDefaultSite() error {
	defaultLink := filepath.Join(s.provider.NginxSitesEnabled(), defaultSiteConfigName)
	if s.dryRun {
		fmt.Printf("[dry-run] would remove %s\n", defaultLink)
		return nil
	}

	if _, err := os.Lstat(defaultLink); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(defaultLink)
}

func (s *Service) disableDefaultConfDSite() error {
	candidates := []string{
		filepath.Join(s.provider.NginxConfigDir(), "default.conf"),
		filepath.Join(s.provider.NginxConfigDir(), "nginx-default.conf"),
	}
	for _, path := range candidates {
		if s.dryRun {
			fmt.Printf("[dry-run] would disable %s if present\n", path)
			continue
		}
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		disabled := path + ".disabled"
		_ = os.Remove(disabled)
		if err := os.Rename(path, disabled); err != nil {
			return err
		}
	}
	return nil
}

// InstallCommand returns the abstrax command to install the given backend.
func InstallCommand(backend string) string {
	switch backend {
	case "apache":
		return "sudo abstrax web install --apache"
	default:
		return "sudo abstrax web install"
	}
}

// Installed reports whether the web server backend binary is available.
func Installed(backend string) bool {
	switch backend {
	case "nginx", "":
		return executil.Exists("nginx")
	case "apache":
		return executil.Exists("apache2") || executil.Exists("httpd")
	default:
		return false
	}
}

// Test tests the nginx or Apache configuration.
func (s *Service) Test(ctx context.Context, backend string) (*TestResult, error) {
	switch backend {
	case "nginx", "":
		res, err := s.runner.RunSilent(ctx, "nginx", "-t")
		return &TestResult{
			OK:      err == nil && res.ExitCode == 0,
			Output:  res.Stdout + res.Stderr,
			Backend: "nginx",
		}, nil
	case "apache":
		return nil, fmt.Errorf("Apache support is not yet implemented")
	default:
		return nil, fmt.Errorf("unknown web server %q", backend)
	}
}

// Reload reloads the web server gracefully.
func (s *Service) Reload(ctx context.Context, backend string) error {
	switch backend {
	case "nginx", "":
		_, err := s.runner.Run(ctx, "nginx", "-s", "reload")
		return err
	case "apache":
		return fmt.Errorf("Apache support is not yet implemented")
	default:
		return fmt.Errorf("unknown web server %q", backend)
	}
}

// Restart restarts the web server.
func (s *Service) Restart(ctx context.Context, backend string) error {
	switch backend {
	case "nginx", "":
		if executil.SystemctlWorks() {
			_, err := s.runner.Run(ctx, "systemctl", "restart", "nginx")
			return err
		}
		if executil.Exists("service") {
			_, err := s.runner.Run(ctx, "service", "nginx", "restart")
			return err
		}
		// Last resort: stop and start nginx directly.
		s.runner.Run(ctx, "nginx", "-s", "stop")
		_, err := s.runner.Run(ctx, "nginx")
		return err
	case "apache":
		return fmt.Errorf("Apache support is not yet implemented")
	default:
		return fmt.Errorf("unknown web server %q", backend)
	}
}
