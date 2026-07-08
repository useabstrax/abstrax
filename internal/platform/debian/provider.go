// Package debian provides Debian/Ubuntu specific helpers and constants.
package debian

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Paths holds filesystem paths used on Debian/Ubuntu systems.
type Paths struct {
	SudoGroup                  string
	CronDir                    string
	SSHConfigDir               string
	AbstraxSSHConfig           string
	SupervisorConfDir          string
	NginxSitesAvailable        string
	NginxSitesEnabled          string
	NginxConfPath              string
	NginxSitesEnabledInclude   string
	AbstraxStateDir            string
	AbstraxConfigDir           string
	AbstraxConfig              string
	AbstraxProjectsDir         string
	AbstraxProjectsDirLegacy   string
	MySQLConfig                string
	MySQLConfigLegacy          string
	AbstraxLogDir              string
	AbstraxPluginsDir          string
	AbstraxPluginsDirAlt       string
	AbstraxPluginStateDir      string
	AbstraxPluginCacheDir      string
	AbstraxPluginRegistryCache string
	PHPSocketDir               string
	PHPConfigRoot              string
}

// DefaultPaths returns the standard Debian/Ubuntu filesystem layout.
func DefaultPaths() Paths {
	return Paths{
		SudoGroup:                  SudoGroup,
		CronDir:                    CronDir,
		SSHConfigDir:               SSHConfigDir,
		AbstraxSSHConfig:           AbstraxSSHConfig,
		SupervisorConfDir:          SupervisorConfDir,
		NginxSitesAvailable:        NginxSitesAvailable,
		NginxSitesEnabled:          NginxSitesEnabled,
		NginxConfPath:              NginxConfPath,
		NginxSitesEnabledInclude:   NginxSitesEnabledInclude,
		AbstraxStateDir:            AbstraxStateDir,
		AbstraxConfigDir:           AbstraxConfigDir,
		AbstraxConfig:              AbstraxConfig,
		AbstraxProjectsDir:         AbstraxProjectsDir,
		AbstraxProjectsDirLegacy:   AbstraxProjectsDirLegacy,
		MySQLConfig:                MySQLConfig,
		MySQLConfigLegacy:          MySQLConfigLegacy,
		AbstraxLogDir:              AbstraxLogDir,
		AbstraxPluginsDir:          AbstraxPluginsDir,
		AbstraxPluginsDirAlt:       AbstraxPluginsDirAlt,
		AbstraxPluginStateDir:      AbstraxPluginStateDir,
		AbstraxPluginCacheDir:      AbstraxPluginCacheDir,
		AbstraxPluginRegistryCache: AbstraxPluginRegistryCacheDir,
		PHPSocketDir:               PHPSocketDir,
		PHPConfigRoot:              PHPConfigRoot,
	}
}

// Provider implements platform conventions for Debian/Ubuntu-based systems.
type Provider struct {
	webUser            string
	webGroup           string
	defaultProjectRoot string
	paths              Paths
}

// ProviderOptions configures a Debian-family provider.
type ProviderOptions struct {
	WebUser            string
	WebGroup           string
	DefaultProjectRoot string
}

// NewProvider creates a Debian-family provider.
func NewProvider(opts ProviderOptions) *Provider {
	webUser := opts.WebUser
	if webUser == "" {
		webUser = "www-data"
	}
	webGroup := opts.WebGroup
	if webGroup == "" {
		webGroup = "www-data"
	}
	projectRoot := opts.DefaultProjectRoot
	if projectRoot == "" {
		projectRoot = "/var/www"
	}
	return &Provider{
		webUser:            webUser,
		webGroup:           webGroup,
		defaultProjectRoot: projectRoot,
		paths:              DefaultPaths(),
	}
}

func (p *Provider) Paths() Paths { return p.paths }

func (p *Provider) WebUser() string  { return p.webUser }
func (p *Provider) WebGroup() string { return p.webGroup }

func (p *Provider) DefaultProjectRoot() string { return p.defaultProjectRoot }

func (p *Provider) PHPFPMServiceName(version string) string {
	return fmt.Sprintf("php%s-fpm", normalizeVersion(version))
}

func (p *Provider) PHPFPMBinary(version string) string {
	return fmt.Sprintf("php-fpm%s", normalizeVersion(version))
}

func (p *Provider) PHPFPMPoolDir(version string) string {
	return filepath.Join(PHPConfigRoot, normalizeVersion(version), "fpm", "pool.d")
}

func (p *Provider) PHPFPMDefaultSocket(version string) string {
	return filepath.Join(PHPSocketDir, fmt.Sprintf("php%s-fpm.sock", normalizeVersion(version)))
}

func (p *Provider) PHPFPMProjectSocket(version, poolSuffix string) string {
	return filepath.Join(PHPSocketDir, fmt.Sprintf("php%s-fpm-%s.sock", normalizeVersion(version), poolSuffix))
}

func (p *Provider) PHPPackageNames(version string, extensions []string) []string {
	version = normalizeVersion(version)
	pkgs := []string{
		"php" + version + "-fpm",
		"php" + version + "-cli",
	}
	for _, ext := range extensions {
		if PHPBundledWithCLI[ext] {
			continue
		}
		pkgs = append(pkgs, "php"+version+"-"+ext)
	}
	return pkgs
}

func (p *Provider) NginxSitesAvailable() string { return p.paths.NginxSitesAvailable }
func (p *Provider) NginxSitesEnabled() string   { return p.paths.NginxSitesEnabled }
func (p *Provider) NginxConfPath() string       { return p.paths.NginxConfPath }
func (p *Provider) NginxSitesEnabledInclude() string {
	return p.paths.NginxSitesEnabledInclude
}
func (p *Provider) SudoGroup() string { return p.paths.SudoGroup }

func normalizeVersion(version string) string {
	return strings.TrimPrefix(strings.TrimSpace(version), "php")
}

// PHPBundledWithCLI lists extension suffixes bundled with php*-cli on Debian/Ubuntu.
var PHPBundledWithCLI = map[string]bool{
	"pcntl": true,
	"posix": true,
}
