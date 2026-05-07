// Package cache manages Redis and Memcached cache servers.
package cache

import (
	"context"
	"fmt"
	"strings"

	executil "abstrax/internal/exec"
	"abstrax/internal/services/pkgmanager"
	"abstrax/internal/services/svcmanager"
)

// Service manages cache drivers.
type Service struct {
	runner *executil.Runner
	svc    *svcmanager.Service
}

// New creates a Service.
func New(dryRun, verbose bool) *Service {
	return &Service{
		runner: executil.New(dryRun, verbose),
		svc:    svcmanager.New(dryRun, verbose),
	}
}

// Install installs a cache driver.
func (s *Service) Install(ctx context.Context, opts InstallOptions) error {
	pkg := string(opts.Driver)
	switch opts.Driver {
	case DriverRedis:
		pkg = "redis-server"
	case DriverMemcached:
		pkg = "memcached"
	default:
		return fmt.Errorf("unsupported cache driver %q; supported: redis, memcached", opts.Driver)
	}

	mgr := pkgmanager.NewApt(false, false)
	if err := mgr.Install(ctx, pkgmanager.InstallOptions{Name: pkg}); err != nil {
		return fmt.Errorf("installing %s: %w", pkg, err)
	}

	if opts.Enable || opts.Start {
		if err := s.svc.Enable(ctx, pkg); err != nil {
			return err
		}
	}
	if opts.Start {
		if err := s.svc.Start(ctx, pkg); err != nil {
			return err
		}
	}

	return nil
}

// Remove removes a cache driver.
func (s *Service) Remove(ctx context.Context, opts RemoveOptions) error {
	pkg := packageName(opts.Driver)
	if pkg == "" {
		return fmt.Errorf("unsupported cache driver %q", opts.Driver)
	}

	mgr := pkgmanager.NewApt(false, false)
	return mgr.Remove(ctx, pkgmanager.RemoveOptions{Name: pkg, Purge: opts.Purge})
}

// Start starts a cache driver.
func (s *Service) Start(ctx context.Context, driver Driver) error {
	pkg := packageName(driver)
	if pkg == "" {
		return fmt.Errorf("unsupported cache driver %q", driver)
	}
	return s.svc.Start(ctx, pkg)
}

// Stop stops a cache driver.
func (s *Service) Stop(ctx context.Context, driver Driver) error {
	pkg := packageName(driver)
	if pkg == "" {
		return fmt.Errorf("unsupported cache driver %q", driver)
	}
	return s.svc.Stop(ctx, pkg)
}

// Restart restarts a cache driver.
func (s *Service) Restart(ctx context.Context, driver Driver) error {
	pkg := packageName(driver)
	if pkg == "" {
		return fmt.Errorf("unsupported cache driver %q", driver)
	}
	return s.svc.Restart(ctx, pkg)
}

// Status returns the status of one or all cache drivers.
func (s *Service) Status(ctx context.Context, driver Driver) ([]StatusInfo, error) {
	drivers := []Driver{DriverRedis, DriverMemcached}
	if driver != "" {
		drivers = []Driver{driver}
	}

	var statuses []StatusInfo
	for _, d := range drivers {
		pkg := packageName(d)
		if pkg == "" {
			continue
		}

		info := StatusInfo{Driver: d}
		if st, err := s.svc.Status(ctx, pkg); err == nil {
			info.Running = st.Active == "active"
			info.Enabled = strings.Contains(st.Enabled, "enabled")
		}
		statuses = append(statuses, info)
	}

	return statuses, nil
}

// Config shows basic configuration for a cache driver.
func (s *Service) Config(_ context.Context, driver Driver) (string, error) {
	switch driver {
	case DriverRedis:
		return "Redis config file: /etc/redis/redis.conf\nTODO: structured config management", nil
	case DriverMemcached:
		return "Memcached config file: /etc/memcached.conf\nTODO: structured config management", nil
	default:
		return "", fmt.Errorf("unsupported cache driver %q", driver)
	}
}

func packageName(d Driver) string {
	switch d {
	case DriverRedis:
		return "redis-server"
	case DriverMemcached:
		return "memcached"
	default:
		return ""
	}
}
