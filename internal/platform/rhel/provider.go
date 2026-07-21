// Package rhel provides RHEL-compatible specific helpers and constants.
package rhel

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var phpVersionRE = regexp.MustCompile(`^([0-9]+)\.([0-9]+)$`)

// SupportedRemiPHPVersions lists PHP versions Abstrax installs via Remi SCL.
var SupportedRemiPHPVersions = []string{"8.1", "8.2", "8.3", "8.4", "8.5"}

func normalizePHPVersion(version string) (string, error) {
	v := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(version), "php"))
	v = strings.TrimSpace(v)
	m := phpVersionRE.FindStringSubmatch(v)
	if m == nil {
		return "", fmt.Errorf("invalid PHP version %q; expected major.minor (for example 8.3)", version)
	}
	return m[1] + "." + m[2], nil
}

func remiModule(version string) (string, error) {
	v, err := normalizePHPVersion(version)
	if err != nil {
		return "", err
	}
	parts := strings.Split(v, ".")
	return "php" + parts[0] + parts[1], nil
}

func supportsRemiPHPVersion(version string) bool {
	v, err := normalizePHPVersion(version)
	if err != nil {
		return false
	}
	for _, s := range SupportedRemiPHPVersions {
		if s == v {
			return true
		}
	}
	return false
}

// Paths holds filesystem paths used on RHEL-compatible systems.
type Paths struct {
	SudoGroup                  string
	CronDir                    string
	SSHConfigDir               string
	AbstraxSSHConfig           string
	SupervisorConfDir          string
	NginxConfigDir             string
	NginxConfPath              string
	NginxConfDInclude          string
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
	PHPFPMPoolDir              string
	PHPFPMDefaultPoolConfig    string
}

// DefaultPaths returns the standard RHEL-family filesystem layout.
func DefaultPaths() Paths {
	return Paths{
		SudoGroup:                  SudoGroup,
		CronDir:                    CronDir,
		SSHConfigDir:               SSHConfigDir,
		AbstraxSSHConfig:           AbstraxSSHConfig,
		SupervisorConfDir:          SupervisorConfDir,
		NginxConfigDir:             NginxConfigDir,
		NginxConfPath:              NginxConfPath,
		NginxConfDInclude:          NginxConfDInclude,
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
		PHPFPMPoolDir:              PHPFPMPoolDir,
		PHPFPMDefaultPoolConfig:    PHPFPMDefaultPoolConfig,
	}
}

// Provider implements platform conventions for RHEL-compatible systems.
type Provider struct {
	webUser            string
	webGroup           string
	defaultProjectRoot string
	paths              Paths
}

// ProviderOptions configures a RHEL-family provider.
type ProviderOptions struct {
	WebUser            string
	WebGroup           string
	DefaultProjectRoot string
}

// NewProvider creates a RHEL-family provider.
func NewProvider(opts ProviderOptions) *Provider {
	webUser := opts.WebUser
	if webUser == "" {
		webUser = "nginx"
	}
	webGroup := opts.WebGroup
	if webGroup == "" {
		webGroup = "nginx"
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

func (p *Provider) Paths() Paths                      { return p.paths }
func (p *Provider) WebUser() string                   { return p.webUser }
func (p *Provider) WebGroup() string                  { return p.webGroup }
func (p *Provider) DefaultProjectRoot() string        { return p.defaultProjectRoot }
func (p *Provider) SupportsMultiplePHPVersions() bool { return true }

func (p *Provider) RequiresExternalRepoForPHP(version string) bool {
	return supportsRemiPHPVersion(version)
}

func (p *Provider) ValidatePHPVersion(version string) error {
	v, err := normalizePHPVersion(version)
	if err != nil {
		return err
	}
	if !supportsRemiPHPVersion(v) {
		return fmt.Errorf("PHP %s is not supported on RHEL-family systems via Remi; supported versions: %s",
			v, strings.Join(SupportedRemiPHPVersions, ", "))
	}
	return nil
}

func (p *Provider) PHPFPMServiceName(version string) string {
	mod, err := remiModule(version)
	if err != nil {
		return "php-fpm"
	}
	return mod + "-php-fpm"
}

func (p *Provider) PHPFPMBinary(version string) string {
	mod, err := remiModule(version)
	if err != nil {
		return "php-fpm"
	}
	return filepath.Join("/opt/remi", mod, "root/usr/sbin/php-fpm")
}

func (p *Provider) PHPCLIBinary(version string) string {
	mod, err := remiModule(version)
	if err != nil {
		return "php"
	}
	return filepath.Join("/opt/remi", mod, "root/usr/bin/php")
}

func (p *Provider) PHPFPMPoolDir(version string) string {
	mod, err := remiModule(version)
	if err != nil {
		return p.paths.PHPFPMPoolDir
	}
	return filepath.Join("/etc/opt/remi", mod, "php-fpm.d")
}

func (p *Provider) PHPFPMDefaultPoolConfig(version string) string {
	return filepath.Join(p.PHPFPMPoolDir(version), "www.conf")
}

func (p *Provider) PHPFPMDefaultSocket(version string) string {
	mod, err := remiModule(version)
	if err != nil {
		return filepath.Join(p.paths.PHPSocketDir, "www.sock")
	}
	return filepath.Join("/var/opt/remi", mod, "run/php-fpm/www.sock")
}

func (p *Provider) PHPFPMProjectSocket(version, poolSuffix string) string {
	mod, err := remiModule(version)
	if err != nil {
		return filepath.Join(p.paths.PHPSocketDir, poolSuffix+".sock")
	}
	return filepath.Join("/var/opt/remi", mod, "run/php-fpm", poolSuffix+".sock")
}

func (p *Provider) PHPPackageNames(version string, extensions []string) []string {
	mod, err := remiModule(version)
	if err != nil {
		return nil
	}
	pkgs := []string{
		mod + "-php-fpm",
		mod + "-php-cli",
	}
	for _, ext := range extensions {
		if PHPBundledWithCLI[ext] {
			continue
		}
		name := ext
		switch ext {
		case "mysql":
			name = "mysqlnd"
		case "sqlite3":
			name = "pdo"
		}
		pkgs = append(pkgs, mod+"-php-"+name)
	}
	return pkgs
}

func (p *Provider) NginxConfigDir() string { return p.paths.NginxConfigDir }
func (p *Provider) NginxConfPath() string  { return p.paths.NginxConfPath }
func (p *Provider) NginxConfDInclude() string {
	return p.paths.NginxConfDInclude
}

func (p *Provider) NginxSiteConfigPath(site string) string {
	name := strings.TrimSuffix(site, ".conf")
	return filepath.Join(p.paths.NginxConfigDir, name+".conf")
}

func (p *Provider) SudoGroup() string { return p.paths.SudoGroup }

// PHPBundledWithCLI lists extension suffixes that should not be installed as
// separate packages on Remi PHP builds.
var PHPBundledWithCLI = map[string]bool{
	"pcntl": true,
	"posix": true,
}
