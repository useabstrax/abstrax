// Package project manages web application projects using nginx.
package project

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"abstrax/internal/backup"
	executil "abstrax/internal/exec"
	"abstrax/internal/identity"
	"abstrax/internal/platform/debian"
	"abstrax/internal/services/config"
	"abstrax/internal/services/web"
)

// Service manages projects.
type Service struct {
	runner       *executil.Runner
	dryRun       bool
	identity     identity.Resolver
	stateDir     string
	nginxAvail   string
	nginxEnabled string
}

// New creates a Service.
func New(dryRun, verbose bool) *Service {
	runner := executil.New(dryRun, verbose)
	svc := &Service{
		runner:       runner,
		dryRun:       dryRun,
		identity:     identity.NewOSResolver(runner),
		stateDir:     debian.AbstraxProjectsDir,
		nginxAvail:   debian.NginxSitesAvailable,
		nginxEnabled: debian.NginxSitesEnabled,
	}
	_ = svc.migrateLegacyProjects()
	return svc
}

// SetIdentityResolver overrides the identity resolver (for tests).
func (s *Service) SetIdentityResolver(resolver identity.Resolver) {
	s.identity = resolver
}

// Add creates a new project and its web server configuration.
func (s *Service) Add(ctx context.Context, opts AddOptions) (*State, error) {
	if opts.WebServer == WebServerApache {
		return nil, fmt.Errorf("Apache support is not yet implemented")
	}

	if opts.WebServer != WebServerNone && !opts.NoVhost && !web.Installed(string(opts.WebServer)) {
		return nil, fmt.Errorf("%s is not installed; install it first with: %s",
			opts.WebServer, web.InstallCommand(string(opts.WebServer)))
	}

	if _, err := s.loadState(opts.Name); err == nil {
		return nil, fmt.Errorf("project %q already exists", opts.Name)
	}

	id, err := ResolveIdentity(ctx, s.identity, opts)
	if err != nil {
		return nil, err
	}

	projectPath, err := ResolveProjectPath(opts.Name, opts.Path, id)
	if err != nil {
		return nil, err
	}

	approvedRoots, err := loadApprovedRoots()
	if err != nil {
		return nil, err
	}

	homes, err := s.identity.ListHomes(ctx)
	if err != nil {
		return nil, err
	}

	validated, err := ValidateProjectPath(PathValidateOptions{
		RequestedPath: projectPath,
		ProjectName:   opts.Name,
		PublicDir:     opts.PublicDir,
		WebRoot:       opts.WebRoot,
		Identity:      id,
		ApprovedRoots: approvedRoots,
		Homes:         homes,
	})
	if err != nil {
		return nil, err
	}

	rb := &addRollback{svc: s}

	switch opts.Runtime {
	case RuntimePHP:
		opts.PHPVersion = normalizePHPVersion(opts.PHPVersion)
	case RuntimeNode:
		opts.NodeVersion = normalizeNodeVersion(opts.NodeVersion)
	case RuntimeRuby:
		opts.RubyVersion = normalizeRubyVersion(opts.RubyVersion)
	}

	if err := s.ensureRuntime(ctx, runtimeSpecFromAdd(opts), opts.Yes, opts.DryRun); err != nil {
		return nil, err
	}

	mkdirResult, err := mkdirProjectTree(validated, id, 0755)
	if err != nil {
		return nil, err
	}
	rb.trackDirs(mkdirResult.Created)

	opts.Path = validated.ProjectPath

	state := &State{
		Name:          opts.Name,
		Path:          validated.ProjectPath,
		Domains:       opts.Domains,
		WebServer:     opts.WebServer,
		Runtime:       opts.Runtime,
		Owner:         id.User,
		Group:         id.Group,
		OwnershipMode: id.Mode,
		OwnerUID:      id.UID,
		OwnerGID:      id.GID,
		OwnerHome:     id.Home,
		ApprovedRoot:  validated.ApprovedRoot,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	switch opts.Runtime {
	case RuntimePHP:
		state.PHPVersion = opts.PHPVersion
		state.PublicDir = opts.PublicDir
	case RuntimeNode:
		state.NodeVersion = opts.NodeVersion
		state.ProxyPort = opts.ProxyPort
	case RuntimeRuby:
		state.RubyVersion = opts.RubyVersion
		state.ProxyPort = opts.ProxyPort
	}

	if id.Mode == OwnershipIsolated {
		if err := ensureWebTraverseAccess(validated, id); err != nil {
			rb.undo(ctx)
			return nil, fmt.Errorf("configuring nginx filesystem access: %w", err)
		}
	}

	if state.Runtime == RuntimePHP && id.Mode == OwnershipIsolated {
		pool, err := s.createPHPPool(ctx, state, id)
		if err != nil {
			rb.undo(ctx)
			return nil, fmt.Errorf("creating php-fpm pool: %w", err)
		}
		if pool != nil {
			rb.trackPool(pool.ConfigPath)
		}
	}

	vhostOpts := vhostOptionsFromAdd(opts, state, validated)
	if opts.WebServer == WebServerNginx && !opts.NoVhost {
		vhostPath, err := s.createNginxVhost(ctx, vhostOpts)
		if err != nil {
			rb.undo(ctx)
			return nil, fmt.Errorf("creating nginx vhost: %w", err)
		}
		state.VhostPath = vhostPath
		rb.trackVhost(vhostPath)
	}

	if err := s.saveState(state); err != nil {
		rb.undo(ctx)
		return nil, fmt.Errorf("saving project state: %w", err)
	}

	for _, warning := range CheckSecurityWarnings(state.Path) {
		fmt.Println(formatWarnings([]SecurityWarning{warning}))
	}

	return state, nil
}

// Remove removes a project and optionally its web server configuration.
func (s *Service) Remove(ctx context.Context, opts RemoveOptions) error {
	state, err := s.loadState(opts.Name)
	if err != nil {
		return fmt.Errorf("project %q not found", opts.Name)
	}

	id := IdentityFromState(state)

	if state.VhostPath != "" && opts.RemoveVhost {
		enabledLink := filepath.Join(s.nginxEnabled, filepath.Base(state.VhostPath))
		_ = os.Remove(enabledLink)

		if _, err := backup.File(state.VhostPath); err != nil {
			return err
		}
		_ = os.Remove(state.VhostPath)
		_, _ = s.runner.Run(ctx, "nginx", "-s", "reload")
	}

	if id.Mode == OwnershipIsolated {
		if err := s.removePHPPool(ctx, state); err != nil {
			return err
		}
	}

	if opts.DeleteFiles && !opts.KeepFiles {
		if err := os.RemoveAll(state.Path); err != nil {
			return fmt.Errorf("deleting project files: %w", err)
		}
	}

	return s.deleteState(opts.Name)
}

// Modify updates project configuration.
func (s *Service) Modify(ctx context.Context, opts ModifyOptions) (*State, error) {
	state, err := s.loadState(opts.Name)
	if err != nil {
		return nil, fmt.Errorf("project %q not found", opts.Name)
	}

	if err := s.ensureRuntime(ctx, runtimeSpecFromState(state, opts), opts.Yes, opts.DryRun); err != nil {
		return nil, err
	}

	if opts.Path != "" {
		state.Path = opts.Path
	}
	if len(opts.Domains) > 0 {
		state.Domains = opts.Domains
	}
	if opts.AddDomain != "" {
		state.Domains = append(state.Domains, opts.AddDomain)
	}
	if opts.RemoveDomain != "" {
		var domains []string
		for _, d := range state.Domains {
			if d != opts.RemoveDomain {
				domains = append(domains, d)
			}
		}
		state.Domains = domains
	}
	if opts.Runtime != "" {
		state.Runtime = opts.Runtime
	}
	if opts.PHPVersion != "" {
		state.PHPVersion = normalizePHPVersion(opts.PHPVersion)
	}
	if opts.NodeVersion != "" {
		state.NodeVersion = normalizeNodeVersion(opts.NodeVersion)
	}
	if opts.RubyVersion != "" {
		state.RubyVersion = normalizeRubyVersion(opts.RubyVersion)
	}
	if opts.PublicDir != "" {
		state.PublicDir = opts.PublicDir
	}
	if opts.ProxyPort != 0 {
		state.ProxyPort = opts.ProxyPort
	}
	state.UpdatedAt = time.Now()

	if state.VhostPath != "" && state.WebServer == WebServerNginx {
		if _, err := backup.File(state.VhostPath); err != nil {
			return nil, err
		}
		if _, err := s.createNginxVhost(ctx, state.vhostConfig()); err != nil {
			return nil, err
		}
	}

	if err := s.saveState(state); err != nil {
		return nil, err
	}
	return state, nil
}

// List returns all projects.
func (s *Service) List(_ context.Context) ([]State, error) {
	entries, err := os.ReadDir(s.stateDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var projects []State
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		state, err := s.loadState(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil {
			continue
		}
		projects = append(projects, *state)
	}
	return projects, nil
}

// Info returns a single project.
func (s *Service) Info(_ context.Context, name string) (*State, error) {
	return s.loadState(name)
}

// Enable enables the nginx vhost for a project.
func (s *Service) Enable(ctx context.Context, name string) error {
	state, err := s.loadState(name)
	if err != nil {
		return fmt.Errorf("project %q not found", name)
	}
	if state.VhostPath == "" {
		return fmt.Errorf("project %q has no vhost", name)
	}
	link := filepath.Join(s.nginxEnabled, filepath.Base(state.VhostPath))
	if err := os.Symlink(state.VhostPath, link); err != nil && !os.IsExist(err) {
		return fmt.Errorf("enabling vhost: %w", err)
	}
	_, err = s.runner.Run(ctx, "nginx", "-s", "reload")
	return err
}

// Disable disables the nginx vhost for a project.
func (s *Service) Disable(ctx context.Context, name string) error {
	state, err := s.loadState(name)
	if err != nil {
		return fmt.Errorf("project %q not found", name)
	}
	if state.VhostPath == "" {
		return fmt.Errorf("project %q has no vhost", name)
	}
	link := filepath.Join(s.nginxEnabled, filepath.Base(state.VhostPath))
	if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("disabling vhost: %w", err)
	}
	_, err = s.runner.Run(ctx, "nginx", "-s", "reload")
	return err
}

// Reload reloads nginx for this project.
func (s *Service) Reload(ctx context.Context, name string) error {
	if _, err := s.loadState(name); err != nil {
		return fmt.Errorf("project %q not found", name)
	}
	if res, err := s.runner.RunSilent(ctx, "nginx", "-t"); err != nil || res.ExitCode != 0 {
		return fmt.Errorf("nginx config test failed: %s", res.Stderr)
	}
	_, err := s.runner.Run(ctx, "nginx", "-s", "reload")
	return err
}

func (s *Service) createNginxVhost(ctx context.Context, opts vhostConfig) (string, error) {
	if err := os.MkdirAll(s.nginxAvail, 0755); err != nil {
		return "", err
	}

	confName := "abstrax-" + opts.Name
	vhostPath := filepath.Join(s.nginxAvail, confName)

	conf := buildNginxConfig(opts)

	if err := os.WriteFile(vhostPath, []byte(conf), 0644); err != nil {
		return "", fmt.Errorf("writing nginx config: %w", err)
	}

	// Validate.
	if res, err := s.runner.RunSilent(ctx, "nginx", "-t"); err != nil || res.ExitCode != 0 {
		_ = os.Remove(vhostPath)
		return "", fmt.Errorf("nginx config validation failed: %s", res.Stderr)
	}

	// Enable.
	if err := os.MkdirAll(s.nginxEnabled, 0755); err != nil {
		return "", err
	}
	link := filepath.Join(s.nginxEnabled, confName)
	_ = os.Remove(link)
	if err := os.Symlink(vhostPath, link); err != nil {
		return "", fmt.Errorf("enabling nginx vhost: %w", err)
	}

	_, _ = s.runner.Run(ctx, "nginx", "-s", "reload")

	return vhostPath, nil
}

func buildNginxConfig(opts vhostConfig) string {
	domains := strings.Join(opts.Domains, " ")
	if domains == "" {
		domains = "_"
	}

	root := opts.Path
	if opts.WebRoot != "" {
		root = opts.WebRoot
	}
	if opts.PublicDir != "" {
		root = filepath.Join(opts.Path, opts.PublicDir)
	}

	var sb strings.Builder
	sb.WriteString("# Managed by Abstrax\n")
	sb.WriteString("server {\n")
	sb.WriteString(fmt.Sprintf("    listen 80;\n"))
	sb.WriteString(fmt.Sprintf("    server_name %s;\n", domains))
	sb.WriteString(fmt.Sprintf("    root %s;\n", root))
	sb.WriteString("    index index.html index.htm index.php;\n\n")

	switch opts.Runtime {
	case RuntimePHP:
		socket := opts.PHPSocket
		if socket == "" {
			socket = filepath.Join("/run/php", fmt.Sprintf("php%s-fpm.sock", normalizePHPVersion(opts.PHPVersion)))
		}
		sb.WriteString("    location / {\n")
		sb.WriteString("        try_files $uri $uri/ /index.php?$query_string;\n")
		sb.WriteString("    }\n\n")
		sb.WriteString("    location ~ \\.php$ {\n")
		sb.WriteString("        include snippets/fastcgi-php.conf;\n")
		sb.WriteString(fmt.Sprintf("        fastcgi_pass unix:%s;\n", socket))
		sb.WriteString("    }\n")
	case RuntimeNode, RuntimeRuby:
		proxyPort := opts.ProxyPort
		if proxyPort == 0 {
			proxyPort = opts.NodePort
		}
		if proxyPort == 0 {
			proxyPort = 3000
		}
		sb.WriteString("    location / {\n")
		sb.WriteString(fmt.Sprintf("        proxy_pass http://127.0.0.1:%d;\n", proxyPort))
		sb.WriteString("        proxy_http_version 1.1;\n")
		sb.WriteString("        proxy_set_header Upgrade $http_upgrade;\n")
		sb.WriteString("        proxy_set_header Connection 'upgrade';\n")
		sb.WriteString("        proxy_set_header Host $host;\n")
		sb.WriteString("        proxy_cache_bypass $http_upgrade;\n")
		sb.WriteString("    }\n")
	default:
		sb.WriteString("    location / {\n")
		sb.WriteString("        try_files $uri $uri/ =404;\n")
		sb.WriteString("    }\n")
	}

	sb.WriteString("}\n")
	return sb.String()
}

func loadApprovedRoots() ([]string, error) {
	cfg := config.New()
	settings, err := cfg.Effective()
	if err != nil {
		return nil, err
	}
	if settings.Projects == nil {
		return nil, nil
	}
	return settings.Projects.ApprovedRoots, nil
}

func (s *Service) statePath(name string) string {
	return filepath.Join(s.stateDir, name+".json")
}

func (s *Service) loadState(name string) (*State, error) {
	data, err := os.ReadFile(s.statePath(name))
	if err != nil {
		return nil, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func (s *Service) saveState(state *State) error {
	if err := os.MkdirAll(s.stateDir, 0750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.statePath(state.Name), data, 0640)
}

func (s *Service) deleteState(name string) error {
	return os.Remove(s.statePath(name))
}

func (s *Service) migrateLegacyProjects() error {
	return migrateProjects(s.stateDir, debian.AbstraxProjectsDirLegacy)
}

func migrateProjects(newDir, legacyDir string) error {
	if legacyDir == newDir {
		return nil
	}

	entries, err := os.ReadDir(legacyDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	if err := os.MkdirAll(newDir, 0750); err != nil {
		return err
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		src := filepath.Join(legacyDir, e.Name())
		dst := filepath.Join(newDir, e.Name())
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("migrating project %q: %w", e.Name(), err)
		}
	}

	remaining, err := os.ReadDir(legacyDir)
	if err != nil {
		return nil
	}
	if len(remaining) == 0 {
		_ = os.Remove(legacyDir)
	}
	return nil
}
