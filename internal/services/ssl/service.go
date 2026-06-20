// Package ssl manages SSL certificates using Certbot.
package ssl

import (
	"context"
	"fmt"
	"strings"
	"time"

	executil "abstrax/internal/exec"
	"abstrax/internal/services/project"
)

// Service manages SSL certificates.
type Service struct {
	runner      *executil.Runner
	projectsSvc *project.Service
	dryRun      bool
}

// New creates a Service.
func New(dryRun, verbose bool) *Service {
	return &Service{
		runner:      executil.New(dryRun, verbose),
		projectsSvc: project.New(dryRun, verbose),
		dryRun:      dryRun,
	}
}

// Add obtains and configures SSL for a project.
func (s *Service) Add(ctx context.Context, opts AddOptions) error {
	if !Installed() {
		return fmt.Errorf("certbot is not installed; install it with: %s", InstallCommand())
	}

	proj, err := s.projectsSvc.Info(ctx, opts.ProjectName)
	if err != nil {
		return fmt.Errorf("project %q not found: %w", opts.ProjectName, err)
	}

	domains := opts.Domains
	if len(domains) == 0 {
		domains = proj.Domains
	}
	if len(domains) == 0 {
		return fmt.Errorf("no domains configured for project %q", opts.ProjectName)
	}

	if opts.Email == "" {
		return fmt.Errorf("--email is required for SSL certificate issuance")
	}

	args := buildCertbotAddArgs(AddOptions{
		Email:        opts.Email,
		Domains:      domains,
		Staging:      opts.Staging,
		RedirectHTTP: opts.RedirectHTTP,
	})

	res, err := s.runner.Run(ctx, "certbot", args...)
	if err != nil {
		if res.Stderr != "" {
			return fmt.Errorf("certbot failed: %s", strings.TrimSpace(res.Stderr))
		}
		return fmt.Errorf("certbot failed: %w", err)
	}

	state, err := s.projectsSvc.Info(ctx, opts.ProjectName)
	if err != nil {
		return fmt.Errorf("project %q not found: %w", opts.ProjectName, err)
	}
	state.SSLEnabled = true
	state.UpdatedAt = time.Now()
	if err := s.projectsSvc.SaveState(state); err != nil {
		return fmt.Errorf("updating project SSL state: %w", err)
	}

	return nil
}

// Remove removes SSL certificates for a project.
func (s *Service) Remove(ctx context.Context, projectName string) error {
	proj, err := s.projectsSvc.Info(ctx, projectName)
	if err != nil {
		return fmt.Errorf("project %q not found: %w", projectName, err)
	}

	if len(proj.Domains) == 0 {
		return fmt.Errorf("no domains configured for project %q", projectName)
	}

	res, err := s.runner.Run(ctx, "certbot", "delete",
		"--cert-name", proj.Domains[0],
		"--non-interactive")
	if err != nil {
		if res.Stderr != "" {
			return fmt.Errorf("certbot failed: %s", strings.TrimSpace(res.Stderr))
		}
		return fmt.Errorf("certbot failed: %w", err)
	}

	state, err := s.projectsSvc.Info(ctx, projectName)
	if err != nil {
		return fmt.Errorf("project %q not found: %w", projectName, err)
	}
	state.SSLEnabled = false
	state.UpdatedAt = time.Now()
	if err := s.projectsSvc.SaveState(state); err != nil {
		return fmt.Errorf("updating project SSL state: %w", err)
	}

	return nil
}

// Renew renews certificates.
func (s *Service) Renew(ctx context.Context, opts RenewOptions) error {
	args := []string{"renew"}
	if opts.DryRun {
		args = append(args, "--dry-run")
	}
	if opts.Project != "" {
		proj, err := s.projectsSvc.Info(ctx, opts.Project)
		if err != nil {
			return fmt.Errorf("project %q not found: %w", opts.Project, err)
		}
		if len(proj.Domains) > 0 {
			args = append(args, "--cert-name", proj.Domains[0])
		}
	}
	_, err := s.runner.Run(ctx, "certbot", args...)
	return err
}

// Status returns SSL certificate status for a project or all projects.
func (s *Service) Status(ctx context.Context, projectName string) ([]CertStatus, error) {
	res, err := s.runner.RunSilent(ctx, "certbot", "certificates")
	if err != nil {
		return nil, fmt.Errorf("certbot certificates: %w", err)
	}

	// Very basic parsing - certbot output is human-readable.
	var statuses []CertStatus
	var current *CertStatus
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Certificate Name:") {
			if current != nil {
				statuses = append(statuses, *current)
			}
			name := strings.TrimPrefix(line, "Certificate Name:")
			current = &CertStatus{ProjectName: strings.TrimSpace(name)}
		} else if current != nil && strings.HasPrefix(line, "Domains:") {
			raw := strings.TrimPrefix(line, "Domains:")
			for _, d := range strings.Fields(raw) {
				current.Domains = append(current.Domains, d)
			}
		} else if current != nil && strings.HasPrefix(line, "Expiry Date:") {
			current.Expiry = strings.TrimSpace(strings.TrimPrefix(line, "Expiry Date:"))
		}
	}
	if current != nil {
		statuses = append(statuses, *current)
	}

	if projectName != "" {
		var filtered []CertStatus
		for _, cs := range statuses {
			if cs.ProjectName == projectName {
				filtered = append(filtered, cs)
			}
		}
		return filtered, nil
	}

	return statuses, nil
}

func buildCertbotAddArgs(opts AddOptions) []string {
	args := []string{
		"--nginx",
		"--non-interactive",
		"--agree-tos",
		"--email", opts.Email,
	}

	for _, d := range opts.Domains {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		args = append(args, "-d", d)
	}

	if opts.Staging {
		args = append(args, "--staging")
	}

	if opts.RedirectHTTP {
		args = append(args, "--redirect")
	} else {
		args = append(args, "--no-redirect")
	}

	return args
}
