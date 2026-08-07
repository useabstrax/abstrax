package platform

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// RepoName identifies a third-party or extra repository Abstrax may enable.
type RepoName string

const (
	RepoEPEL   RepoName = "epel"
	RepoCRB    RepoName = "crb"
	RepoRemi   RepoName = "remi"
	RepoOndrej RepoName = "ondrej"
)

// RepoOptions controls repository enablement behaviour.
type RepoOptions struct {
	// EnableRequiredRepos allows automatic enablement of policy-sensitive repos
	// (EPEL/Remi on RHEL/Oracle). Rocky/Alma may enable EPEL without this flag.
	// Ondřej on Debian-family is enabled automatically when PHP is installed.
	EnableRequiredRepos bool
	// Yes skips confirmation prompts when confirmation is used by callers.
	Yes     bool
	DryRun  bool
	Verbose bool
	// Purpose is a short reason shown in Remi enablement messages
	// (for example "multi-version PHP" or "Redis"). Empty defaults to a
	// generic Remi description.
	Purpose string
}

// RepoEnabler installs packages and runs commands for repository setup.
type RepoEnabler struct {
	Install    func(ctx context.Context, name string) error
	Run        func(ctx context.Context, name string, args ...string) error
	Exists     func(name string) bool
	FileExists func(path string) bool
	ReadFile   func(path string) ([]byte, error)
	WriteFile  func(path string, data []byte, perm os.FileMode) error
	RemoveFile func(path string) error
	Glob       func(pattern string) ([]string, error)
}

// DefaultRepoEnabler builds a RepoEnabler using the system package manager helpers.
func DefaultRepoEnabler(install func(ctx context.Context, name string) error, run func(ctx context.Context, name string, args ...string) error) RepoEnabler {
	return RepoEnabler{
		Install: install,
		Run:     run,
		Exists: func(name string) bool {
			_, err := exec.LookPath(name)
			return err == nil
		},
	}
}

// SupportsAutomaticRepoEnable reports whether the provider may enable a repo without
// an explicit --enable-required-repos flag.
func SupportsAutomaticRepoEnable(provider Provider, repo RepoName) bool {
	if provider == nil {
		return false
	}
	if provider.Profile().IsDebianFamily() {
		return repo == RepoOndrej
	}
	if !provider.Profile().IsRHELFamily() {
		return false
	}
	id := strings.ToLower(provider.Profile().DistroID)
	switch repo {
	case RepoEPEL:
		return id == "rocky" || id == "almalinux" || id == "centos"
	case RepoCRB:
		return id == "rocky" || id == "almalinux" || id == "centos"
	case RepoRemi:
		// Remi is always treated as requiring explicit consent on all RHEL-family hosts
		// unless EnableRequiredRepos is set, except we allow Rocky/Alma with the flag
		// or with automatic path when EnableRequiredRepos is true.
		return false
	default:
		return false
	}
}

// EnsureRepository enables a named repository according to provider policy.
func EnsureRepository(ctx context.Context, provider Provider, repo RepoName, opts RepoOptions, enabler RepoEnabler) error {
	if provider == nil {
		return nil
	}
	switch repo {
	case RepoEPEL, RepoCRB, RepoRemi:
		if !provider.Profile().IsRHELFamily() {
			return nil
		}
	case RepoOndrej:
		if !provider.Profile().IsDebianFamily() {
			return fmt.Errorf("Ondřej PHP repository is only available on Debian-family systems")
		}
		return ensureOndrejRepo(ctx, provider, opts, enabler)
	default:
		return fmt.Errorf("unknown repository %q", repo)
	}
	switch repo {
	case RepoEPEL:
		return ensureEPELRepo(ctx, provider, opts, enabler)
	case RepoCRB:
		return ensureCRBRepo(ctx, provider, opts, enabler)
	case RepoRemi:
		return ensureRemiRepo(ctx, provider, opts, enabler)
	default:
		return fmt.Errorf("unknown repository %q", repo)
	}
}

func ensureEPELRepo(ctx context.Context, provider Provider, opts RepoOptions, enabler RepoEnabler) error {
	id := strings.ToLower(provider.Profile().DistroID)
	auto := SupportsAutomaticRepoEnable(provider, RepoEPEL)
	if !auto && !opts.EnableRequiredRepos {
		return fmt.Errorf("EPEL is required but not enabled automatically on %s; run `sudo abstrax repo enable epel --enable-required-repos` (or pass --enable-required-repos to the command that needs it)", provider.Profile().DistroName)
	}
	if id == "rhel" || id == "ol" || id == "oracle" {
		if !opts.EnableRequiredRepos {
			return fmt.Errorf("EPEL on %s requires explicit consent; run `sudo abstrax repo enable epel --enable-required-repos`", provider.Profile().DistroName)
		}
		fmt.Println("Enabling EPEL repository (explicit --enable-required-repos)...")
	} else {
		fmt.Println("Installing EPEL repository...")
	}
	if err := enabler.Install(ctx, "epel-release"); err != nil {
		return fmt.Errorf("installing epel-release: %w", err)
	}
	_ = enabler.Run(ctx, "dnf", "makecache")
	return nil
}

func ensureCRBRepo(ctx context.Context, provider Provider, opts RepoOptions, enabler RepoEnabler) error {
	id := strings.ToLower(provider.Profile().DistroID)
	auto := SupportsAutomaticRepoEnable(provider, RepoCRB)
	if !auto && !opts.EnableRequiredRepos {
		return fmt.Errorf("CRB/CodeReady Builder is required; run `sudo abstrax repo enable crb --enable-required-repos` or pass --enable-required-repos")
	}
	fmt.Println("Enabling CRB / CodeReady Builder repository...")
	// dnf config-manager --set-enabled crb (Rocky/Alma/CentOS Stream 9+)
	name := "crb"
	if id == "rhel" {
		major, err := EnterpriseLinuxMajor(provider)
		if err != nil {
			return err
		}
		name = fmt.Sprintf("codeready-builder-for-rhel-%d-x86_64-rpms", major)
	}
	if err := enabler.Run(ctx, "dnf", "config-manager", "--set-enabled", name); err != nil {
		// Fallback common alias.
		if err2 := enabler.Run(ctx, "dnf", "config-manager", "--set-enabled", "crb"); err2 != nil {
			return fmt.Errorf("enabling CRB: %w", err)
		}
	}
	return nil
}

func remiPurpose(opts RepoOptions) string {
	purpose := strings.TrimSpace(opts.Purpose)
	if purpose == "" {
		return "Remi packages"
	}
	return purpose
}

func ensureRemiRepo(ctx context.Context, provider Provider, opts RepoOptions, enabler RepoEnabler) error {
	purpose := remiPurpose(opts)
	if !opts.EnableRequiredRepos && !SupportsAutomaticRepoEnable(provider, RepoRemi) {
		// Allow Rocky/Alma to proceed when EnableRequiredRepos is set; otherwise require flag.
		id := strings.ToLower(provider.Profile().DistroID)
		if id == "rocky" || id == "almalinux" || id == "centos" {
			if !opts.EnableRequiredRepos {
				return fmt.Errorf("Remi is required for %s on %s; re-run with --enable-required-repos (or run `sudo abstrax repo enable remi --enable-required-repos`)", purpose, provider.Profile().DistroName)
			}
		} else {
			return fmt.Errorf("Remi is required for %s on %s; run `sudo abstrax repo enable remi --enable-required-repos`", purpose, provider.Profile().DistroName)
		}
	}

	// Remi depends on EPEL (+ often CRB).
	if err := ensureEPELRepo(ctx, provider, opts, enabler); err != nil {
		return err
	}
	_ = ensureCRBRepo(ctx, provider, opts, enabler)

	remiURL, err := RemiReleaseURL(provider)
	if err != nil {
		return err
	}

	fmt.Printf("Installing Remi repository (required for %s)...\n", purpose)
	if err := enabler.Install(ctx, remiURL); err != nil {
		// Try generic remi-release package if URL install fails (already configured mirror).
		if err2 := enabler.Install(ctx, "remi-release"); err2 != nil {
			return fmt.Errorf("installing Remi release package: %w (fallback: %v)", err, err2)
		}
	}
	_ = enabler.Run(ctx, "dnf", "makecache")
	return nil
}

// EnterpriseLinuxMajor returns the major Enterprise Linux version from the
// provider profile (for example 9 or 10).
func EnterpriseLinuxMajor(provider Provider) (int, error) {
	if provider == nil {
		return 0, fmt.Errorf("platform provider is nil")
	}
	maj, _, ok := parseVersionID(provider.Profile().VersionID)
	if !ok {
		return 0, fmt.Errorf("could not determine Enterprise Linux major version from VERSION_ID %q", provider.Profile().VersionID)
	}
	if maj < 9 {
		return 0, fmt.Errorf("Enterprise Linux %d is not supported; Rocky/Alma 9+ is required", maj)
	}
	return maj, nil
}

// RemiReleaseURL returns the Remi release RPM URL matching the host EL major.
func RemiReleaseURL(provider Provider) (string, error) {
	maj, err := EnterpriseLinuxMajor(provider)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://rpms.remirepo.net/enterprise/remi-release-%d.rpm", maj), nil
}

// EnsurePHPRepository enables the third-party PHP repository required by the
// provider (Ondřej on Debian-family, Remi on RHEL-family).
func EnsurePHPRepository(ctx context.Context, provider Provider, version string, opts RepoOptions, enabler RepoEnabler) error {
	if provider == nil || !provider.RequiresExternalRepoForPHP(version) {
		return nil
	}
	if opts.Purpose == "" {
		opts.Purpose = "multi-version PHP"
	}
	if provider.Profile().IsDebianFamily() {
		return EnsureRepository(ctx, provider, RepoOndrej, opts, enabler)
	}
	return EnsureRepository(ctx, provider, RepoRemi, opts, enabler)
}

// EnsureRedisRepository enables Remi and the Remi Redis module stream when the
// provider requires an external repository for Redis (for example Rocky/Alma 10+,
// where AppStream ships Valkey instead of Redis).
func EnsureRedisRepository(ctx context.Context, provider Provider, opts RepoOptions, enabler RepoEnabler) error {
	if provider == nil || !provider.RequiresExternalRepoForRedis() {
		return nil
	}
	if opts.Purpose == "" {
		opts.Purpose = "Redis"
	}
	if err := EnsureRepository(ctx, provider, RepoRemi, opts, enabler); err != nil {
		return err
	}
	stream := provider.RedisModuleStream()
	if stream == "" {
		return fmt.Errorf("no Remi Redis module stream configured for this platform")
	}
	fmt.Printf("Enabling Remi Redis module stream %s...\n", stream)
	_ = enabler.Run(ctx, "dnf", "module", "reset", "redis", "-y")
	if err := enabler.Run(ctx, "dnf", "module", "enable", stream, "-y"); err != nil {
		return fmt.Errorf("enabling Redis module stream %s: %w", stream, err)
	}
	_ = enabler.Run(ctx, "dnf", "makecache")
	return nil
}

// RepoEnableHint returns a user-facing instruction for enabling a repository.
func RepoEnableHint(repo RepoName) string {
	return fmt.Sprintf("sudo abstrax repo enable %s --enable-required-repos", repo)
}
