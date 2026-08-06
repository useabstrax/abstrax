package platform

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	ondrejKeyringPath   = "/usr/share/keyrings/abstrax-ondrej-php.gpg"
	ondrejListPath      = "/etc/apt/sources.list.d/abstrax-ondrej-php.list"
	ondrejKeyID         = "4F4EA0AAE5267A6C" // Ondřej Surý Launchpad PPA signing key
	ondrejFallbackSuite = "noble"            // Ubuntu 24.04 LTS — published on the PPA

	suryKeyringPath   = "/usr/share/keyrings/debsuryorg-archive-keyring.gpg"
	suryKeyringDebURL = "https://packages.sury.org/debsuryorg-archive-keyring.deb"
	suryPHPRepoURL    = "https://packages.sury.org/php/"
)

// ondrejPublishedSuites are Ubuntu codenames known to have Release files on
// ppa:ondrej/php. Newer devel releases fall back to noble until published.
var ondrejPublishedSuites = map[string]bool{
	"focal":    true,
	"jammy":    true,
	"noble":    true,
	"oracular": true,
	"plucky":   true,
}

// UsesOndrejLaunchpadPPA reports whether the host should use ppa:ondrej/php
// (Ubuntu and Ubuntu derivatives) rather than packages.sury.org (Debian).
func UsesOndrejLaunchpadPPA(provider Provider) bool {
	if provider == nil || !provider.Profile().IsDebianFamily() {
		return false
	}
	id := strings.ToLower(provider.Profile().DistroID)
	switch id {
	case "ubuntu", "pop", "linuxmint":
		return true
	default:
		return false
	}
}

// PHPAptSuite returns the apt suite/codename used for the Ondřej PHP repository.
func PHPAptSuite(provider Provider) (suite string, fallback bool, err error) {
	if provider == nil {
		return "", false, fmt.Errorf("platform provider is nil")
	}
	p := provider.Profile()
	if UsesOndrejLaunchpadPPA(provider) {
		codename := p.UbuntuCodename
		if codename == "" {
			codename = p.VersionCodename
		}
		suite, fallback = resolveOndrejUbuntuSuite(codename)
		return suite, fallback, nil
	}
	codename := p.VersionCodename
	if codename == "" {
		return "", false, fmt.Errorf("could not determine Debian suite from VERSION_CODENAME; ensure /etc/os-release is populated")
	}
	return codename, false, nil
}

func resolveOndrejUbuntuSuite(codename string) (suite string, fallback bool) {
	codename = strings.ToLower(strings.TrimSpace(codename))
	if ondrejPublishedSuites[codename] {
		return codename, false
	}
	return ondrejFallbackSuite, codename != "" && codename != ondrejFallbackSuite
}

func ondrejListContent(suite string, launchpad bool) string {
	if launchpad {
		return fmt.Sprintf("# Managed by Abstrax (ppa:ondrej/php)\ndeb [signed-by=%s] https://ppa.launchpadcontent.net/ondrej/php/ubuntu %s main\n",
			ondrejKeyringPath, suite)
	}
	return fmt.Sprintf("# Managed by Abstrax (packages.sury.org/php)\ndeb [signed-by=%s] %s %s main\n",
		suryKeyringPath, suryPHPRepoURL, suite)
}

func (e RepoEnabler) fileExists(path string) bool {
	if e.FileExists != nil {
		return e.FileExists(path)
	}
	_, err := os.Stat(path)
	return err == nil
}

func (e RepoEnabler) readFile(path string) ([]byte, error) {
	if e.ReadFile != nil {
		return e.ReadFile(path)
	}
	return os.ReadFile(path)
}

func (e RepoEnabler) writeFile(path string, data []byte, perm os.FileMode) error {
	if e.WriteFile != nil {
		return e.WriteFile(path, data, perm)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, perm)
}

func (e RepoEnabler) removeFile(path string) error {
	if e.RemoveFile != nil {
		return e.RemoveFile(path)
	}
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (e RepoEnabler) glob(pattern string) ([]string, error) {
	if e.Glob != nil {
		return e.Glob(pattern)
	}
	return filepath.Glob(pattern)
}

func ondrejPHPRepoReady(enabler RepoEnabler, suite string, launchpad bool) bool {
	data, err := enabler.readFile(ondrejListPath)
	if err != nil {
		return false
	}
	body := string(data)
	want := ondrejListContent(suite, launchpad)
	if body != want {
		return false
	}
	if launchpad {
		return enabler.fileExists(ondrejKeyringPath)
	}
	return enabler.fileExists(suryKeyringPath)
}

// removeConflictingOndrejLists drops auto-added ondrej/sury apt sources that may
// conflict with the Abstrax-managed entry (for example unpublished Ubuntu suites).
func removeConflictingOndrejLists(enabler RepoEnabler) error {
	for _, pattern := range []string{
		"/etc/apt/sources.list.d/*ondrej*",
		"/etc/apt/sources.list.d/*sury*",
		"/etc/apt/sources.list.d/php.list",
		"/etc/apt/sources.list.d/php.sources",
	} {
		matches, err := enabler.glob(pattern)
		if err != nil {
			continue
		}
		for _, path := range matches {
			if path == ondrejListPath {
				continue
			}
			if err := enabler.removeFile(path); err != nil {
				return fmt.Errorf("removing conflicting apt source %s: %w", path, err)
			}
		}
	}
	return nil
}

func ensureOndrejKeyringLaunchpad(ctx context.Context, enabler RepoEnabler) error {
	if enabler.fileExists(ondrejKeyringPath) {
		return nil
	}
	for _, pkg := range []string{"ca-certificates", "curl", "gnupg"} {
		if err := enabler.Install(ctx, pkg); err != nil {
			return fmt.Errorf("installing %s: %w", pkg, err)
		}
	}
	script := fmt.Sprintf(`set -euo pipefail
tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT
curl -fsSL "https://keyserver.ubuntu.com/pks/lookup?op=get&search=0x%s" -o "$tmp"
gpg --batch --yes --dearmor -o %q "$tmp"
chmod 0644 %q
`, ondrejKeyID, ondrejKeyringPath, ondrejKeyringPath)
	if err := enabler.Run(ctx, "bash", "-c", script); err != nil {
		return fmt.Errorf("importing Ondřej PPA signing key: %w", err)
	}
	return nil
}

func ensureOndrejKeyringSury(ctx context.Context, enabler RepoEnabler) error {
	if enabler.fileExists(suryKeyringPath) {
		return nil
	}
	for _, pkg := range []string{"ca-certificates", "curl"} {
		if err := enabler.Install(ctx, pkg); err != nil {
			return fmt.Errorf("installing %s: %w", pkg, err)
		}
	}
	script := fmt.Sprintf(`set -euo pipefail
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
curl -fsSLo "$tmp/debsuryorg-archive-keyring.deb" %q
dpkg -i "$tmp/debsuryorg-archive-keyring.deb"
`, suryKeyringDebURL)
	if err := enabler.Run(ctx, "bash", "-c", script); err != nil {
		return fmt.Errorf("installing packages.sury.org archive keyring: %w", err)
	}
	if !enabler.fileExists(suryKeyringPath) {
		return fmt.Errorf("packages.sury.org keyring missing after install (%s)", suryKeyringPath)
	}
	return nil
}

func ensureOndrejRepo(ctx context.Context, provider Provider, opts RepoOptions, enabler RepoEnabler) error {
	_ = opts
	if provider == nil || !provider.Profile().IsDebianFamily() {
		return fmt.Errorf("Ondřej PHP repository is only available on Debian-family systems")
	}

	suite, usedFallback, err := PHPAptSuite(provider)
	if err != nil {
		return err
	}
	launchpad := UsesOndrejLaunchpadPPA(provider)

	if ondrejPHPRepoReady(enabler, suite, launchpad) {
		if err := removeConflictingOndrejLists(enabler); err != nil {
			return err
		}
		fmt.Printf("Ondřej PHP repository already configured (suite %s).\n", suite)
		return nil
	}

	if err := removeConflictingOndrejLists(enabler); err != nil {
		return err
	}

	if launchpad {
		if usedFallback {
			fmt.Printf("Ubuntu suite is not published on ppa:ondrej/php yet; using %s packages...\n", suite)
		} else {
			fmt.Printf("Configuring Ondřej PHP PPA (ppa:ondrej/php, suite %s)...\n", suite)
		}
		if err := ensureOndrejKeyringLaunchpad(ctx, enabler); err != nil {
			return err
		}
	} else {
		fmt.Printf("Configuring Ondřej PHP repository (packages.sury.org, suite %s)...\n", suite)
		if err := ensureOndrejKeyringSury(ctx, enabler); err != nil {
			return err
		}
	}

	content := ondrejListContent(suite, launchpad)
	if err := enabler.writeFile(ondrejListPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing Ondřej apt source: %w", err)
	}

	fmt.Println("Updating package lists after configuring Ondřej PHP repository...")
	if err := enabler.Run(ctx, "apt-get", "update"); err != nil {
		return fmt.Errorf("updating package lists: %w", err)
	}
	return nil
}
