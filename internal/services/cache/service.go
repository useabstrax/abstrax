// Package cache manages Redis, Valkey, and Memcached cache servers.
package cache

import (
	"context"
	"fmt"
	"os"
	"strings"

	executil "abstrax/internal/exec"
	"abstrax/internal/globals"
	"abstrax/internal/platform"
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

// Install installs a cache driver. The returned driver is the one that was
// installed (Redis may fall back to Valkey on EL10 without Remi).
func (s *Service) Install(ctx context.Context, opts InstallOptions) (Driver, error) {
	if opts.Version != "" {
		return "", fmt.Errorf("cache version pinning is not yet supported; omit --version")
	}

	mgr, _, err := pkgmanager.NewFromPlatform(opts.DryRun, false)
	if err != nil {
		return "", err
	}
	provider := platform.Resolve()

	opts.Driver = maybeFallbackRedisToValkey(provider, opts.Driver)

	pkg, svcName, err := driverNames(provider, opts.Driver)
	if err != nil {
		return "", err
	}

	if opts.Driver == DriverRedis && provider.RequiresExternalRepoForRedis() {
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
		if err := platform.EnsureRedisRepository(ctx, provider, repoOpts, enabler); err != nil {
			return "", err
		}
	}

	if err := mgr.Install(ctx, pkgmanager.InstallOptions{Name: pkg, DryRun: opts.DryRun}); err != nil {
		hint := ""
		if opts.Driver == DriverRedis && provider.RequiresExternalRepoForRedis() && !globals.Flags.EnableRequiredRepos {
			hint = "; for Rocky/Alma 10+ Redis requires Remi - re-run with --enable-required-repos (or run `sudo abstrax repo enable remi --enable-required-repos` first), or install AppStream Valkey with `abstrax cache install valkey`"
		}
		return "", fmt.Errorf("installing %s: %w%s", pkg, err, hint)
	}

	if opts.Enable || opts.Start {
		if err := s.svc.Enable(ctx, svcName); err != nil {
			return "", err
		}
	}
	if opts.Start {
		if err := s.svc.Start(ctx, svcName); err != nil {
			return "", err
		}
	}

	if err := applyDriverConfig(provider, opts); err != nil {
		return "", err
	}

	if !opts.DryRun && (opts.Port > 0 || opts.Bind != "" || opts.Memory != "") {
		if err := s.svc.Restart(ctx, svcName); err != nil {
			return "", fmt.Errorf("restarting %s after config change: %w", svcName, err)
		}
	}

	return opts.Driver, nil
}

// maybeFallbackRedisToValkey switches Redis to AppStream Valkey on platforms
// where Redis needs Remi and --enable-required-repos was not passed.
func maybeFallbackRedisToValkey(provider platform.Provider, driver Driver) Driver {
	if driver != DriverRedis {
		return driver
	}
	if !provider.RequiresExternalRepoForRedis() || globals.Flags.EnableRequiredRepos {
		return driver
	}
	fmt.Fprintln(os.Stderr, "Note: Redis on this OS requires Remi; installing AppStream Valkey instead. Pass --enable-required-repos to install Redis.")
	return DriverValkey
}

// Remove removes a cache driver.
func (s *Service) Remove(ctx context.Context, opts RemoveOptions) error {
	provider := platform.Resolve()
	pkg, _, err := driverNames(provider, opts.Driver)
	if err != nil {
		return err
	}

	mgr, _, err := pkgmanager.NewFromPlatform(opts.DryRun, false)
	if err != nil {
		return err
	}
	return mgr.Remove(ctx, pkgmanager.RemoveOptions{Name: pkg, Purge: opts.Purge})
}

// Start starts a cache driver.
func (s *Service) Start(ctx context.Context, driver Driver) error {
	_, svcName, err := driverNames(platform.Resolve(), driver)
	if err != nil {
		return err
	}
	return s.svc.Start(ctx, svcName)
}

// Stop stops a cache driver.
func (s *Service) Stop(ctx context.Context, driver Driver) error {
	_, svcName, err := driverNames(platform.Resolve(), driver)
	if err != nil {
		return err
	}
	return s.svc.Stop(ctx, svcName)
}

// Restart restarts a cache driver.
func (s *Service) Restart(ctx context.Context, driver Driver) error {
	_, svcName, err := driverNames(platform.Resolve(), driver)
	if err != nil {
		return err
	}
	return s.svc.Restart(ctx, svcName)
}

// Status returns the status of one or all cache drivers.
func (s *Service) Status(ctx context.Context, driver Driver) ([]StatusInfo, error) {
	drivers := []Driver{DriverRedis, DriverValkey, DriverMemcached}
	if driver != "" {
		drivers = []Driver{driver}
	}
	provider := platform.Resolve()

	var statuses []StatusInfo
	for _, d := range drivers {
		_, svcName, err := driverNames(provider, d)
		if err != nil {
			continue
		}

		info := StatusInfo{Driver: d, Version: nil}
		s.fillStatus(ctx, provider, d, &info)
		if info.Installed {
			if st, err := s.svc.Status(ctx, svcName); err == nil {
				info.Running = st.Active == "active"
				info.Enabled = strings.Contains(st.Enabled, "enabled")
			}
		}
		statuses = append(statuses, info)
	}

	return statuses, nil
}

func driverNames(provider platform.Provider, d Driver) (pkg, service string, err error) {
	switch d {
	case DriverRedis:
		return provider.RedisPackage(), provider.RedisServiceName(), nil
	case DriverValkey:
		return provider.ValkeyPackage(), provider.ValkeyServiceName(), nil
	case DriverMemcached:
		return provider.MemcachedPackage(), provider.MemcachedServiceName(), nil
	default:
		return "", "", fmt.Errorf("unsupported cache driver %q; supported: redis, valkey, memcached", d)
	}
}
