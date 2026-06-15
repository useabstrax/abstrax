package project

import (
	"context"
	"fmt"
)

// RestartService restarts a project-owned supervisor service.
func (s *Service) RestartService(ctx context.Context, projectName, serviceName string) error {
	daemonName, err := s.ResolveProjectDaemon(ctx, projectName, serviceName)
	if err != nil {
		return err
	}
	if _, err := s.runner.Run(ctx, "supervisorctl", "restart", daemonName); err != nil {
		return fmt.Errorf("restarting service %q: %w", serviceName, err)
	}
	return nil
}

// ReloadService sends HUP to a project-owned supervisor service.
func (s *Service) ReloadService(ctx context.Context, projectName, serviceName string) error {
	daemonName, err := s.ResolveProjectDaemon(ctx, projectName, serviceName)
	if err != nil {
		return err
	}
	if _, err := s.runner.Run(ctx, "supervisorctl", "signal", "HUP", daemonName); err != nil {
		return fmt.Errorf("reloading service %q: %w", serviceName, err)
	}
	return nil
}
