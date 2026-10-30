package sshcfg

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

const sshdUnavailableNote = "sshd -T is unavailable; values were read from config files"

// Show returns the SSH settings sshd is using.
func (s *Service) Show(ctx context.Context) (*SSHConfig, error) {
	cfg, err := s.showFromSSHD(ctx)
	if err == nil {
		return s.completeShow(cfg), nil
	}
	cfg, ferr := s.showFromFiles()
	if ferr != nil {
		return nil, err
	}
	cfg.Note = sshdUnavailableNote
	return cfg, nil
}

func (s *Service) showFromSSHD(ctx context.Context) (*SSHConfig, error) {
	if s.runner == nil {
		return nil, fmt.Errorf("sshd -T is unavailable")
	}
	res, err := s.runner.RunSilent(ctx, "sshd", "-T")
	if err != nil || res.ExitCode != 0 || strings.TrimSpace(res.Stdout) == "" {
		detail := strings.TrimSpace(res.Stderr)
		if detail == "" && err != nil {
			detail = err.Error()
		}
		if detail == "" {
			detail = "sshd -T failed"
		}
		return nil, fmt.Errorf("sshd -T failed: %s", detail)
	}
	return parseSSHDDump(res.Stdout), nil
}

func parseSSHDDump(out string) *SSHConfig {
	cfg := &SSHConfig{}
	var ports []string
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := normalizeKeyword(fields[0])
		value := strings.Join(fields[1:], " ")
		switch key {
		case "port":
			ports = append(ports, value)
		case "permitrootlogin", "passwordauthentication", "clientaliveinterval":
			if seen[key] {
				continue
			}
			seen[key] = true
			switch key {
			case "permitrootlogin":
				cfg.PermitRootLogin = value
			case "passwordauthentication":
				cfg.PasswordAuth = value
			case "clientaliveinterval":
				cfg.ClientAliveInterval = value
			}
		}
	}
	cfg.Port = strings.Join(ports, ", ")
	return cfg
}

func (s *Service) showFromFiles() (*SSHConfig, error) {
	vals := map[string]string{}
	var ports []string
	err := s.walkDirectives(func(_ string, key, value string) {
		if strings.EqualFold(key, "port") {
			ports = append(ports, value)
		}
		k := normalizeKeyword(key)
		if _, seen := vals[k]; seen {
			return
		}
		vals[k] = value
	})
	if err != nil {
		return nil, err
	}
	cfg := &SSHConfig{
		Port:                strings.Join(ports, ", "),
		PermitRootLogin:     vals["permitrootlogin"],
		PasswordAuth:        vals["passwordauthentication"],
		ClientAliveInterval: vals["clientaliveinterval"],
	}
	return s.completeShow(cfg), nil
}

func (s *Service) completeShow(cfg *SSHConfig) *SSHConfig {
	if cfg.Port == "" {
		cfg.Port = sshDefault("port")
	}
	if cfg.PermitRootLogin == "" {
		cfg.PermitRootLogin = sshDefault("permitrootlogin")
	}
	if cfg.PasswordAuth == "" {
		cfg.PasswordAuth = sshDefault("passwordauthentication")
	}
	if cfg.ClientAliveInterval == "" {
		cfg.ClientAliveInterval = sshDefault("clientaliveinterval")
	}
	s.attachSources(cfg)
	return cfg
}

func (s *Service) attachSources(cfg *SSHConfig) {
	cfg.PortSource = s.sourceFor("Port", cfg.Port)
	cfg.PermitRootLoginSource = s.sourceFor("PermitRootLogin", cfg.PermitRootLogin)
	cfg.PasswordAuthSource = s.sourceFor("PasswordAuthentication", cfg.PasswordAuth)
	cfg.ClientAliveIntervalSource = s.sourceFor("ClientAliveInterval", cfg.ClientAliveInterval)
}

func (s *Service) sourceFor(key, effective string) string {
	hits := s.settingsFor(key)
	if strings.EqualFold(key, "port") {
		if len(hits) == 0 {
			return "default"
		}
		return s.sourceLabel(hits[0].path)
	}
	for _, hit := range hits {
		if strings.EqualFold(hit.value, effective) {
			return s.sourceLabel(hit.path)
		}
	}
	if len(hits) == 0 || strings.EqualFold(effective, sshDefault(key)) {
		return "default"
	}
	return s.sourceLabel(hits[0].path)
}

func (s *Service) sourceLabel(path string) string {
	base := filepath.Base(path)
	if base == managedDropInName || base == legacyManagedFile || filepath.Clean(path) == filepath.Clean(s.managedConfigPath()) {
		return "abstrax"
	}
	return path
}

func sshDefault(key string) string {
	switch normalizeKeyword(key) {
	case "port":
		return "22"
	case "permitrootlogin":
		return "prohibit-password"
	case "passwordauthentication":
		return "yes"
	case "clientaliveinterval":
		return "0"
	default:
		return ""
	}
}
