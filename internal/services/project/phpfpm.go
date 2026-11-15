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
	// sharedPHPPoolName is the Abstrax-managed pool used by shared-web-user
	// PHP projects on RHEL-family systems (Remi's default www pool only
	// allows apache to connect, while nginx runs as nginx).
	sharedPHPPoolName   = "abstrax-web"
	sharedPHPPoolSuffix = "web"
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
	socketPath := platformProvider.PHPFPMProjectSocket(phpVersion, suffix)
	configPath := filepath.Join(platformProvider.PHPFPMPoolDir(phpVersion), poolName+".conf")
	return PHPPoolConfig{
		PoolName:   poolName,
		ConfigPath: configPath,
		SocketPath: socketPath,
	}
}

func truncatePoolNameForSocket(poolName, phpVersion string) string {
	suffix := strings.TrimPrefix(poolName, phpPoolPrefix)
	socket := platformProvider.PHPFPMProjectSocket(phpVersion, suffix)
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

	if err := s.writeAndActivatePHPPool(ctx, conf, id, state.PHPVersion); err != nil {
		return nil, err
	}

	state.PHPPoolName = conf.PoolName
	state.PHPSocketPath = conf.SocketPath
	return &conf, nil
}

// needsManagedSharedPHPPool reports whether shared-web-user PHP projects need an
// Abstrax-managed pool instead of the distro default www socket.
func needsManagedSharedPHPPool() bool {
	return platformProvider.Profile().IsRHELFamily()
}

// sharedPHPPoolPaths returns the per-PHP-version Abstrax shared pool paths.
func sharedPHPPoolPaths(phpVersion string) PHPPoolConfig {
	v := normalizePHPVersion(phpVersion)
	return PHPPoolConfig{
		PoolName:   sharedPHPPoolName,
		ConfigPath: filepath.Join(platformProvider.PHPFPMPoolDir(v), sharedPHPPoolName+".conf"),
		SocketPath: platformProvider.PHPFPMProjectSocket(v, sharedPHPPoolSuffix),
	}
}

// ensureSharedPHPPool creates the Abstrax-managed shared web pool for a PHP
// version when missing. Remi's www.conf is left alone. The pool is shared
// across projects, so project remove does not delete it.
func (s *Service) ensureSharedPHPPool(ctx context.Context, phpVersion string, id RuntimeIdentity) (*PHPPoolConfig, error) {
	if !needsManagedSharedPHPPool() {
		return nil, nil
	}
	conf := sharedPHPPoolPaths(phpVersion)
	if _, err := os.Stat(conf.ConfigPath); err == nil {
		return &conf, nil
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	if err := s.writeAndActivatePHPPool(ctx, conf, id, phpVersion); err != nil {
		return nil, err
	}
	return &conf, nil
}

func (s *Service) writeAndActivatePHPPool(ctx context.Context, conf PHPPoolConfig, id RuntimeIdentity, phpVersion string) error {
	content := renderPHPPool(conf, id)
	if err := os.MkdirAll(filepath.Dir(conf.ConfigPath), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(conf.ConfigPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing php-fpm pool: %w", err)
	}

	fpmBin := platformProvider.PHPFPMBinary(normalizePHPVersion(phpVersion))
	if res, err := s.runner.RunSilent(ctx, fpmBin, "--test"); err != nil || res.ExitCode != 0 {
		_ = os.Remove(conf.ConfigPath)
		msg := res.Stderr
		if msg == "" {
			msg = res.Stdout
		}
		return fmt.Errorf("php-fpm configuration test failed: %s", msg)
	}

	if err := s.reloadPHPFPM(ctx, phpVersion); err != nil {
		_ = os.Remove(conf.ConfigPath)
		return fmt.Errorf("reloading php-fpm: %w", err)
	}
	return nil
}

func (s *Service) removePHPPool(ctx context.Context, state *State) error {
	if state.PHPPoolName == "" {
		return nil
	}
	// Never remove the shared abstrax-web pool; other projects may still use it.
	if state.PHPPoolName == sharedPHPPoolName {
		return nil
	}
	_ = os.Remove(state.PHPPoolPath())
	if state.PHPVersion != "" {
		_ = s.reloadPHPFPM(ctx, state.PHPVersion)
	}
	_ = os.Remove(state.PHPSocketPath)
	return nil
}

func (st *State) PHPPoolPath() string {
	if st.PHPPoolName == "" || st.PHPVersion == "" {
		return ""
	}
	return filepath.Join(platformProvider.PHPFPMPoolDir(normalizePHPVersion(st.PHPVersion)), st.PHPPoolName+".conf")
}

func phpSocketForState(state *State) string {
	if state.PHPSocketPath != "" {
		return state.PHPSocketPath
	}
	if state.PHPVersion == "" {
		return ""
	}
	v := normalizePHPVersion(state.PHPVersion)
	if needsManagedSharedPHPPool() && state.PHPPoolName == "" {
		mode := state.OwnershipMode
		if mode == "" || mode == OwnershipShared {
			return sharedPHPPoolPaths(v).SocketPath
		}
	}
	return platformProvider.PHPFPMDefaultSocket(v)
}

func (s *Service) reloadPHPFPM(ctx context.Context, phpVersion string) error {
	if s.phpfpmReloader != nil {
		return s.phpfpmReloader(ctx, normalizePHPVersion(phpVersion))
	}
	svc := svcmanager.New(s.dryRun, false)
	return svc.Reload(ctx, platformProvider.PHPFPMServiceName(normalizePHPVersion(phpVersion)))
}
