// Package svcmanager provides service management using systemd.
package svcmanager

import (
	"context"
	"fmt"
	"strings"

	executil "abstrax/internal/exec"
)

// Service provides systemd service management.
type Service struct {
	runner *executil.Runner
}

// New creates a Service.
func New(dryRun, verbose bool) *Service {
	return &Service{runner: executil.New(dryRun, verbose)}
}

// Start starts a service.
func (s *Service) Start(ctx context.Context, name string) error {
	_, err := s.runner.Run(ctx, "systemctl", "start", name)
	if err != nil {
		return fmt.Errorf("starting service %s: %w", name, err)
	}
	return nil
}

// Stop stops a service.
func (s *Service) Stop(ctx context.Context, name string) error {
	_, err := s.runner.Run(ctx, "systemctl", "stop", name)
	if err != nil {
		return fmt.Errorf("stopping service %s: %w", name, err)
	}
	return nil
}

// Restart restarts a service.
func (s *Service) Restart(ctx context.Context, name string) error {
	_, err := s.runner.Run(ctx, "systemctl", "restart", name)
	if err != nil {
		return fmt.Errorf("restarting service %s: %w", name, err)
	}
	return nil
}

// Reload reloads a service.
func (s *Service) Reload(ctx context.Context, name string) error {
	_, err := s.runner.Run(ctx, "systemctl", "reload", name)
	if err != nil {
		return fmt.Errorf("reloading service %s: %w", name, err)
	}
	return nil
}

// Enable enables a service to start at boot.
func (s *Service) Enable(ctx context.Context, name string) error {
	_, err := s.runner.Run(ctx, "systemctl", "enable", name)
	if err != nil {
		return fmt.Errorf("enabling service %s: %w", name, err)
	}
	return nil
}

// Disable disables a service from starting at boot.
func (s *Service) Disable(ctx context.Context, name string) error {
	_, err := s.runner.Run(ctx, "systemctl", "disable", name)
	if err != nil {
		return fmt.Errorf("disabling service %s: %w", name, err)
	}
	return nil
}

// Status returns the status of a service.
func (s *Service) Status(ctx context.Context, name string) (*ServiceStatus, error) {
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
