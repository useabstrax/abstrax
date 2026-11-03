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

	executil "abstrax/internal/exec"
	"abstrax/internal/identity"
	"abstrax/internal/platform"
	"abstrax/internal/services/config"
	"abstrax/internal/services/web"
)

// Service manages projects.
type Service struct {
	runner       *executil.Runner
	dryRun       bool
	identity     identity.Resolver
	provider     platform.Provider
	stateDir     string
	nginxLayout  platform.NginxLayout
	nginxAvail   string
	nginxEnabled string
	nginxConfDir string
	// nginxLogDir is the directory for per-project access and error logs.
	// The default is /var/log/nginx. Tests set this to a temporary directory.
	nginxLogDir string
	// letsEncryptDir is the certbot config directory. Empty means /etc/letsencrypt.
	letsEncryptDir string
	// nginxInstalled reports whether nginx is installed. Nil uses web.Installed.
	// Tests set this so they do not depend on the host.
	nginxInstalled func() bool
	// phpfpmReloader reloads the PHP-FPM service for a version. Nil uses the
	// system service manager. Tests set it to avoid touching real services.
	phpfpmReloader func(ctx context.Context, phpVersion string) error
	// runtimeInstalled overrides the runtime availability check. Nil uses the
	// real package and binary checks. Tests set it.
	runtimeInstalled func(RuntimeSpec) bool
}

// New creates a Service.
func New(dryRun, verbose bool) *Service {
	runner := executil.New(dryRun, verbose)
	provider := platform.Resolve()
	paths := provider.Paths()
	svc := &Service{
		runner:       runner,
		dryRun:       dryRun,
		identity:     identity.NewOSResolver(runner),
		provider:     provider,
		stateDir:     paths.AbstraxProjectsDir,
		nginxLayout:  provider.NginxLayout(),
		nginxAvail:   provider.NginxSitesAvailable(),
		nginxEnabled: provider.NginxSitesEnabled(),
		nginxConfDir: provider.NginxConfigDir(),
		nginxLogDir:  defaultNginxLogDir,
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
	report := func(step, message string) {
		if opts.Progress != nil {
			opts.Progress(step, message)
		}
	}

	report("validate", "Validating options")
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

	report("identity", "Resolving project ownership")
	id, err := ResolveIdentity(ctx, s.identity, opts)
	if err != nil {
		return nil, err
	}

	report("path", "Resolving project path")
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

	report("runtime", fmt.Sprintf("Checking runtime availability (%s)", opts.Runtime))
	if err := s.ensureRuntime(ctx, runtimeSpecFromAdd(opts), opts.Yes, opts.DryRun, opts.Progress); err != nil {
		return nil, err
	}

	report("directories", "Creating project directories")
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
		Port:          opts.Port,
		WebRoot:       opts.WebRoot,
		WebServer:     opts.WebServer,
		Runtime:       opts.Runtime,
		Owner:         id.User,
		Group:         id.Group,
		SharedLogs:    opts.SharedLogs,
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
		report("ownership", "Configuring ownership and nginx filesystem access")
		if err := ensureWebTraverseAccess(validated, id); err != nil {
			rb.undo(ctx)
			return nil, fmt.Errorf("configuring nginx filesystem access: %w", err)
		}
	} else {
		report("ownership", "Using shared web user ownership")
	}

	if state.Runtime == RuntimePHP && id.Mode == OwnershipIsolated {
		report("php_pool", fmt.Sprintf("Creating PHP %s FPM pool", state.PHPVersion))
		pool, err := s.createPHPPool(ctx, state, id)
		if err != nil {
			rb.undo(ctx)
			return nil, fmt.Errorf("creating php-fpm pool: %w", err)
		}
		if pool != nil {
			rb.trackPool(pool.ConfigPath)
		}
	} else if state.Runtime == RuntimePHP {
		report("php_pool", fmt.Sprintf("Using shared PHP %s FPM socket", state.PHPVersion))
	}

	vhostOpts := vhostOptionsFromAdd(opts, state, validated)
	if opts.WebServer == WebServerNginx && !opts.NoVhost {
		report("nginx_vhost", "Creating nginx virtual host")
		vhostPath, err := s.createNginxVhost(ctx, vhostOpts)
		if err != nil {
			rb.undo(ctx)
			return nil, fmt.Errorf("creating nginx vhost: %w", err)
		}
		state.VhostPath = vhostPath
		rb.trackVhost(vhostPath)
	}

	report("state", "Saving project state")
	if err := s.saveState(state); err != nil {
		rb.undo(ctx)
		return nil, fmt.Errorf("saving project state: %w", err)
	}
	s.applyLogPaths(state)

	if opts.Progress == nil {
		for _, warning := range CheckSecurityWarnings(state.Path) {
			fmt.Println(formatWarnings([]SecurityWarning{warning}))
		}
		if note := platform.SELinuxWarning(s.provider.Profile().SELinuxStatus, "project paths, nginx config, or PHP-FPM settings"); note != "" {
			fmt.Println("Security note: " + note)
		}
	}

	report("complete", "Project created successfully")
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
		_ = s.disableVhostFile(state.VhostPath)
		_ = os.Remove(state.VhostPath)
		_ = os.Remove(disabledVhostPath(state.VhostPath))
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
		if err := s.removeProjectLogs(state.Name); err != nil {
			return err
		}
	}

	return s.deleteState(opts.Name)
}

// Modify updates project configuration.
//
// Fields are applied from opts.Changed, not from zero-value checks, so a
// changed flag with an empty value clears the stored field. See applyModify.
func (s *Service) Modify(ctx context.Context, opts ModifyOptions) (*State, error) {
	state, err := s.loadState(opts.Name)
	if err != nil {
		return nil, fmt.Errorf("project %q not found", opts.Name)
	}

	before := state.vhostConfig()
	if err := s.applyModify(ctx, state, opts); err != nil {
		return nil, err
	}

	if modifyTouchesRuntime(state, opts) {
		if err := s.ensureRuntime(ctx, runtimeSpecFromState(state), opts.Yes, opts.DryRun, nil); err != nil {
			return nil, err
		}
	}
	state.UpdatedAt = time.Now()

	// Projects set up before Abstrax stored SSL metadata keep their HTTPS
	// listener when the vhost is rewritten.
	s.AdoptSSL(state)

	if state.VhostPath != "" && state.WebServer == WebServerNginx && !vhostConfigEqual(before, state.vhostConfig()) {
		// RenderVhost keeps a disabled vhost disabled and applies TLS from state.
		if err := s.RenderVhost(ctx, state); err != nil {
			return nil, err
		}
	}

	if err := s.saveState(state); err != nil {
		return nil, err
	}
	s.applyLogPaths(state)
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
	state, err := s.loadState(name)
	if err != nil {
		return nil, err
	}
	s.applyLogPaths(state)
	return state, nil
}

// SaveState persists project state.
func (s *Service) SaveState(state *State) error {
	return s.saveState(state)
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
	if err := s.enableVhostFile(state.VhostPath); err != nil {
		return fmt.Errorf("enabling vhost: %w", err)
	}
	state.VhostDisabled = false
	state.UpdatedAt = time.Now()
	if err := s.saveState(state); err != nil {
		return fmt.Errorf("saving project state: %w", err)
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
	if err := s.disableVhostFile(state.VhostPath); err != nil {
		return fmt.Errorf("disabling vhost: %w", err)
	}
	state.VhostDisabled = true
	state.UpdatedAt = time.Now()
	if err := s.saveState(state); err != nil {
		return fmt.Errorf("saving project state: %w", err)
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
	return s.writeNginxVhost(ctx, opts, false)
}

// nginxVhostFilePath returns the path of the managed vhost file for a project
// and creates the directory that holds it.
func (s *Service) nginxVhostFilePath(name string) (string, error) {
	confName := "abstrax-" + name
	if s.nginxLayout == platform.NginxConfD {
		if err := os.MkdirAll(s.nginxConfDir, 0755); err != nil {
			return "", err
		}
		return s.provider.NginxSiteConfigPath(confName), nil
	}
	if err := os.MkdirAll(s.nginxAvail, 0755); err != nil {
		return "", err
	}
	return filepath.Join(s.nginxAvail, confName), nil
}

// vhostSnapshot records a vhost's files so a failed rewrite can be undone.
type vhostSnapshot struct {
	path        string
	active      []byte
	hasActive   bool
	disabled    []byte
	hasDisabled bool
	linked      bool
}

func (s *Service) snapshotVhost(vhostPath string) vhostSnapshot {
	snap := vhostSnapshot{path: vhostPath}
	if data, err := os.ReadFile(vhostPath); err == nil {
		snap.active, snap.hasActive = data, true
	}
	if s.nginxLayout == platform.NginxConfD {
		if data, err := os.ReadFile(disabledVhostPath(vhostPath)); err == nil {
			snap.disabled, snap.hasDisabled = data, true
		}
		return snap
	}
	link := filepath.Join(s.nginxEnabled, filepath.Base(vhostPath))
	if _, err := os.Lstat(link); err == nil {
		snap.linked = true
	}
	return snap
}

func (s *Service) restoreVhost(snap vhostSnapshot) {
	if snap.hasActive {
		_ = os.WriteFile(snap.path, snap.active, 0644)
	} else {
		_ = os.Remove(snap.path)
	}
	if s.nginxLayout == platform.NginxConfD {
		disabled := disabledVhostPath(snap.path)
		if snap.hasDisabled {
			_ = os.WriteFile(disabled, snap.disabled, 0644)
		} else {
			_ = os.Remove(disabled)
		}
		return
	}
	if snap.linked {
		_ = s.enableVhostFile(snap.path)
	} else {
		_ = s.disableVhostFile(snap.path)
	}
}

// writeNginxVhost writes the vhost file, validates it with nginx -t, and then
// leaves it enabled, or disabled when keepDisabled is set. A failed validation
// restores the previous files.
func (s *Service) writeNginxVhost(ctx context.Context, opts vhostConfig, keepDisabled bool) (string, error) {
	vhostPath, err := s.nginxVhostFilePath(opts.Name)
	if err != nil {
		return "", err
	}

	conf, err := s.renderNginxConfig(opts)
	if err != nil {
		return "", err
	}

	snap := s.snapshotVhost(vhostPath)

	if err := os.WriteFile(vhostPath, []byte(conf), 0644); err != nil {
		s.restoreVhost(snap)
		return "", fmt.Errorf("writing nginx config: %w", err)
	}
	if s.nginxLayout == platform.NginxConfD {
		// The fresh file supersedes a disabled copy. Leaving both would let a
		// later enable rename the stale copy over this one.
		_ = os.Remove(disabledVhostPath(vhostPath))
	}

	// Enable before validating so nginx -t reads the new file.
	if err := s.enableVhostFile(vhostPath); err != nil {
		s.restoreVhost(snap)
		return "", fmt.Errorf("enabling nginx vhost: %w", err)
	}

	if res, err := s.runner.RunSilent(ctx, "nginx", "-t"); err != nil || res.ExitCode != 0 {
		s.restoreVhost(snap)
		return "", fmt.Errorf("nginx config validation failed: %s", res.Stderr)
	}

	if keepDisabled {
		if err := s.disableVhostFile(vhostPath); err != nil {
			s.restoreVhost(snap)
			return "", fmt.Errorf("disabling nginx vhost: %w", err)
		}
		// Nothing live changed, so nginx needs no reload.
		return vhostPath, nil
	}

	_, _ = s.runner.Run(ctx, "nginx", "-s", "reload")

	return vhostPath, nil
}

func disabledVhostPath(vhostPath string) string {
	return vhostPath + ".disabled"
}

func (s *Service) enableVhostFile(vhostPath string) error {
	if s.nginxLayout == platform.NginxConfD {
		disabled := disabledVhostPath(vhostPath)
		if _, err := os.Stat(disabled); err == nil {
			if err := os.Rename(disabled, vhostPath); err != nil {
				return err
			}
		}
		// Files present in conf.d are enabled by default.
		return nil
	}

	if err := os.MkdirAll(s.nginxEnabled, 0755); err != nil {
		return err
	}
	link := filepath.Join(s.nginxEnabled, filepath.Base(vhostPath))
	_ = os.Remove(link)
	return os.Symlink(vhostPath, link)
}

func (s *Service) disableVhostFile(vhostPath string) error {
	if s.nginxLayout == platform.NginxConfD {
		disabled := disabledVhostPath(vhostPath)
		if _, err := os.Stat(vhostPath); os.IsNotExist(err) {
			return nil
		}
		_ = os.Remove(disabled)
		return os.Rename(vhostPath, disabled)
	}

	link := filepath.Join(s.nginxEnabled, filepath.Base(vhostPath))
	if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

const defaultNginxLogDir = "/var/log/nginx"

func (s *Service) nginxLogDirectory() string {
	if s != nil && s.nginxLogDir != "" {
		return s.nginxLogDir
	}
	return defaultNginxLogDir
}

func (s *Service) renderNginxConfig(opts vhostConfig) (string, error) {
	if opts.LogDir == "" {
		opts.LogDir = s.nginxLogDirectory()
	}
	s.tlsAssets(&opts)
	return buildNginxConfig(opts)
}

func (s *Service) applyLogPaths(state *State) {
	if state == nil {
		return
	}
	state.AccessLogPath = ""
	state.ErrorLogPath = ""
	if state.SharedLogs {
		return
	}
	accessPath, errorPath, err := projectLogPaths(s.nginxLogDirectory(), state.Name)
	if err != nil {
		return
	}
	state.AccessLogPath = accessPath
	state.ErrorLogPath = errorPath
}

func (s *Service) removeProjectLogs(name string) error {
	accessPath, errorPath, err := projectLogPaths(s.nginxLogDirectory(), name)
	if err != nil {
		// The name cannot be mapped onto a path inside the log directory.
		return nil
	}
	for _, path := range []string{accessPath, errorPath} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("deleting nginx log %s: %w", path, err)
		}
	}
	return nil
}

func projectLogPaths(logDir, name string) (string, string, error) {
	if err := validateProjectLogName(name); err != nil {
		return "", "", err
	}
	if logDir == "" {
		logDir = defaultNginxLogDir
	}
	logDir = filepath.Clean(logDir)
	accessPath := filepath.Join(logDir, name+"-access.log")
	errorPath := filepath.Join(logDir, name+"-error.log")
	if !pathInsideDir(logDir, accessPath) || !pathInsideDir(logDir, errorPath) {
		return "", "", fmt.Errorf("nginx log path for project %q escapes %s", name, logDir)
	}
	return accessPath, errorPath, nil
}

func validateProjectLogName(name string) error {
	if name == "" || name == "." || strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, `\`) || filepath.Base(name) != name {
		return fmt.Errorf("invalid project name %q for nginx log path", name)
	}
	return nil
}

func pathInsideDir(dir, target string) bool {
	rel, err := filepath.Rel(dir, target)
	if err != nil || rel == "." || rel == ".." {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func buildNginxConfig(opts vhostConfig) (string, error) {
	domains := strings.Join(opts.Domains, " ")
	if domains == "" {
		domains = "_"
	}

	listenPort := opts.Port
	if listenPort == 0 {
		listenPort = 80
	}

	root := opts.Path
	if opts.WebRoot != "" {
		root = opts.WebRoot
	}
	if opts.PublicDir != "" {
		root = filepath.Join(opts.Path, opts.PublicDir)
	}

	var logLines string
	if !opts.SharedLogs {
		accessPath, errorPath, err := projectLogPaths(opts.LogDir, opts.Name)
		if err != nil {
			return "", err
		}
		logLines = fmt.Sprintf("    access_log %s;\n    error_log %s;\n", accessPath, errorPath)
	}

	if opts.SSL != nil && !ValidCertName(opts.SSL.CertificateName) {
		return "", fmt.Errorf("invalid SSL certificate name %q for project %q", opts.SSL.CertificateName, opts.Name)
	}

	var sb strings.Builder
	sb.WriteString("# Managed by Abstrax\n")

	// Plain HTTP server. With SSL and RedirectHTTP it only redirects; the
	// location block (not a server-level return) leaves room for certbot's
	// temporary challenge location during renewal.
	sb.WriteString("server {\n")
	sb.WriteString(fmt.Sprintf("    listen %d;\n", listenPort))
	sb.WriteString(fmt.Sprintf("    server_name %s;\n", domains))
	if opts.SSL != nil && opts.SSL.RedirectHTTP {
		sb.WriteString(logLines)
		sb.WriteString("\n")
		sb.WriteString("    location / {\n")
		sb.WriteString("        return 301 https://$host$request_uri;\n")
		sb.WriteString("    }\n")
		sb.WriteString("}\n")
	} else {
		if err := writeServerBody(&sb, opts, root, logLines); err != nil {
			return "", err
		}
		sb.WriteString("}\n")
	}

	if opts.SSL != nil {
		sb.WriteString("\nserver {\n")
		sb.WriteString("    listen 443 ssl;\n")
		sb.WriteString(fmt.Sprintf("    server_name %s;\n", domains))
		writeTLSDirectives(&sb, opts)
		if err := writeServerBody(&sb, opts, root, logLines); err != nil {
			return "", err
		}
		sb.WriteString("}\n")
	}

	return sb.String(), nil
}

// writeServerBody writes root, index, logs, and runtime locations for a server block.
func writeServerBody(sb *strings.Builder, opts vhostConfig, root, logLines string) error {
	sb.WriteString(fmt.Sprintf("    root %s;\n", root))
	sb.WriteString("    index index.html index.htm index.php;\n")
	sb.WriteString(logLines)
	sb.WriteString("\n")

	// Redirects come first so they win over the runtime locations below.
	redirects, err := renderRedirects(opts.Redirects)
	if err != nil {
		return err
	}
	sb.WriteString(redirects)

	switch opts.Runtime {
	case RuntimePHP:
		socket := opts.PHPSocket
		if socket == "" {
			socket = platformProvider.PHPFPMDefaultSocket(normalizePHPVersion(opts.PHPVersion))
		}
		sb.WriteString("    location / {\n")
		sb.WriteString("        try_files $uri $uri/ /index.php?$query_string;\n")
		sb.WriteString("    }\n\n")
		sb.WriteString("    location ~ \\.php$ {\n")
		if include := platformProvider.NginxPHPFastCGIInclude(); include != "" {
			sb.WriteString(fmt.Sprintf("        include %s;\n", include))
		} else {
			// RHEL-family nginx has no snippets/fastcgi-php.conf; inline equivalents.
			sb.WriteString("        try_files $uri =404;\n")
			sb.WriteString("        fastcgi_index index.php;\n")
			sb.WriteString("        include fastcgi_params;\n")
		}
		sb.WriteString(fmt.Sprintf("        fastcgi_pass unix:%s;\n", socket))
		sb.WriteString("        fastcgi_param SCRIPT_FILENAME $realpath_root$fastcgi_script_name;\n")
		sb.WriteString("        fastcgi_param DOCUMENT_ROOT $realpath_root;\n")
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
	return nil
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
	stored := *state
	stored.AccessLogPath = ""
	stored.ErrorLogPath = ""
	data, err := json.MarshalIndent(&stored, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.statePath(state.Name), data, 0640)
}

func (s *Service) deleteState(name string) error {
	return os.Remove(s.statePath(name))
}

func (s *Service) migrateLegacyProjects() error {
	return migrateProjects(s.stateDir, s.provider.Paths().AbstraxProjectsDirLegacy)
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
