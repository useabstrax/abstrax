package project

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"abstrax/internal/validate"
)

// SetRuntimeOptions holds options for changing a project's runtime type.
type SetRuntimeOptions struct {
	Name    string
	Runtime Runtime
	// PHPVersion, NodeVersion and RubyVersion apply only to the matching target
	// runtime. A version for a different runtime is rejected.
	PHPVersion  string
	NodeVersion string
	RubyVersion string
	// PublicDir applies to php and static projects.
	PublicDir string
	// ProxyPort applies to node and ruby projects.
	ProxyPort int
	Yes       bool
	DryRun    bool
}

// SetRuntime changes a project between php, node, ruby and static.
//
// The change is transactional. The target runtime is ensured first. A new PHP-FPM
// pool is created before the vhost is switched, and an old pool is removed only
// after the vhost and project state are saved. If any step fails, the vhost bytes
// and any new pool are restored and the stored state is left as it was.
func (s *Service) SetRuntime(ctx context.Context, opts SetRuntimeOptions) (*State, error) {
	state, err := s.loadState(opts.Name)
	if err != nil {
		return nil, fmt.Errorf("project %q not found", opts.Name)
	}

	next, err := runtimeTargetState(state, opts)
	if err != nil {
		return nil, err
	}

	if !runtimeChanged(state, next) {
		s.applyLogPaths(state)
		return state, nil
	}

	if err := s.ensureRuntime(ctx, runtimeSpecFromTarget(next), opts.Yes, opts.DryRun, nil); err != nil {
		return nil, err
	}

	if opts.DryRun {
		next.UpdatedAt = time.Now()
		s.applyLogPaths(next)
		return next, nil
	}

	id := IdentityFromState(state)
	isolated := id.Mode == OwnershipIsolated
	versionChanged := state.Runtime != RuntimePHP || next.PHPVersion != normalizePHPVersion(state.PHPVersion)
	createPool := isolated && next.Runtime == RuntimePHP && versionChanged
	removeOldPool := isolated && state.PHPPoolName != "" &&
		(next.Runtime != RuntimePHP || next.PHPVersion != normalizePHPVersion(state.PHPVersion))

	var createdPool *State
	var restoreVhost func(context.Context)
	undo := func() {
		if restoreVhost != nil {
			restoreVhost(ctx)
		}
		if createdPool != nil {
			_ = s.removePHPPool(ctx, createdPool)
		}
	}

	if createPool {
		if _, err := s.createPHPPool(ctx, next, id); err != nil {
			return nil, fmt.Errorf("creating php-fpm pool: %w", err)
		}
		pool := *next
		createdPool = &pool
	}

	// Projects set up before Abstrax stored SSL metadata keep their HTTPS
	// listener when the vhost is rewritten.
	s.AdoptSSL(next)

	if next.WebServer == WebServerNginx && next.VhostPath != "" {
		restoreVhost, err = s.rewriteVhost(ctx, next)
		if err != nil {
			undo()
			return nil, fmt.Errorf("updating nginx vhost: %w", err)
		}
	}

	next.UpdatedAt = time.Now()
	if err := s.saveState(next); err != nil {
		undo()
		_ = s.saveState(state)
		return nil, fmt.Errorf("saving project state: %w", err)
	}

	if removeOldPool {
		if err := s.removePHPPool(ctx, state); err != nil {
			return nil, err
		}
	}

	s.applyLogPaths(next)
	return next, nil
}

// runtimeTargetState returns a copy of state switched to the requested runtime,
// with every field that belongs to another runtime cleared.
func runtimeTargetState(state *State, opts SetRuntimeOptions) (*State, error) {
	if err := validateRuntimeSetOptions(state, opts); err != nil {
		return nil, err
	}

	next := *state
	next.Domains = append([]string(nil), state.Domains...)
	next.Services = append([]ProjectService(nil), state.Services...)
	next.Runtime = opts.Runtime

	next.PHPVersion = ""
	next.NodeVersion = ""
	next.RubyVersion = ""
	next.ProxyPort = 0
	next.PublicDir = ""

	samePHP := state.Runtime == RuntimePHP && opts.Runtime == RuntimePHP
	next.PHPPoolName = ""
	next.PHPSocketPath = ""

	switch opts.Runtime {
	case RuntimePHP:
		next.PHPVersion = normalizePHPVersion(firstNonEmpty(opts.PHPVersion, state.PHPVersion))
		next.PublicDir = firstNonEmpty(opts.PublicDir, state.PublicDir)
		if samePHP && next.PHPVersion == normalizePHPVersion(state.PHPVersion) {
			next.PHPPoolName = state.PHPPoolName
			next.PHPSocketPath = state.PHPSocketPath
		}
	case RuntimeNode:
		next.NodeVersion = normalizeNodeVersion(firstNonEmpty(opts.NodeVersion, state.NodeVersion))
		next.ProxyPort = proxyPortFor(state, opts)
	case RuntimeRuby:
		next.RubyVersion = normalizeRubyVersion(firstNonEmpty(opts.RubyVersion, state.RubyVersion))
		next.ProxyPort = proxyPortFor(state, opts)
	case RuntimeStatic:
		next.PublicDir = firstNonEmpty(opts.PublicDir, state.PublicDir)
	}
	return &next, nil
}

func validateRuntimeSetOptions(state *State, opts SetRuntimeOptions) error {
	switch opts.Runtime {
	case RuntimePHP, RuntimeNode, RuntimeRuby, RuntimeStatic:
	default:
		return fmt.Errorf("unsupported runtime %q (use php, node, ruby or static)", opts.Runtime)
	}

	reject := func(flag string, set bool) error {
		if set {
			return fmt.Errorf("--%s does not apply to a %s project", flag, opts.Runtime)
		}
		return nil
	}
	var checks []error
	if opts.Runtime != RuntimePHP {
		checks = append(checks, reject("php-version", opts.PHPVersion != ""))
	}
	if opts.Runtime != RuntimeNode {
		checks = append(checks, reject("node-version", opts.NodeVersion != ""))
	}
	if opts.Runtime != RuntimeRuby {
		checks = append(checks, reject("ruby-version", opts.RubyVersion != ""))
	}
	if opts.Runtime != RuntimeNode && opts.Runtime != RuntimeRuby {
		checks = append(checks, reject("proxy-port", opts.ProxyPort != 0))
	}
	if opts.Runtime != RuntimePHP && opts.Runtime != RuntimeStatic {
		checks = append(checks, reject("public-dir", opts.PublicDir != ""))
	}
	for _, err := range checks {
		if err != nil {
			return err
		}
	}

	if opts.ProxyPort != 0 {
		if err := validate.Port(opts.ProxyPort); err != nil {
			return err
		}
	}
	if opts.PublicDir != "" {
		if filepath.IsAbs(opts.PublicDir) {
			return fmt.Errorf("--public-dir must be relative to the project path")
		}
		root := filepath.Clean(state.Path)
		if err := ensureInside(root, filepath.Join(root, opts.PublicDir), "public directory"); err != nil {
			return err
		}
	}
	return nil
}

// proxyPortFor keeps the existing proxy port when moving between node and ruby.
func proxyPortFor(state *State, opts SetRuntimeOptions) int {
	if opts.ProxyPort != 0 {
		return opts.ProxyPort
	}
	if state.Runtime == RuntimeNode || state.Runtime == RuntimeRuby {
		return state.ProxyPort
	}
	return 0
}

func runtimeSpecFromTarget(state *State) RuntimeSpec {
	switch state.Runtime {
	case RuntimePHP:
		return RuntimeSpec{Runtime: RuntimePHP, Version: state.PHPVersion}
	case RuntimeNode:
		return RuntimeSpec{Runtime: RuntimeNode, Version: state.NodeVersion}
	case RuntimeRuby:
		return RuntimeSpec{Runtime: RuntimeRuby, Version: state.RubyVersion}
	default:
		return RuntimeSpec{Runtime: state.Runtime}
	}
}

func runtimeChanged(old, next *State) bool {
	return old.Runtime != next.Runtime ||
		old.PHPVersion != next.PHPVersion ||
		old.NodeVersion != next.NodeVersion ||
		old.RubyVersion != next.RubyVersion ||
		old.PublicDir != next.PublicDir ||
		old.ProxyPort != next.ProxyPort
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// rewriteVhost regenerates a project's nginx vhost from state through
// writeNginxVhost, so TLS from stored SSL state is kept, a disabled vhost stays
// disabled, and a vhost nginx rejects is rolled back. The returned function
// restores the previous vhost after a later failure.
func (s *Service) rewriteVhost(ctx context.Context, state *State) (func(context.Context), error) {
	snap := s.snapshotVhost(state.VhostPath)
	vhostPath, err := s.writeNginxVhost(ctx, state.vhostConfig(), state.VhostDisabled)
	if err != nil {
		return nil, err
	}
	state.VhostPath = vhostPath
	return func(ctx context.Context) {
		s.restoreVhost(snap)
		if !state.VhostDisabled {
			_, _ = s.runner.Run(ctx, "nginx", "-s", "reload")
		}
	}, nil
}
