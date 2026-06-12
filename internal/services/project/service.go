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
	"abstrax/internal/platform/debian"
	"abstrax/internal/services/web"
)

// Service manages projects.
type Service struct {
	runner       *executil.Runner
	stateDir     string
	nginxAvail   string
	nginxEnabled string
}

// New creates a Service.
func New(dryRun, verbose bool) *Service {
	return &Service{
		runner:       executil.New(dryRun, verbose),
		stateDir:     debian.AbstraxProjectsDir,
		nginxAvail:   debian.NginxSitesAvailable,
		nginxEnabled: debian.NginxSitesEnabled,
	}
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

	if err := os.MkdirAll(opts.Path, 0755); err != nil {
		return nil, fmt.Errorf("creating project path: %w", err)
	}

	state := &State{
		Name:      opts.Name,
		Path:      opts.Path,
		Domains:   opts.Domains,
		WebServer: opts.WebServer,
		Runtime:   opts.Runtime,
		Owner:     opts.User,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if opts.WebServer == WebServerNginx && !opts.NoVhost {
		vhostPath, err := s.createNginxVhost(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("creating nginx vhost: %w", err)
		}
		state.VhostPath = vhostPath
	}

	if err := s.saveState(state); err != nil {
		return nil, fmt.Errorf("saving project state: %w", err)
	}

	return state, nil
}

// Remove removes a project and optionally its web server configuration.
func (s *Service) Remove(ctx context.Context, opts RemoveOptions) error {
	state, err := s.loadState(opts.Name)
	if err != nil {
		return fmt.Errorf("project %q not found", opts.Name)
	}

	if state.VhostPath != "" && opts.RemoveVhost {
		enabledLink := filepath.Join(s.nginxEnabled, filepath.Base(state.VhostPath))
		_ = os.Remove(enabledLink)

		if _, err := backup.File(state.VhostPath); err != nil {
			return err
		}
		_ = os.Remove(state.VhostPath)
		_, _ = s.runner.Run(ctx, "nginx", "-s", "reload")
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
	state.UpdatedAt = time.Now()

	if state.VhostPath != "" && state.WebServer == WebServerNginx {
		addOpts := AddOptions{
			Name:      state.Name,
			Path:      state.Path,
			WebServer: state.WebServer,
			Domains:   state.Domains,
			Runtime:   state.Runtime,
		}
		if _, err := backup.File(state.VhostPath); err != nil {
			return nil, err
		}
		if _, err := s.createNginxVhost(ctx, addOpts); err != nil {
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

func (s *Service) createNginxVhost(ctx context.Context, opts AddOptions) (string, error) {
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

func buildNginxConfig(opts AddOptions) string {
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
		phpVersion := opts.PHPVersion
		if phpVersion == "" {
			phpVersion = "8.2"
		}
		sb.WriteString("    location / {\n")
		sb.WriteString("        try_files $uri $uri/ /index.php?$query_string;\n")
		sb.WriteString("    }\n\n")
		sb.WriteString("    location ~ \\.php$ {\n")
		sb.WriteString("        include snippets/fastcgi-php.conf;\n")
		sb.WriteString(fmt.Sprintf("        fastcgi_pass unix:/run/php/php%s-fpm.sock;\n", phpVersion))
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
