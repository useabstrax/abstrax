package project

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"abstrax/internal/confirm"
	executil "abstrax/internal/exec"
	"abstrax/internal/services/pkgmanager"
	"abstrax/internal/services/svcmanager"
)

// RuntimeSpec describes a runtime version requirement.
type RuntimeSpec struct {
	Runtime Runtime
	Version string
}

func runtimeSpecFromAdd(opts AddOptions) RuntimeSpec {
	switch opts.Runtime {
	case RuntimePHP:
		return RuntimeSpec{Runtime: RuntimePHP, Version: normalizePHPVersion(opts.PHPVersion)}
	case RuntimeNode:
		return RuntimeSpec{Runtime: RuntimeNode, Version: normalizeNodeVersion(opts.NodeVersion)}
	case RuntimeRuby:
		return RuntimeSpec{Runtime: RuntimeRuby, Version: normalizeRubyVersion(opts.RubyVersion)}
	default:
		return RuntimeSpec{}
	}
}

func runtimeSpecFromState(state *State, opts ModifyOptions) RuntimeSpec {
	runtime := state.Runtime
	if opts.Runtime != "" {
		runtime = opts.Runtime
	}

	switch runtime {
	case RuntimePHP:
		version := state.PHPVersion
		if opts.PHPVersion != "" {
			version = normalizePHPVersion(opts.PHPVersion)
		} else if version == "" {
			version = DefaultPHPVersion
		}
		return RuntimeSpec{Runtime: RuntimePHP, Version: version}
	case RuntimeNode:
		version := state.NodeVersion
		if opts.NodeVersion != "" {
			version = normalizeNodeVersion(opts.NodeVersion)
		} else if version == "" {
			version = DefaultNodeVersion
		}
		return RuntimeSpec{Runtime: RuntimeNode, Version: version}
	case RuntimeRuby:
		version := state.RubyVersion
		if opts.RubyVersion != "" {
			version = normalizeRubyVersion(opts.RubyVersion)
		} else if version == "" {
			version = DefaultRubyVersion
		}
		return RuntimeSpec{Runtime: RuntimeRuby, Version: version}
	default:
		return RuntimeSpec{}
	}
}

func (spec RuntimeSpec) label() string {
	switch spec.Runtime {
	case RuntimePHP:
		return fmt.Sprintf("PHP %s", spec.Version)
	case RuntimeNode:
		return fmt.Sprintf("Node.js %s", spec.Version)
	case RuntimeRuby:
		return fmt.Sprintf("Ruby %s", spec.Version)
	default:
		return ""
	}
}

// Installed reports whether the requested runtime version is available.
func (spec RuntimeSpec) Installed() bool {
	switch spec.Runtime {
	case RuntimePHP:
		return packageInstalled(fmt.Sprintf("php%s-fpm", spec.Version))
	case RuntimeNode:
		return nodeMajorVersion() == nodeMajor(spec.Version)
	case RuntimeRuby:
		return rubyMatchesVersion(spec.Version)
	default:
		return true
	}
}

func (s *Service) ensureRuntime(ctx context.Context, spec RuntimeSpec, yes, dryRun bool) error {
	if spec.Runtime == "" || spec.Runtime == RuntimeStatic {
		return nil
	}

	if spec.Installed() {
		return nil
	}

	fmt.Printf("%s is not installed on this server.\n", spec.label())
	fmt.Printf("Abstrax can install %s.\n", spec.label())

	if dryRun {
		fmt.Printf("[dry-run] would prompt to install %s\n", spec.label())
		return nil
	}

	ok, err := confirm.Ask(fmt.Sprintf("Install %s now?", spec.label()), yes)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s is not installed", spec.label())
	}

	return s.installRuntime(ctx, spec, dryRun)
}

func (s *Service) installRuntime(ctx context.Context, spec RuntimeSpec, dryRun bool) error {
	mgr := pkgmanager.NewApt(dryRun, false)
	svc := svcmanager.New(dryRun, false)

	if err := mgr.Update(ctx); err != nil {
		return fmt.Errorf("updating package lists: %w", err)
	}

	switch spec.Runtime {
	case RuntimePHP:
		fpmPkg := fmt.Sprintf("php%s-fpm", spec.Version)
		cliPkg := fmt.Sprintf("php%s-cli", spec.Version)
		for _, pkg := range []string{fpmPkg, cliPkg} {
			if err := mgr.Install(ctx, pkgmanager.InstallOptions{Name: pkg}); err != nil {
				return fmt.Errorf("installing %s: %w", pkg, err)
			}
		}
		if err := svc.Enable(ctx, fpmPkg); err != nil {
			return err
		}
		return svc.Start(ctx, fpmPkg)

	case RuntimeNode:
		major := nodeMajor(spec.Version)
		setupScript := fmt.Sprintf("curl -fsSL https://deb.nodesource.com/setup_%s.x | bash -", major)
		if _, err := s.runner.Run(ctx, "bash", "-c", setupScript); err != nil {
			return fmt.Errorf("configuring NodeSource repository for Node.js %s: %w", spec.Version, err)
		}
		if err := mgr.Install(ctx, pkgmanager.InstallOptions{Name: "nodejs"}); err != nil {
			return fmt.Errorf("installing Node.js %s: %w", spec.Version, err)
		}
		if nodeMajorVersion() != major {
			return fmt.Errorf("Node.js %s was requested but a different version is installed", spec.Version)
		}

	case RuntimeRuby:
		pkg := fmt.Sprintf("ruby%s", spec.Version)
		if err := mgr.Install(ctx, pkgmanager.InstallOptions{Name: pkg}); err != nil {
			if err := mgr.Install(ctx, pkgmanager.InstallOptions{Name: "ruby-full"}); err != nil {
				return fmt.Errorf("installing Ruby %s: %w", spec.Version, err)
			}
		}
		if !rubyMatchesVersion(spec.Version) {
			return fmt.Errorf("Ruby %s was requested but a different version is installed", spec.Version)
		}

	default:
		return fmt.Errorf("unsupported runtime %q", spec.Runtime)
	}

	return nil
}

func packageInstalled(name string) bool {
	cmd := exec.Command("dpkg-query", "-W", "-f=${Status}", name)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "install ok installed")
}

var nodeVersionRE = regexp.MustCompile(`^v?(\d+)`)

func nodeMajor(version string) string {
	parts := strings.SplitN(strings.TrimPrefix(version, "v"), ".", 2)
	if len(parts) == 0 || parts[0] == "" {
		return version
	}
	return parts[0]
}

func nodeMajorVersion() string {
	if !executil.Exists("node") {
		return ""
	}
	cmd := exec.Command("node", "--version")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	m := nodeVersionRE.FindStringSubmatch(strings.TrimSpace(string(out)))
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func rubyMatchesVersion(want string) bool {
	if !executil.Exists("ruby") {
		return false
	}
	cmd := exec.Command("ruby", "--version")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	wantMajor := rubyMajorMinor(want)
	if wantMajor == "" {
		return false
	}
	re := regexp.MustCompile(`ruby (\d+\.\d+)`)
	m := re.FindStringSubmatch(strings.TrimSpace(string(out)))
	if len(m) < 2 {
		return false
	}
	return m[1] == wantMajor
}

func rubyMajorMinor(version string) string {
	parts := strings.Split(strings.TrimPrefix(version, "v"), ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	if len(parts) == 1 && parts[0] != "" {
		return parts[0]
	}
	return version
}
