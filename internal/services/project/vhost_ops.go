package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"abstrax/internal/services/web"
)

func (s *Service) isNginxInstalled() bool {
	if s.nginxInstalled != nil {
		return s.nginxInstalled()
	}
	return web.Installed(string(WebServerNginx))
}

// AddVhost creates an nginx vhost for a project that has none, such as a
// directory project added with --no-vhost. Domains, Port, and WebRoot in opts
// replace the stored values when set. Project files are not touched.
func (s *Service) AddVhost(ctx context.Context, opts AddVhostOptions) (*State, error) {
	state, err := s.loadState(opts.Name)
	if err != nil {
		return nil, fmt.Errorf("project %q not found", opts.Name)
	}
	if state.VhostPath != "" {
		return nil, fmt.Errorf("project %q already has a vhost", opts.Name)
	}
	if state.WebServer == WebServerApache {
		return nil, fmt.Errorf("Apache support is not yet implemented")
	}
	if !s.isNginxInstalled() {
		return nil, fmt.Errorf("nginx is not installed; install it first with: %s",
			web.InstallCommand(string(WebServerNginx)))
	}

	if len(opts.Domains) > 0 {
		state.Domains = opts.Domains
	}
	if opts.Port != 0 {
		if opts.Port < 1 || opts.Port > 65535 {
			return nil, fmt.Errorf("invalid port %d", opts.Port)
		}
		state.Port = opts.Port
	}
	if opts.WebRoot != "" {
		webRoot := filepath.Clean(opts.WebRoot)
		if !filepath.IsAbs(webRoot) {
			return nil, fmt.Errorf("web root %q must be an absolute path", opts.WebRoot)
		}
		if err := ensureInside(state.Path, webRoot, "web root"); err != nil {
			return nil, err
		}
		state.WebRoot = webRoot
	}
	state.WebServer = WebServerNginx

	vhostPath, err := s.createNginxVhost(ctx, state.vhostConfig())
	if err != nil {
		return nil, fmt.Errorf("creating nginx vhost: %w", err)
	}
	state.VhostPath = vhostPath
	state.VhostDisabled = false
	state.UpdatedAt = time.Now()

	if err := s.saveState(state); err != nil {
		_ = s.deleteVhostFiles(ctx, vhostPath)
		return nil, fmt.Errorf("saving project state: %w", err)
	}
	s.applyLogPaths(state)
	return state, nil
}

// RemoveVhost deletes a project's nginx vhost and keeps the project, its
// files, and its state. The project becomes a directory project again.
func (s *Service) RemoveVhost(ctx context.Context, name string) (*State, error) {
	state, err := s.loadState(name)
	if err != nil {
		return nil, fmt.Errorf("project %q not found", name)
	}
	if state.VhostPath == "" {
		return nil, fmt.Errorf("project %q has no vhost", name)
	}

	if err := s.deleteVhostFiles(ctx, state.VhostPath); err != nil {
		return nil, fmt.Errorf("removing vhost: %w", err)
	}

	state.VhostPath = ""
	state.VhostDisabled = false
	state.SSLEnabled = false
	state.UpdatedAt = time.Now()
	if err := s.saveState(state); err != nil {
		return nil, fmt.Errorf("saving project state: %w", err)
	}
	s.applyLogPaths(state)
	return state, nil
}

// deleteVhostFiles removes the vhost file, its enabled link or .disabled copy,
// and reloads nginx. A failed reload is ignored, as in Remove.
func (s *Service) deleteVhostFiles(ctx context.Context, vhostPath string) error {
	if err := s.disableVhostFile(vhostPath); err != nil {
		return err
	}
	for _, path := range []string{vhostPath, disabledVhostPath(vhostPath)} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	_, _ = s.runner.Run(ctx, "nginx", "-s", "reload")
	return nil
}
