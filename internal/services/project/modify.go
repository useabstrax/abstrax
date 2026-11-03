package project

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"abstrax/internal/validate"
)

// applyModify applies opts to state in memory. Nothing is written to disk.
//
// Every field except Runtime and SharedLogs is applied only when its flag is
// present in opts.Changed. A present flag with an empty value clears the field:
// empty domains, public-dir, or version, or proxy-port 0. Runtime is applied
// when non-empty. SharedLogs is applied when non-nil.
//
// Path updates the stored path only. Files on disk are not moved or copied.
func (s *Service) applyModify(ctx context.Context, state *State, opts ModifyOptions) error {
	if opts.Changed["path"] {
		if err := s.modifyPath(ctx, state, opts); err != nil {
			return err
		}
	}

	if err := modifyDomains(state, opts); err != nil {
		return err
	}

	if opts.Runtime != "" {
		state.Runtime = opts.Runtime
	}

	if opts.Changed["public-dir"] {
		publicDir := strings.TrimSpace(opts.PublicDir)
		if err := validatePublicDir(state.Path, publicDir); err != nil {
			return err
		}
		state.PublicDir = publicDir
	}

	if opts.Changed["proxy-port"] {
		if opts.ProxyPort != 0 {
			if err := validate.Port(opts.ProxyPort); err != nil {
				return err
			}
		}
		state.ProxyPort = opts.ProxyPort
	}

	if err := modifyVersions(state, opts); err != nil {
		return err
	}

	if opts.SharedLogs != nil {
		state.SharedLogs = *opts.SharedLogs
	}
	return nil
}

func (s *Service) modifyPath(ctx context.Context, state *State, opts ModifyOptions) error {
	requested := strings.TrimSpace(opts.Path)
	if requested == "" {
		return fmt.Errorf("path cannot be empty")
	}
	if err := validate.FilePath(requested); err != nil {
		return err
	}

	approvedRoots, err := loadApprovedRoots()
	if err != nil {
		return err
	}
	homes, err := s.identity.ListHomes(ctx)
	if err != nil {
		return err
	}

	validated, err := ValidateProjectPath(PathValidateOptions{
		RequestedPath: requested,
		ProjectName:   state.Name,
		PublicDir:     state.PublicDir,
		Identity:      IdentityFromState(state),
		ApprovedRoots: approvedRoots,
		Homes:         homes,
	})
	if err != nil {
		return err
	}
	state.Path = validated.ProjectPath
	if state.OwnershipMode == OwnershipIsolated {
		state.ApprovedRoot = validated.ApprovedRoot
	}
	return nil
}

func validatePublicDir(projectPath, publicDir string) error {
	if publicDir == "" {
		return nil
	}
	if err := validate.SingleLine("public-dir", publicDir); err != nil {
		return err
	}
	return ensureInside(projectPath, filepath.Join(projectPath, publicDir), "public path")
}

// modifyDomains applies domains, add-domain, and remove-domain in that order.
// Emptying the list is refused when the project has a vhost, because nginx
// would then be written with server_name _ and answer for every host.
func modifyDomains(state *State, opts ModifyOptions) error {
	touched := false

	if opts.Changed["domains"] {
		touched = true
		domains := make([]string, 0, len(opts.Domains))
		for _, d := range opts.Domains {
			d = strings.TrimSpace(d)
			if d == "" {
				continue
			}
			if err := validate.Domain(d); err != nil {
				return err
			}
			if !slices.Contains(domains, d) {
				domains = append(domains, d)
			}
		}
		state.Domains = domains
	}

	if opts.Changed["add-domain"] {
		touched = true
		d := strings.TrimSpace(opts.AddDomain)
		if err := validate.Domain(d); err != nil {
			return err
		}
		if !slices.Contains(state.Domains, d) {
			state.Domains = append(state.Domains, d)
		}
	}

	if opts.Changed["remove-domain"] {
		touched = true
		d := strings.TrimSpace(opts.RemoveDomain)
		if d == "" {
			return fmt.Errorf("domain cannot be empty")
		}
		state.Domains = slices.DeleteFunc(slices.Clone(state.Domains), func(existing string) bool {
			return existing == d
		})
	}

	if touched && len(state.Domains) == 0 && state.VhostPath != "" {
		return fmt.Errorf("cannot remove every domain from project %q: its nginx vhost would use server_name _ and answer for all hosts; set at least one domain, or remove the vhost first", state.Name)
	}
	return nil
}

// modifyVersions applies php-version, node-version, and ruby-version. An empty
// value clears the stored version. For the active runtime, the default version
// is stored instead so the vhost and runtime check have a version to use.
func modifyVersions(state *State, opts ModifyOptions) error {
	for _, v := range []struct {
		flag    string
		value   string
		runtime Runtime
		target  *string
		normal  func(string) string
	}{
		{"php-version", opts.PHPVersion, RuntimePHP, &state.PHPVersion, normalizePHPVersion},
		{"node-version", opts.NodeVersion, RuntimeNode, &state.NodeVersion, normalizeNodeVersion},
		{"ruby-version", opts.RubyVersion, RuntimeRuby, &state.RubyVersion, normalizeRubyVersion},
	} {
		if !opts.Changed[v.flag] {
			continue
		}
		value := strings.TrimSpace(v.value)
		if err := validate.SingleLine(v.flag, value); err != nil {
			return err
		}
		if value == "" && state.Runtime != v.runtime {
			*v.target = ""
			continue
		}
		*v.target = v.normal(value)
	}
	return nil
}

// modifyTouchesRuntime reports whether the options change the version of the
// active runtime, or switch runtime, so the runtime needs to be checked.
func modifyTouchesRuntime(state *State, opts ModifyOptions) bool {
	if opts.Runtime != "" {
		return true
	}
	switch state.Runtime {
	case RuntimePHP:
		return opts.Changed["php-version"]
	case RuntimeNode:
		return opts.Changed["node-version"]
	case RuntimeRuby:
		return opts.Changed["ruby-version"]
	default:
		return false
	}
}

// vhostConfigEqual reports whether two vhost configs render the same vhost.
func vhostConfigEqual(a, b vhostConfig) bool {
	if !slices.Equal(a.Domains, b.Domains) {
		return false
	}
	a.Domains, b.Domains = nil, nil
	return a.Name == b.Name && a.Path == b.Path && a.Port == b.Port &&
		a.Runtime == b.Runtime && a.PHPVersion == b.PHPVersion &&
		a.NodeVersion == b.NodeVersion && a.RubyVersion == b.RubyVersion &&
		a.PublicDir == b.PublicDir && a.WebRoot == b.WebRoot &&
		a.ProxyPort == b.ProxyPort && a.NodePort == b.NodePort &&
		a.PHPSocket == b.PHPSocket && a.LogDir == b.LogDir &&
		a.SharedLogs == b.SharedLogs
}
