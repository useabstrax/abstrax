// Package sshcfg manages SSH server configuration safely.
package sshcfg

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	executil "abstrax/internal/exec"
	"abstrax/internal/platform/debian"
)

const (
	sshdConfigPath = "/etc/ssh/sshd_config"
)

// Service manages sshd configuration.
type Service struct {
	runner *executil.Runner
}

// New creates a Service.
func New(dryRun, verbose bool) *Service {
	return &Service{runner: executil.New(dryRun, verbose)}
}

// Show returns the current Abstrax-managed SSH config values.
func (s *Service) Show(_ context.Context) (*SSHConfig, error) {
	cfg := &SSHConfig{}

	entries, err := s.readManaged()
	if err != nil {
		// Fall back to reading main config.
		entries, err = readConfigFile(sshdConfigPath)
		if err != nil {
			return nil, err
		}
	}

	for _, e := range entries {
		switch strings.ToLower(e.Key) {
		case "port":
			cfg.Port = e.Value
		case "permitrootlogin":
			cfg.PermitRootLogin = e.Value
		case "passwordauthentication":
			cfg.PasswordAuth = e.Value
		case "clientaliveinterval":
			cfg.ClientAliveInterval = e.Value
		}
	}

	return cfg, nil
}

// SetPort changes the SSH listening port.
func (s *Service) SetPort(ctx context.Context, opts SetPortOptions) error {
	if err := s.writeDirective(ctx, "Port", fmt.Sprintf("%d", opts.Port)); err != nil {
		return err
	}
	return s.validateAndReload(ctx)
}

// SetTimeout sets the ClientAliveInterval.
func (s *Service) SetTimeout(ctx context.Context, opts SetTimeoutOptions) error {
	if err := s.writeDirective(ctx, "ClientAliveInterval", fmt.Sprintf("%d", opts.Seconds)); err != nil {
		return err
	}
	return s.validateAndReload(ctx)
}

// DisableRootLogin sets PermitRootLogin to no.
func (s *Service) DisableRootLogin(ctx context.Context, dryRun bool) error {
	if err := s.writeDirective(ctx, "PermitRootLogin", "no"); err != nil {
		return err
	}
	return s.validateAndReload(ctx)
}

// EnableRootLogin sets PermitRootLogin to yes.
func (s *Service) EnableRootLogin(ctx context.Context, dryRun bool) error {
	if err := s.writeDirective(ctx, "PermitRootLogin", "yes"); err != nil {
		return err
	}
	return s.validateAndReload(ctx)
}

// DisablePasswordAuth sets PasswordAuthentication to no.
func (s *Service) DisablePasswordAuth(ctx context.Context, dryRun bool) error {
	if err := s.writeDirective(ctx, "PasswordAuthentication", "no"); err != nil {
		return err
	}
	return s.validateAndReload(ctx)
}

// EnablePasswordAuth sets PasswordAuthentication to yes.
func (s *Service) EnablePasswordAuth(ctx context.Context, dryRun bool) error {
	if err := s.writeDirective(ctx, "PasswordAuthentication", "yes"); err != nil {
		return err
	}
	return s.validateAndReload(ctx)
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

// writeDirective writes or updates a directive in the managed include file.
func (s *Service) writeDirective(_ context.Context, key, value string) error {
	managed := debian.AbstraxSSHConfig

	// Ensure include directory exists.
	if _, err := os.Stat(debian.SSHConfigDir); os.IsNotExist(err) {
		if err := os.MkdirAll(debian.SSHConfigDir, 0755); err != nil {
			return fmt.Errorf("creating sshd_config.d: %w", err)
		}
	}

	// Read existing entries.
	entries, _ := s.readManaged()

	// Update or add the directive.
	found := false
	for i, e := range entries {
		if strings.EqualFold(e.Key, key) {
			entries[i].Value = value
			found = true
			break
		}
	}
	if !found {
		entries = append(entries, ConfigEntry{Key: key, Value: value})
	}

	return writeConfigFile(managed, entries)
}

func (s *Service) readManaged() ([]ConfigEntry, error) {
	return readConfigFile(debian.AbstraxSSHConfig)
}

func (s *Service) validateAndReload(ctx context.Context) error {
	if executil.Exists("sshd") {
		res, err := s.runner.RunSilent(ctx, "sshd", "-t")
		if err != nil || res.ExitCode != 0 {
			return fmt.Errorf("sshd config validation failed: %s", res.Stderr)
		}
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
