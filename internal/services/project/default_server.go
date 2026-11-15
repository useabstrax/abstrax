package project

import (
	"context"
	"fmt"

	"abstrax/internal/services/web"
)

func init() {
	web.SetProjectDefaultServerCheck(func() (bool, error) {
		svc := New(false, false)
		holder, err := svc.findDefaultServerProject("")
		if err != nil {
			return false, err
		}
		return holder != nil, nil
	})
}

// findDefaultServerProject returns the project that currently holds
// default_server, if any. except skips that project name.
func (s *Service) findDefaultServerProject(except string) (*State, error) {
	projects, err := s.List(context.Background())
	if err != nil {
		return nil, err
	}
	for i := range projects {
		p := &projects[i]
		if p.Name == except {
			continue
		}
		if p.DefaultServer && p.VhostPath != "" && !p.VhostDisabled {
			return p, nil
		}
	}
	return nil, nil
}

// claimDefaultServer sets DefaultServer on state when it has no domains and no
// other project already holds the IP. Returns whether this project answers on
// the server IP.
func (s *Service) claimDefaultServer(state *State) (bool, error) {
	if len(state.Domains) > 0 {
		state.DefaultServer = false
		return false, nil
	}
	other, err := s.findDefaultServerProject(state.Name)
	if err != nil {
		return false, err
	}
	if other != nil {
		state.DefaultServer = false
		return false, nil
	}
	state.DefaultServer = true
	return true, nil
}

// releaseDefaultServer clears DefaultServer on state and returns true when it
// previously held the IP.
func releaseDefaultServer(state *State) bool {
	if !state.DefaultServer {
		return false
	}
	state.DefaultServer = false
	return true
}

func (s *Service) syncWebIPDefault(ctx context.Context, projectHoldsDefault bool) error {
	return web.SyncIPDefaultServer(ctx, s.dryRun, false, projectHoldsDefault)
}

// DefaultServerMessages returns human-facing notes after creating or changing
// a domainless nginx vhost.
func DefaultServerMessages(state *State, claimed bool) []string {
	if state.VhostPath == "" || len(state.Domains) > 0 {
		return nil
	}
	if claimed || state.DefaultServer {
		return []string{"This project answers on the server IP (listen … default_server)."}
	}
	return []string{"This project will not answer on the server IP until it has a domain, because another project is already the default site."}
}

// ReleasedDefaultServerMessage explains that the project no longer owns the IP.
func ReleasedDefaultServerMessage() string {
	return "This project no longer answers on the server IP; default_server was removed."
}

// errDomainlessVhostBlocked is returned when clearing every domain would leave
// a vhost that cannot take default_server because another project already has it.
func errDomainlessVhostBlocked(project, holder string) error {
	return fmt.Errorf("cannot remove every domain from project %q: another project (%s) already answers on the server IP; set at least one domain, or remove the other project's default first", project, holder)
}
