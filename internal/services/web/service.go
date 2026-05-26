// Package web manages web servers (nginx initially; Apache is a stub).
package web

import (
	"context"
	"fmt"

	executil "abstrax/internal/exec"
)

// Service manages web servers.
type Service struct {
	runner *executil.Runner
}

// New creates a Service.
func New(dryRun, verbose bool) *Service {
	return &Service{runner: executil.New(dryRun, verbose)}
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
