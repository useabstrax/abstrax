// Package firewall manages the system firewall using UFW.
package firewall

import (
	"context"
	"fmt"
	"strings"

	executil "abstrax/internal/exec"
	"abstrax/internal/services/sshcfg"
)

// Service manages the firewall.
type Service struct {
	runner *executil.Runner
}

// New creates a Service.
func New(dryRun, verbose bool) *Service {
	return &Service{runner: executil.New(dryRun, verbose)}
}

// GetStatus returns the current firewall status.
func (s *Service) GetStatus(ctx context.Context) (*Status, error) {
	if !executil.Exists("ufw") {
		return nil, fmt.Errorf("no supported firewall backend found (ufw not available)")
	}

	res, err := s.runner.RunSilent(ctx, "ufw", "status", "numbered")
	if err != nil {
		return nil, fmt.Errorf("ufw status: %w", err)
	}

	status := &Status{Backend: "ufw"}
	status.Active = strings.Contains(res.Stdout, "Status: active")
	status.Rules = parseUFWRules(res.Stdout)

	return status, nil
}

// Enable enables UFW.
func (s *Service) Enable(ctx context.Context, opts EnableOptions) (SSHProtectResult, error) {
	protect, err := s.ensureClientSSHAllow(ctx)
	if err != nil {
		return protect, err
	}

	if !executil.Exists("ufw") {
		return protect, fmt.Errorf("ufw is not installed")
	}

	sshPort := opts.SSHPort
	if sshPort == 0 {
		sshPort = 22
	}
	if sshPort == 22 {
		if configured, err := sshcfg.SSHPort(); err == nil {
			sshPort = configured
		}
	}

	if opts.AllowSSH {
		if _, err := s.runner.Run(ctx, "ufw", "allow",
			fmt.Sprintf("%d/tcp", sshPort)); err != nil {
			return protect, fmt.Errorf("allowing SSH port: %w", err)
		}
	}

	_, err = s.runner.Run(ctx, "ufw", "--force", "enable")
	return protect, err
}

// Disable disables UFW.
func (s *Service) Disable(ctx context.Context) error {
	if !executil.Exists("ufw") {
		return fmt.Errorf("ufw is not installed")
	}
	_, err := s.runner.Run(ctx, "ufw", "--force", "disable")
	return err
}

// Allow adds an allow rule.
func (s *Service) Allow(ctx context.Context, opts AllowOptions) (SSHProtectResult, error) {
	protect, err := s.ensureClientSSHAllow(ctx)
	if err != nil {
		return protect, err
	}
	return protect, s.addRule(ctx, "allow", opts)
}

// Deny adds a deny rule.
func (s *Service) Deny(ctx context.Context, opts AllowOptions) (SSHProtectResult, error) {
	protect, err := s.ensureClientSSHAllow(ctx)
	if err != nil {
		return protect, err
	}
	return protect, s.addRule(ctx, "deny", opts)
}

// AllowIP allows traffic from an IP or CIDR.
func (s *Service) AllowIP(ctx context.Context, opts AllowOptions) (SSHProtectResult, error) {
	protect, err := s.ensureClientSSHAllow(ctx)
	if err != nil {
		return protect, err
	}
	return protect, s.allowIP(ctx, opts)
}

// DenyIP denies traffic from an IP or CIDR.
func (s *Service) DenyIP(ctx context.Context, opts AllowOptions) (SSHProtectResult, error) {
	protect, err := s.ensureClientSSHAllow(ctx)
	if err != nil {
		return protect, err
	}
	return protect, s.denyIP(ctx, opts)
}

// RuleList returns the current rules.
func (s *Service) RuleList(ctx context.Context) ([]Rule, error) {
	status, err := s.GetStatus(ctx)
	if err != nil {
		return nil, err
	}
	return status.Rules, nil
}

// RuleRemove removes a rule by number.
func (s *Service) RuleRemove(ctx context.Context, id string) error {
	_, err := s.runner.Run(ctx, "ufw", "--force", "delete", id)
	return err
}

func (s *Service) allowIP(ctx context.Context, opts AllowOptions) error {
	args := []string{"allow", "from", opts.From}
	if opts.To != "" {
		args = append(args, "to", opts.To)
	}
	if opts.Port != "" {
		args = append(args, "port", opts.Port)
	}
	_, err := s.runner.Run(ctx, "ufw", args...)
	return err
}

func (s *Service) denyIP(ctx context.Context, opts AllowOptions) error {
	args := []string{"deny", "from", opts.From}
	if opts.To != "" {
		args = append(args, "to", opts.To)
	}
	_, err := s.runner.Run(ctx, "ufw", args...)
	return err
}

func (s *Service) addRule(ctx context.Context, action string, opts AllowOptions) error {
	port := opts.Port
	if opts.Protocol != "" {
		port = fmt.Sprintf("%s/%s", port, opts.Protocol)
	}

	args := []string{action}
	if opts.From != "" {
		args = append(args, "from", opts.From)
		if port != "" {
			args = append(args, "to", "any", "port", port)
		}
	} else if port != "" {
		args = append(args, port)
	} else {
		return fmt.Errorf("must specify a port or IP")
	}

	if opts.Comment != "" {
		args = append(args, "comment", opts.Comment)
	}

	_, err := s.runner.Run(ctx, "ufw", args...)
	return err
}

func parseUFWRules(output string) []Rule {
	var rules []Rule
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "[") {
			continue
		}

		// Example: [ 1] 22/tcp ALLOW IN Anywhere
		// Example: [ 2] 22/tcp ALLOW IN 203.0.113.5
		closeBracket := strings.Index(line, "]")
		if closeBracket < 0 {
			continue
		}

		id := strings.Trim(line[1:closeBracket], " ")
		rest := strings.TrimSpace(line[closeBracket+1:])

		r := Rule{ID: id}
		parts := strings.Fields(rest)
		if len(parts) >= 1 {
			r.Port = parts[0]
		}
		if len(parts) >= 2 {
			r.Action = parts[1]
		}
		if len(parts) >= 4 && strings.EqualFold(parts[2], "IN") {
			from := parts[3]
			if from != "Anywhere" && !strings.HasPrefix(from, "(") {
				r.From = from
			}
		}
		rules = append(rules, r)
	}
	return rules
}
