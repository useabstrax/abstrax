// Package ssl manages SSL certificates using Certbot.
package ssl

import (
	"context"
	"fmt"
	"strings"

	executil "abstrax/internal/exec"
	"abstrax/internal/services/project"
)

// Service manages SSL certificates.
type Service struct {
	runner      *executil.Runner
	projectsSvc *project.Service
}

// New creates a Service.
func New(dryRun, verbose bool) *Service {
	return &Service{
		runner:      executil.New(dryRun, verbose),
		projectsSvc: project.New(dryRun, verbose),
	}
}

// Add obtains and configures SSL for a project.
func (s *Service) Add(ctx context.Context, opts AddOptions) error {
	if !executil.Exists("certbot") {
		return fmt.Errorf("certbot is not installed; install it with: abstrax package install certbot")
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

	args := []string{
		"--nginx",
		"--non-interactive",
		"--agree-tos",
		"--email", opts.Email,
	}

	for _, d := range domains {
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

	_, err = s.runner.Run(ctx, "certbot", append([]string{"certonly"}, args...)...)
	return err
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

	_, err = s.runner.Run(ctx, "certbot", "delete",
		"--cert-name", proj.Domains[0],
		"--non-interactive")
	return err
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

	// Very basic parsing – certbot output is human-readable.
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
