package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"abstrax/internal/platform"
)

const inspectAPIVersion = "v1"

// InspectRuntime describes a project's runtime for the inspect API.
type InspectRuntime struct {
	Type    string `json:"type"`
	Version string `json:"version"`
}

// InspectService describes a project service for the inspect API.
type InspectService struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// InspectVhost describes the project's web server vhost for the inspect API.
type InspectVhost struct {
	// Present is true when the project has a managed vhost recorded in state.
	Present bool `json:"present"`
	// Path is the vhost config path from state (empty when not present).
	Path string `json:"path"`
	// Enabled reports whether the web server will load the vhost right now.
	Enabled bool `json:"enabled"`
}

// InspectSSL describes the project's SSL status for the inspect API.
type InspectSSL struct {
	Enabled         bool     `json:"enabled"`
	CertificateName string   `json:"certificate_name,omitempty"`
	Domains         []string `json:"domains,omitempty"`
	RedirectHTTP    bool     `json:"redirect_http,omitempty"`
	Staging         bool     `json:"staging,omitempty"`
	ExpiresAt       string   `json:"expires_at,omitempty"` // RFC3339 UTC
}

// InspectProject is the project payload for the inspect API.
type InspectProject struct {
	Name          string           `json:"name"`
	Path          string           `json:"path"`
	User          string           `json:"user"`
	Group         string           `json:"group,omitempty"`
	Mode          string           `json:"ownership_mode,omitempty"`
	WebServer     string           `json:"web_server"`
	Runtime       InspectRuntime   `json:"runtime"`
	PublicDir     string           `json:"public_dir,omitempty"`
	ProxyPort     int              `json:"proxy_port,omitempty"`
	Domains       []string         `json:"domains"`
	Vhost         InspectVhost     `json:"vhost"`
	SharedLogs    bool             `json:"shared_logs"`
	AccessLogPath string           `json:"access_log_path,omitempty"`
	ErrorLogPath  string           `json:"error_log_path,omitempty"`
	SSL           InspectSSL       `json:"ssl"`
	Services      []InspectService `json:"services"`
	// CreatedAt and UpdatedAt are RFC3339 UTC timestamps. They are omitted
	// when the state file has no timestamp.
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// InspectResponse is the versioned public inspect API response.
type InspectResponse struct {
	APIVersion string         `json:"api_version"`
	Project    InspectProject `json:"project"`
}

// Inspect returns the versioned public project inspect payload.
func (s *Service) Inspect(ctx context.Context, name string) (*InspectResponse, error) {
	state, err := s.Info(ctx, name)
	if err != nil {
		return nil, err
	}

	services, err := s.discoverServices(state)
	if err != nil {
		return nil, err
	}

	runtime := InspectRuntime{Type: string(state.Runtime)}
	switch state.Runtime {
	case RuntimePHP:
		runtime.Version = state.PHPVersion
	case RuntimeNode:
		runtime.Version = state.NodeVersion
	case RuntimeRuby:
		runtime.Version = state.RubyVersion
	}

	user := state.Owner
	if user == "" {
		user = SharedWebUser
	}
	group := state.Group
	if group == "" {
		group = SharedWebGroup
	}

	mode := string(state.OwnershipMode)
	if mode == "" {
		mode = string(OwnershipShared)
	}

	return &InspectResponse{
		APIVersion: inspectAPIVersion,
		Project: InspectProject{
			Name:          state.Name,
			Path:          state.Path,
			User:          user,
			Group:         group,
			Mode:          mode,
			WebServer:     string(state.WebServer),
			Runtime:       runtime,
			PublicDir:     state.PublicDir,
			ProxyPort:     state.ProxyPort,
			Domains:       state.Domains,
			Vhost:         s.inspectVhost(state),
			SharedLogs:    state.SharedLogs,
			AccessLogPath: state.AccessLogPath,
			ErrorLogPath:  state.ErrorLogPath,
			SSL:           inspectSSL(state),
			Services:      services,
			CreatedAt:     formatInspectTime(state.CreatedAt),
			UpdatedAt:     formatInspectTime(state.UpdatedAt),
		},
	}, nil
}

func formatInspectTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func inspectSSL(state *State) InspectSSL {
	out := InspectSSL{Enabled: state.SSLEnabled}
	if ssl := state.EffectiveSSL(); ssl != nil {
		out.CertificateName = ssl.CertificateName
		out.Domains = ssl.Domains
		out.RedirectHTTP = ssl.RedirectHTTP
		out.Staging = ssl.Staging
		out.ExpiresAt = formatInspectTime(ssl.ExpiresAt)
	}
	return out
}

// inspectVhost reports whether the project has a vhost and whether it is enabled.
//
// Present is true when state records a VhostPath. Enabled is checked on disk
// using the same rules as project enable and disable:
//   - Debian layout (sites-available/sites-enabled): the symlink in the
//     sites-enabled directory named after the vhost file must exist.
//   - RHEL layout (conf.d): the vhost file must exist at VhostPath. A file
//     renamed to <path>.disabled counts as disabled.
//   - Any other or unknown layout: the vhost file must exist at VhostPath.
func (s *Service) inspectVhost(state *State) InspectVhost {
	if state.VhostPath == "" {
		return InspectVhost{}
	}
	vhost := InspectVhost{Present: true, Path: state.VhostPath}
	if state.VhostDisabled || strings.HasSuffix(state.VhostPath, ".disabled") {
		return vhost
	}

	if s.nginxLayout == platform.NginxSitesAvailableEnabled && s.nginxEnabled != "" {
		link := filepath.Join(s.nginxEnabled, filepath.Base(state.VhostPath))
		if _, err := os.Lstat(link); err != nil {
			return vhost
		}
		// The link must resolve; a dangling link is not loaded by nginx.
		_, err := os.Stat(link)
		vhost.Enabled = err == nil
		return vhost
	}

	info, err := os.Stat(state.VhostPath)
	vhost.Enabled = err == nil && !info.IsDir()
	return vhost
}

func (s *Service) discoverServices(state *State) ([]InspectService, error) {
	if len(state.Services) > 0 {
		services := make([]InspectService, 0, len(state.Services))
		for _, svc := range state.Services {
			services = append(services, InspectService{
				Name: svc.Name,
				Type: svc.Type,
			})
		}
		return services, nil
	}

	confDir := supervisorConfDir()
	entries, err := os.ReadDir(confDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading supervisor configs: %w", err)
	}

	prefix := "abstrax-" + state.Name + "-"
	var services []InspectService
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) || !strings.HasSuffix(e.Name(), ".conf") {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".conf")
		serviceName := strings.TrimPrefix(base, "abstrax-"+state.Name+"-")
		services = append(services, InspectService{
			Name: serviceName,
			Type: "worker",
		})
	}
	return services, nil
}

var supervisorConfDirPath = "/etc/supervisor/conf.d"

func supervisorConfDir() string {
	return supervisorConfDirPath
}

// ResolveProjectDaemon resolves a service name to a supervisor daemon owned by the project.
func (s *Service) ResolveProjectDaemon(ctx context.Context, projectName, serviceName string) (string, error) {
	state, err := s.Info(ctx, projectName)
	if err != nil {
		return "", err
	}

	services, err := s.discoverServices(state)
	if err != nil {
		return "", err
	}

	for _, svc := range services {
		if svc.Name == serviceName {
			daemonName := "abstrax-" + projectName + "-" + serviceName
			confPath := filepath.Join(supervisorConfDir(), daemonName+".conf")
			if _, err := os.Stat(confPath); err != nil {
				return "", fmt.Errorf("service %q is registered for project %q but supervisor config was not found", serviceName, projectName)
			}
			return daemonName, nil
		}
	}

	return "", fmt.Errorf("service %q does not belong to project %q", serviceName, projectName)
}
