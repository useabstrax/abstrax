// Package svcmanager provides service management using systemctl with a
// fallback to the traditional "service" command when systemd is unavailable.
package svcmanager

import (
	"context"
	"fmt"
	"strings"

	executil "abstrax/internal/exec"
)

// Service provides system service management.
type Service struct {
	runner *executil.Runner
}

// New creates a Service.
func New(dryRun, verbose bool) *Service {
	return &Service{runner: executil.New(dryRun, verbose)}
}

// Start starts a service.
func (s *Service) Start(ctx context.Context, name string) error {
	if err := s.runAction(ctx, "start", name); err != nil {
		return fmt.Errorf("starting service %s: %w", name, err)
	}
	return nil
}

// Stop stops a service.
func (s *Service) Stop(ctx context.Context, name string) error {
	if err := s.runAction(ctx, "stop", name); err != nil {
		return fmt.Errorf("stopping service %s: %w", name, err)
	}
	return nil
}

// Restart restarts a service.
func (s *Service) Restart(ctx context.Context, name string) error {
	if err := s.runAction(ctx, "restart", name); err != nil {
		return fmt.Errorf("restarting service %s: %w", name, err)
	}
	return nil
}

// Reload reloads a service.
func (s *Service) Reload(ctx context.Context, name string) error {
	if err := s.runAction(ctx, "reload", name); err != nil {
		return fmt.Errorf("reloading service %s: %w", name, err)
	}
	return nil
}

// Enable enables a service to start at boot.
func (s *Service) Enable(ctx context.Context, name string) error {
	if executil.SystemctlWorks() {
		_, err := s.runner.Run(ctx, "systemctl", "enable", name)
		if err != nil {
			return fmt.Errorf("enabling service %s: %w", name, err)
		}
		return nil
	}
	if executil.Exists("update-rc.d") {
		_, err := s.runner.Run(ctx, "update-rc.d", name, "defaults")
		if err != nil {
			return fmt.Errorf("enabling service %s: %w", name, err)
		}
		return nil
	}
	return fmt.Errorf("enabling service %s: no supported init system found", name)
}

// Disable disables a service from starting at boot.
func (s *Service) Disable(ctx context.Context, name string) error {
	if executil.SystemctlWorks() {
		_, err := s.runner.Run(ctx, "systemctl", "disable", name)
		if err != nil {
			return fmt.Errorf("disabling service %s: %w", name, err)
		}
		return nil
	}
	if executil.Exists("update-rc.d") {
		_, err := s.runner.Run(ctx, "update-rc.d", name, "disable")
		if err != nil {
			return fmt.Errorf("disabling service %s: %w", name, err)
		}
		return nil
	}
	return fmt.Errorf("disabling service %s: no supported init system found", name)
}

// Status returns the status of a service.
func (s *Service) Status(ctx context.Context, name string) (*ServiceStatus, error) {
	if executil.SystemctlWorks() {
		return s.statusSystemctl(ctx, name)
	}
	return s.statusService(ctx, name)
}

func (s *Service) statusSystemctl(ctx context.Context, name string) (*ServiceStatus, error) {
	res, _ := s.runner.RunSilent(ctx, "systemctl", "show", name,
		"--property=ActiveState,SubState,Description,MainPID,UnitFileState")

	status := &ServiceStatus{Name: name}
	for _, line := range strings.Split(res.Stdout, "\n") {
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "ActiveState":
			status.Active = kv[1]
		case "SubState":
			status.Sub = kv[1]
		case "Description":
			status.Description = kv[1]
		case "MainPID":
			status.PID = kv[1]
		case "UnitFileState":
			status.Enabled = kv[1]
		}
	}
	return status, nil
}

func (s *Service) statusService(ctx context.Context, name string) (*ServiceStatus, error) {
	res, err := s.runner.RunSilent(ctx, "service", name, "status")

	status := &ServiceStatus{Name: name}
	output := res.Stdout + " " + res.Stderr
	lower := strings.ToLower(output)

	if err != nil || res.ExitCode != 0 {
		status.Active = "inactive"
		status.Sub = "dead"
		if strings.Contains(lower, "not running") || strings.Contains(lower, "stopped") {
			status.Active = "inactive"
			status.Sub = "dead"
		}
	} else {
		status.Active = "active"
		status.Sub = "running"
	}

	if strings.Contains(lower, "running") {
		status.Active = "active"
		status.Sub = "running"
	}

	return status, nil
}

// runAction executes a service lifecycle action (start/stop/restart/reload),
// preferring systemctl and falling back to the service command.
func (s *Service) runAction(ctx context.Context, action, name string) error {
	if executil.SystemctlWorks() {
		_, err := s.runner.Run(ctx, "systemctl", action, name)
		return err
	}
	if executil.Exists("service") {
		_, err := s.runner.Run(ctx, "service", name, action)
		return err
	}
	return fmt.Errorf("no supported init system found (need systemctl or service)")
}
