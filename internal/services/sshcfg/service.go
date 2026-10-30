// Package sshcfg manages SSH server configuration safely.
package sshcfg

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	executil "abstrax/internal/exec"
)

const (
	sshdConfigPath = "/etc/ssh/sshd_config"
)

// Service manages sshd configuration.
type Service struct {
	runner             *executil.Runner
	dryRun             bool
	managedPath        string
	mainConfigPath     string
	socketDropIn       string
	openFirewallFn     func(ctx context.Context, port int) error
	closeFirewallFn    func(ctx context.Context, port int) error
	selinuxEnforcingFn func() bool
	labelFn            func(ctx context.Context, port int) error
	socketActiveFn     func(ctx context.Context) bool
	validateFn         func(ctx context.Context) error
	reloadFn           func(ctx context.Context) error
	restartSocketFn    func(ctx context.Context) error
	bannerFn           func(port int) error
}

// New creates a Service.
func New(dryRun, verbose bool) *Service {
	return &Service{
		runner: executil.New(dryRun, verbose),
		dryRun: dryRun,
	}
}

// SetTimeout sets the ClientAliveInterval.
func (s *Service) SetTimeout(ctx context.Context, opts SetTimeoutOptions) error {
	return s.applyDirective(ctx, "ClientAliveInterval", fmt.Sprintf("%d", opts.Seconds), opts.DryRun)
}

// DisableRootLogin sets PermitRootLogin to no.
func (s *Service) DisableRootLogin(ctx context.Context, dryRun bool) error {
	return s.applyDirective(ctx, "PermitRootLogin", "no", dryRun)
}

// EnableRootLogin sets PermitRootLogin to yes.
func (s *Service) EnableRootLogin(ctx context.Context, dryRun bool) error {
	return s.applyDirective(ctx, "PermitRootLogin", "yes", dryRun)
}

// DisablePasswordAuth sets PasswordAuthentication to no.
func (s *Service) DisablePasswordAuth(ctx context.Context, dryRun bool) error {
	return s.applyDirective(ctx, "PasswordAuthentication", "no", dryRun)
}

// EnablePasswordAuth sets PasswordAuthentication to yes.
func (s *Service) EnablePasswordAuth(ctx context.Context, dryRun bool) error {
	return s.applyDirective(ctx, "PasswordAuthentication", "yes", dryRun)
}

// Reload reloads sshd.
func (s *Service) Reload(ctx context.Context, opts ReloadOptions) error {
	return s.sshAction(ctx, "reload")
}

// Restart restarts sshd.
func (s *Service) Restart(ctx context.Context, opts ReloadOptions) error {
	return s.sshAction(ctx, "restart")
}

// sshAction runs a lifecycle action against the SSH service, trying systemctl
// first and falling back to the service command.  Both "ssh" and "sshd" names
// are tried because distributions differ.
func (s *Service) sshAction(ctx context.Context, action string) error {
	if executil.SystemctlWorks() {
		_, err := s.runner.Run(ctx, "systemctl", action, "ssh")
		if err != nil {
			_, err = s.runner.Run(ctx, "systemctl", action, "sshd")
		}
		return err
	}
	if executil.Exists("service") {
		_, err := s.runner.Run(ctx, "service", "ssh", action)
		if err != nil {
			_, err = s.runner.Run(ctx, "service", "sshd", action)
		}
		return err
	}
	return fmt.Errorf("no supported init system found (need systemctl or service)")
}

func (s *Service) validateAndReload(ctx context.Context) error {
	if s.validateFn != nil {
		if err := s.validateFn(ctx); err != nil {
			return err
		}
	} else if executil.Exists("sshd") {
		res, err := s.runner.RunSilent(ctx, "sshd", "-t")
		if err != nil || res.ExitCode != 0 {
			return fmt.Errorf("sshd config validation failed: %s", res.Stderr)
		}
	}
	if s.reloadFn != nil {
		return s.reloadFn(ctx)
	}
	return s.Reload(ctx, ReloadOptions{})
}

func readConfigFile(path string) ([]ConfigEntry, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []ConfigEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 {
			entries = append(entries, ConfigEntry{
				Key:   parts[0],
				Value: strings.TrimSpace(parts[1]),
			})
		}
	}
	return entries, scanner.Err()
}

func writeConfigFile(path string, entries []ConfigEntry) error {
	var sb strings.Builder
	sb.WriteString("# Managed by Abstrax - do not edit manually\n")
	for _, e := range entries {
		sb.WriteString(fmt.Sprintf("%s %s\n", e.Key, e.Value))
	}
	return os.WriteFile(path, []byte(sb.String()), 0644)
}
