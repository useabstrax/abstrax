package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// InspectProject is the project payload for the inspect API.
type InspectProject struct {
	Name     string           `json:"name"`
	Path     string           `json:"path"`
	User     string           `json:"user"`
	Group    string           `json:"group,omitempty"`
	Mode     string           `json:"ownership_mode,omitempty"`
	Runtime  InspectRuntime   `json:"runtime"`
	Domains  []string         `json:"domains"`
	Services []InspectService `json:"services"`
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
			Name:     state.Name,
			Path:     state.Path,
			User:     user,
			Group:    group,
			Mode:     mode,
			Runtime:  runtime,
			Domains:  state.Domains,
			Services: services,
		},
	}, nil
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
