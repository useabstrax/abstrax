package project

import (
	"context"
	"fmt"
	"path/filepath"

	"abstrax/internal/identity"
	"abstrax/internal/validate"
)

const (
	// SharedWebUser is the default runtime and project owner for shared mode.
	SharedWebUser = "www-data"
	// SharedWebGroup is the default project group for shared mode.
	SharedWebGroup = "www-data"
	// DefaultSharedBase is the default project root for shared mode.
	DefaultSharedBase = "/var/www"
	// NginxUser is the user nginx workers run as on Debian/Ubuntu.
	NginxUser = "www-data"
)

// OwnershipMode describes how a project is owned and executed.
type OwnershipMode string

const (
	OwnershipShared   OwnershipMode = "shared"
	OwnershipIsolated OwnershipMode = "isolated"
)

// RuntimeIdentity holds resolved ownership and runtime user details.
type RuntimeIdentity struct {
	Mode          OwnershipMode
	User          string
	Group         string
	UID           int
	GID           int
	Home          string
	WebServerUser string
}

// ResolveIdentity determines ownership mode and runtime identity from add options.
func ResolveIdentity(ctx context.Context, resolver identity.Resolver, opts AddOptions) (RuntimeIdentity, error) {
	if !opts.UserExplicit {
		group := opts.Group
		if group == "" {
			group = SharedWebGroup
		}
		if err := validate.GroupName(group); err != nil {
			return RuntimeIdentity{}, err
		}
		return RuntimeIdentity{
			Mode:          OwnershipShared,
			User:          SharedWebUser,
			Group:         group,
			WebServerUser: NginxUser,
		}, nil
	}

	if err := validate.Username(opts.User); err != nil {
		return RuntimeIdentity{}, err
	}

	account, err := resolver.Lookup(ctx, opts.User)
	if err != nil {
		return RuntimeIdentity{}, err
	}

	return RuntimeIdentity{
		Mode:          OwnershipIsolated,
		User:          account.Username,
		Group:         account.PrimaryGroup,
		UID:           account.UID,
		GID:           account.GID,
		Home:          account.Home,
		WebServerUser: NginxUser,
	}, nil
}

// ResolveProjectPath applies path precedence rules and returns the project root.
func ResolveProjectPath(name, explicitPath string, id RuntimeIdentity) (string, error) {
	if explicitPath != "" {
		return filepath.Clean(explicitPath), nil
	}
	if id.Mode == OwnershipIsolated {
		if id.Home == "" {
			return "", fmt.Errorf("user %q has no home directory", id.User)
		}
		return filepath.Join(id.Home, name), nil
	}
	return filepath.Join(DefaultSharedBase, name), nil
}

// IdentityFromState reconstructs runtime identity from persisted project state.
func IdentityFromState(state *State) RuntimeIdentity {
	mode := state.OwnershipMode
	if mode == "" {
		mode = OwnershipShared
	}

	user := state.Owner
	if user == "" {
		user = SharedWebUser
	}
	group := state.Group
	if group == "" {
		group = SharedWebGroup
	}

	return RuntimeIdentity{
		Mode:          mode,
		User:          user,
		Group:         group,
		UID:           state.OwnerUID,
		GID:           state.OwnerGID,
		Home:          state.OwnerHome,
		WebServerUser: NginxUser,
	}
}
