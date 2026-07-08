package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"abstrax/internal/services/svcmanager"
)

const (
	maxUnixSocketPathLen = 104
	phpPoolPrefix        = "abstrax-"
)

// PHPPoolConfig describes a generated PHP-FPM pool.
type PHPPoolConfig struct {
	PoolName   string
	ConfigPath string
	SocketPath string
}

// generatePHPPoolName returns a deterministic, PHP-FPM-safe pool name.
func generatePHPPoolName(projectName string) string {
	var b strings.Builder
	b.WriteString(phpPoolPrefix)
	for _, r := range strings.ToLower(projectName) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
		case r == '-', r == '_', r == '.':
			b.WriteRune('-')
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == phpPoolPrefix {
		name = phpPoolPrefix + "project"
	}
	return name
}

// buildPHPPoolPaths returns pool and socket paths for a project.
func buildPHPPoolPaths(projectName, phpVersion string) PHPPoolConfig {
	poolName := generatePHPPoolName(projectName)
	poolName = truncatePoolNameForSocket(poolName, phpVersion)
	suffix := strings.TrimPrefix(poolName, phpPoolPrefix)
	socketPath := debianPlatform.PHPFPMProjectSocket(phpVersion, suffix)
	configPath := filepath.Join(debianPlatform.PHPFPMPoolDir(phpVersion), poolName+".conf")
	return PHPPoolConfig{
		PoolName:   poolName,
		ConfigPath: configPath,
		SocketPath: socketPath,
	}
}

func truncatePoolNameForSocket(poolName, phpVersion string) string {
	suffix := strings.TrimPrefix(poolName, phpPoolPrefix)
	socket := debianPlatform.PHPFPMProjectSocket(phpVersion, suffix)
	if len(socket) <= maxUnixSocketPathLen {
		return poolName
	}
	over := len(socket) - maxUnixSocketPathLen
	if over >= len(suffix) {
		suffix = suffix[:8]
	} else {
		suffix = suffix[:len(suffix)-over]
	}
	return phpPoolPrefix + suffix
}

func renderPHPPool(conf PHPPoolConfig, id RuntimeIdentity) string {
	var sb strings.Builder
	sb.WriteString("; Managed by Abstrax\n")
	sb.WriteString(fmt.Sprintf("[%s]\n\n", conf.PoolName))
	sb.WriteString(fmt.Sprintf("user = %s\n", id.User))
	sb.WriteString(fmt.Sprintf("group = %s\n\n", id.Group))
	sb.WriteString(fmt.Sprintf("listen = %s\n", conf.SocketPath))
	sb.WriteString(fmt.Sprintf("listen.owner = %s\n", id.User))
	sb.WriteString(fmt.Sprintf("listen.group = %s\n", id.WebServerUser))
	sb.WriteString("listen.mode = 0660\n\n")
	sb.WriteString("pm = ondemand\n")
	sb.WriteString("pm.max_children = 5\n")
	sb.WriteString("pm.process_idle_timeout = 10s\n")
	return sb.String()
}

func (s *Service) createPHPPool(ctx context.Context, state *State, id RuntimeIdentity) (*PHPPoolConfig, error) {
	if state.Runtime != RuntimePHP || id.Mode != OwnershipIsolated {
		return nil, nil
	}

	conf := buildPHPPoolPaths(state.Name, normalizePHPVersion(state.PHPVersion))
	if _, err := os.Stat(conf.ConfigPath); err == nil {
		return nil, fmt.Errorf("php-fpm pool config %q already exists", conf.ConfigPath)
	}

	content := renderPHPPool(conf, id)
	if err := os.MkdirAll(filepath.Dir(conf.ConfigPath), 0755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(conf.ConfigPath, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("writing php-fpm pool: %w", err)
	}

	fpmBin := debianPlatform.PHPFPMBinary(normalizePHPVersion(state.PHPVersion))
	if res, err := s.runner.RunSilent(ctx, fpmBin, "--test"); err != nil || res.ExitCode != 0 {
		_ = os.Remove(conf.ConfigPath)
		msg := res.Stderr
		if msg == "" {
			msg = res.Stdout
		}
		return nil, fmt.Errorf("php-fpm configuration test failed: %s", msg)
	}

	svc := svcmanager.New(s.dryRun, false)
	if err := svc.Reload(ctx, debianPlatform.PHPFPMServiceName(normalizePHPVersion(state.PHPVersion))); err != nil {
		_ = os.Remove(conf.ConfigPath)
		return nil, fmt.Errorf("reloading php-fpm: %w", err)
	}

	state.PHPPoolName = conf.PoolName
	state.PHPSocketPath = conf.SocketPath
	return &conf, nil
}

func (s *Service) removePHPPool(ctx context.Context, state *State) error {
	if state.PHPPoolName == "" {
		return nil
	}
	_ = os.Remove(state.PHPPoolPath())
	if state.PHPVersion != "" {
		svc := svcmanager.New(s.dryRun, false)
		_ = svc.Reload(ctx, debianPlatform.PHPFPMServiceName(normalizePHPVersion(state.PHPVersion)))
	}
	_ = os.Remove(state.PHPSocketPath)
	return nil
}

func (st *State) PHPPoolPath() string {
	if st.PHPPoolName == "" || st.PHPVersion == "" {
		return ""
	}
	return filepath.Join(debianPlatform.PHPFPMPoolDir(normalizePHPVersion(st.PHPVersion)), st.PHPPoolName+".conf")
}

func phpSocketForState(state *State) string {
	if state.PHPSocketPath != "" {
		return state.PHPSocketPath
	}
	if state.PHPVersion == "" {
		return ""
	}
	return debianPlatform.PHPFPMDefaultSocket(normalizePHPVersion(state.PHPVersion))
}

func (s *Service) reloadPHPFPM(ctx context.Context, phpVersion string) error {
	svc := svcmanager.New(s.dryRun, false)
	return svc.Reload(ctx, debianPlatform.PHPFPMServiceName(normalizePHPVersion(phpVersion)))
}
