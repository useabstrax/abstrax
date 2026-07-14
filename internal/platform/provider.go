package platform

import (
	"fmt"
	"strings"

	"abstrax/internal/platform/debian"
	"abstrax/internal/platform/rhel"
)

// Provider exposes platform-specific paths and naming conventions.
type Provider interface {
	Profile() Profile
	Paths() Paths
	WebUser() string
	WebGroup() string
	DefaultProjectRoot() string
	PHPFPMServiceName(version string) string
	PHPFPMBinary(version string) string
	PHPCLIBinary(version string) string
	PHPFPMPoolDir(version string) string
	PHPFPMDefaultPoolConfig(version string) string
	PHPFPMDefaultSocket(version string) string
	PHPFPMProjectSocket(version, poolSuffix string) string
	PHPPackageNames(version string, extensions []string) []string
	SupportsMultiplePHPVersions() bool
	RequiresExternalRepoForPHP(version string) bool
	ValidatePHPVersion(version string) error
	NginxLayout() NginxLayout
	NginxConfigDir() string
	NginxSiteConfigPath(site string) string
	NginxSitesAvailable() string
	NginxSitesEnabled() string
	NginxConfPath() string
	NginxSitesEnabledInclude() string
	// NginxPHPFastCGIInclude returns an nginx include used inside PHP location
	// blocks (for example snippets/fastcgi-php.conf on Debian). An empty string
	// means the caller should inline equivalent fastcgi directives.
	NginxPHPFastCGIInclude() string
	SudoGroup() string

	// Database conventions
	DatabaseEngine() DatabaseEngine
	DatabasePackage(version string) string
	DatabaseServiceName() string
	DatabaseDisplayName() string
	DatabaseAuthPlugin() DatabaseAuthPlugin
	DatabaseSocketCandidates() []string
	DatabaseConfigPaths() []string
	DatabaseDataDir() string
	DatabasePIDFile() string
	DatabaseDefaultsFile() string

	// Certbot
	CertbotPackages() []string
	RequiresEPELForCertbot() bool

	// Supervisor
	SupervisorPackage() string
	SupervisorServiceName() string
	SupervisorConfigDir() string
	SupervisorConfigExtension() string

	// Runtimes
	NodeSourceSetupURL(version string) (string, error)
	NodePackage() string
	RubyPackages(version string) ([]string, error)
	RubySupportsExactVersion() bool

	// Firewall
	FirewallSupportsNumberedRules() bool
}

type debianProvider struct {
	profile  Profile
	provider *debian.Provider
}

func (p *debianProvider) Profile() Profile { return p.profile }
func (p *debianProvider) Paths() Paths {
	dp := p.provider.Paths()
	return Paths{
		SudoGroup:                  dp.SudoGroup,
		CronDir:                    dp.CronDir,
		SSHConfigDir:               dp.SSHConfigDir,
		AbstraxSSHConfig:           dp.AbstraxSSHConfig,
		SupervisorConfDir:          dp.SupervisorConfDir,
		NginxSitesAvailable:        dp.NginxSitesAvailable,
		NginxSitesEnabled:          dp.NginxSitesEnabled,
		NginxConfPath:              dp.NginxConfPath,
		NginxSitesEnabledInclude:   dp.NginxSitesEnabledInclude,
		NginxConfigDir:             dp.NginxSitesAvailable,
		AbstraxStateDir:            dp.AbstraxStateDir,
		AbstraxConfigDir:           dp.AbstraxConfigDir,
		AbstraxConfig:              dp.AbstraxConfig,
		AbstraxProjectsDir:         dp.AbstraxProjectsDir,
		AbstraxProjectsDirLegacy:   dp.AbstraxProjectsDirLegacy,
		MySQLConfig:                dp.MySQLConfig,
		MySQLConfigLegacy:          dp.MySQLConfigLegacy,
		AbstraxLogDir:              dp.AbstraxLogDir,
		AbstraxPluginsDir:          dp.AbstraxPluginsDir,
		AbstraxPluginsDirAlt:       dp.AbstraxPluginsDirAlt,
		AbstraxPluginStateDir:      dp.AbstraxPluginStateDir,
		AbstraxPluginCacheDir:      dp.AbstraxPluginCacheDir,
		AbstraxPluginRegistryCache: dp.AbstraxPluginRegistryCache,
		PHPSocketDir:               dp.PHPSocketDir,
		PHPConfigRoot:              dp.PHPConfigRoot,
		PHPFPMPoolDir:              "",
		PHPFPMDefaultPoolConfig:    "",
	}
}
func (p *debianProvider) WebUser() string            { return p.provider.WebUser() }
func (p *debianProvider) WebGroup() string           { return p.provider.WebGroup() }
func (p *debianProvider) DefaultProjectRoot() string { return p.provider.DefaultProjectRoot() }
func (p *debianProvider) PHPFPMServiceName(version string) string {
	return p.provider.PHPFPMServiceName(version)
}
func (p *debianProvider) PHPFPMBinary(version string) string { return p.provider.PHPFPMBinary(version) }
func (p *debianProvider) PHPCLIBinary(version string) string { return p.provider.PHPCLIBinary(version) }
func (p *debianProvider) PHPFPMPoolDir(version string) string {
	return p.provider.PHPFPMPoolDir(version)
}
func (p *debianProvider) SupportsMultiplePHPVersions() bool { return true }
func (p *debianProvider) RequiresExternalRepoForPHP(version string) bool {
	_ = version
	return false
}
func (p *debianProvider) ValidatePHPVersion(version string) error {
	_, err := NormalizePHPVersion(version)
	return err
}
func (p *debianProvider) PHPFPMDefaultPoolConfig(version string) string {
	return p.provider.PHPFPMPoolDir(version) + "/www.conf"
}
func (p *debianProvider) PHPFPMDefaultSocket(version string) string {
	return p.provider.PHPFPMDefaultSocket(version)
}
func (p *debianProvider) PHPFPMProjectSocket(version, poolSuffix string) string {
	return p.provider.PHPFPMProjectSocket(version, poolSuffix)
}
func (p *debianProvider) PHPPackageNames(version string, extensions []string) []string {
	return p.provider.PHPPackageNames(version, extensions)
}
func (p *debianProvider) NginxLayout() NginxLayout { return NginxSitesAvailableEnabled }
func (p *debianProvider) NginxConfigDir() string   { return p.provider.NginxSitesAvailable() }
func (p *debianProvider) NginxSiteConfigPath(site string) string {
	return p.provider.NginxSitesAvailable() + "/" + site
}
func (p *debianProvider) NginxSitesAvailable() string { return p.provider.NginxSitesAvailable() }
func (p *debianProvider) NginxSitesEnabled() string   { return p.provider.NginxSitesEnabled() }
func (p *debianProvider) NginxConfPath() string       { return p.provider.NginxConfPath() }
func (p *debianProvider) NginxSitesEnabledInclude() string {
	return p.provider.NginxSitesEnabledInclude()
}
func (p *debianProvider) NginxPHPFastCGIInclude() string {
	return "snippets/fastcgi-php.conf"
}
func (p *debianProvider) SudoGroup() string { return p.provider.SudoGroup() }

func (p *debianProvider) DatabaseEngine() DatabaseEngine { return DatabaseMySQL }
func (p *debianProvider) DatabasePackage(version string) string {
	if version != "" {
		return "mysql-server-" + version
	}
	return "mysql-server"
}
func (p *debianProvider) DatabaseServiceName() string { return "mysql" }
func (p *debianProvider) DatabaseDisplayName() string { return "MySQL" }
func (p *debianProvider) DatabaseAuthPlugin() DatabaseAuthPlugin {
	return DatabaseAuthCachingSHA2
}
func (p *debianProvider) DatabaseSocketCandidates() []string {
	return []string{"/var/run/mysqld/mysqld.sock", "/run/mysqld/mysqld.sock"}
}
func (p *debianProvider) DatabaseConfigPaths() []string {
	return []string{"/etc/mysql/my.cnf", "/etc/mysql/conf.d", "/etc/mysql/mysql.conf.d", "/etc/mysql/mariadb.conf.d"}
}
func (p *debianProvider) DatabaseDataDir() string      { return "/var/lib/mysql" }
func (p *debianProvider) DatabasePIDFile() string      { return "/var/run/mysqld/mysqld.pid" }
func (p *debianProvider) DatabaseDefaultsFile() string { return "/etc/mysql/my.cnf" }

func (p *debianProvider) CertbotPackages() []string {
	return []string{"certbot", "python3-certbot-nginx"}
}
func (p *debianProvider) RequiresEPELForCertbot() bool { return false }

func (p *debianProvider) SupervisorPackage() string         { return "supervisor" }
func (p *debianProvider) SupervisorServiceName() string     { return "supervisor" }
func (p *debianProvider) SupervisorConfigDir() string       { return p.provider.Paths().SupervisorConfDir }
func (p *debianProvider) SupervisorConfigExtension() string { return ".conf" }

func (p *debianProvider) NodeSourceSetupURL(version string) (string, error) {
	return NodeSourceSetupURL("debian", version)
}
func (p *debianProvider) NodePackage() string { return "nodejs" }
func (p *debianProvider) RubyPackages(version string) ([]string, error) {
	v := strings.TrimSpace(strings.TrimPrefix(version, "v"))
	// Primary package first; callers may fall back to ruby-full on failure.
	return []string{"ruby" + v}, nil
}
func (p *debianProvider) RubySupportsExactVersion() bool { return true }

func (p *debianProvider) FirewallSupportsNumberedRules() bool { return true }

func newDebianProvider(profile Profile) Provider {
	return &debianProvider{
		profile: profile,
		provider: debian.NewProvider(debian.ProviderOptions{
			WebUser:            profile.WebUser,
			WebGroup:           profile.WebGroup,
			DefaultProjectRoot: profile.DefaultProjectRoot,
		}),
	}
}

type rhelProvider struct {
	profile  Profile
	provider *rhel.Provider
}

func (p *rhelProvider) Profile() Profile { return p.profile }
func (p *rhelProvider) Paths() Paths {
	rp := p.provider.Paths()
	return Paths{
		SudoGroup:                  rp.SudoGroup,
		CronDir:                    rp.CronDir,
		SSHConfigDir:               rp.SSHConfigDir,
		AbstraxSSHConfig:           rp.AbstraxSSHConfig,
		SupervisorConfDir:          rp.SupervisorConfDir,
		NginxSitesAvailable:        rp.NginxConfigDir,
		NginxSitesEnabled:          rp.NginxConfigDir,
		NginxConfPath:              rp.NginxConfPath,
		NginxSitesEnabledInclude:   rp.NginxConfDInclude,
		NginxConfigDir:             rp.NginxConfigDir,
		AbstraxStateDir:            rp.AbstraxStateDir,
		AbstraxConfigDir:           rp.AbstraxConfigDir,
		AbstraxConfig:              rp.AbstraxConfig,
		AbstraxProjectsDir:         rp.AbstraxProjectsDir,
		AbstraxProjectsDirLegacy:   rp.AbstraxProjectsDirLegacy,
		MySQLConfig:                rp.MySQLConfig,
		MySQLConfigLegacy:          rp.MySQLConfigLegacy,
		AbstraxLogDir:              rp.AbstraxLogDir,
		AbstraxPluginsDir:          rp.AbstraxPluginsDir,
		AbstraxPluginsDirAlt:       rp.AbstraxPluginsDirAlt,
		AbstraxPluginStateDir:      rp.AbstraxPluginStateDir,
		AbstraxPluginCacheDir:      rp.AbstraxPluginCacheDir,
		AbstraxPluginRegistryCache: rp.AbstraxPluginRegistryCache,
		PHPSocketDir:               rp.PHPSocketDir,
		PHPConfigRoot:              rp.PHPConfigRoot,
		PHPFPMPoolDir:              rp.PHPFPMPoolDir,
		PHPFPMDefaultPoolConfig:    rp.PHPFPMDefaultPoolConfig,
	}
}
func (p *rhelProvider) WebUser() string            { return p.provider.WebUser() }
func (p *rhelProvider) WebGroup() string           { return p.provider.WebGroup() }
func (p *rhelProvider) DefaultProjectRoot() string { return p.provider.DefaultProjectRoot() }
func (p *rhelProvider) PHPFPMServiceName(version string) string {
	return p.provider.PHPFPMServiceName(version)
}
func (p *rhelProvider) PHPFPMBinary(version string) string { return p.provider.PHPFPMBinary(version) }
func (p *rhelProvider) PHPCLIBinary(version string) string { return p.provider.PHPCLIBinary(version) }
func (p *rhelProvider) PHPFPMPoolDir(version string) string {
	return p.provider.PHPFPMPoolDir(version)
}
func (p *rhelProvider) SupportsMultiplePHPVersions() bool {
	return p.provider.SupportsMultiplePHPVersions()
}
func (p *rhelProvider) RequiresExternalRepoForPHP(version string) bool {
	return p.provider.RequiresExternalRepoForPHP(version)
}
func (p *rhelProvider) ValidatePHPVersion(version string) error {
	return p.provider.ValidatePHPVersion(version)
}
func (p *rhelProvider) PHPFPMDefaultPoolConfig(version string) string {
	return p.provider.PHPFPMDefaultPoolConfig(version)
}
func (p *rhelProvider) PHPFPMDefaultSocket(version string) string {
	return p.provider.PHPFPMDefaultSocket(version)
}
func (p *rhelProvider) PHPFPMProjectSocket(version, poolSuffix string) string {
	return p.provider.PHPFPMProjectSocket(version, poolSuffix)
}
func (p *rhelProvider) PHPPackageNames(version string, extensions []string) []string {
	return p.provider.PHPPackageNames(version, extensions)
}
func (p *rhelProvider) NginxLayout() NginxLayout { return NginxConfD }
func (p *rhelProvider) NginxConfigDir() string   { return p.provider.NginxConfigDir() }
func (p *rhelProvider) NginxSiteConfigPath(site string) string {
	return p.provider.NginxSiteConfigPath(site)
}
func (p *rhelProvider) NginxSitesAvailable() string { return p.provider.NginxConfigDir() }
func (p *rhelProvider) NginxSitesEnabled() string   { return p.provider.NginxConfigDir() }
func (p *rhelProvider) NginxConfPath() string       { return p.provider.NginxConfPath() }
func (p *rhelProvider) NginxSitesEnabledInclude() string {
	return p.provider.NginxConfDInclude()
}
func (p *rhelProvider) NginxPHPFastCGIInclude() string {
	// RHEL/Rocky/Alma nginx packages do not ship Debian's snippets/fastcgi-php.conf.
	return ""
}
func (p *rhelProvider) SudoGroup() string { return p.provider.SudoGroup() }

func (p *rhelProvider) DatabaseEngine() DatabaseEngine { return DatabaseMariaDB }
func (p *rhelProvider) DatabasePackage(version string) string {
	_ = version
	return "mariadb-server"
}
func (p *rhelProvider) DatabaseServiceName() string { return "mariadb" }
func (p *rhelProvider) DatabaseDisplayName() string {
	return "MariaDB (MySQL-compatible)"
}
func (p *rhelProvider) DatabaseAuthPlugin() DatabaseAuthPlugin {
	return DatabaseAuthNativePassword
}
func (p *rhelProvider) DatabaseSocketCandidates() []string {
	return []string{
		"/var/lib/mysql/mysql.sock",
		"/run/mysqld/mysqld.sock",
		"/var/run/mysqld/mysqld.sock",
		"/run/mariadb/mariadb.sock",
	}
}
func (p *rhelProvider) DatabaseConfigPaths() []string {
	return []string{"/etc/my.cnf", "/etc/my.cnf.d"}
}
func (p *rhelProvider) DatabaseDataDir() string      { return "/var/lib/mysql" }
func (p *rhelProvider) DatabasePIDFile() string      { return "/var/run/mariadb/mariadb.pid" }
func (p *rhelProvider) DatabaseDefaultsFile() string { return "/etc/my.cnf" }

func (p *rhelProvider) CertbotPackages() []string {
	return []string{"certbot", "python3-certbot-nginx"}
}
func (p *rhelProvider) RequiresEPELForCertbot() bool { return true }

func (p *rhelProvider) SupervisorPackage() string         { return "supervisor" }
func (p *rhelProvider) SupervisorServiceName() string     { return "supervisord" }
func (p *rhelProvider) SupervisorConfigDir() string       { return p.provider.Paths().SupervisorConfDir }
func (p *rhelProvider) SupervisorConfigExtension() string { return ".ini" }

func (p *rhelProvider) NodeSourceSetupURL(version string) (string, error) {
	return NodeSourceSetupURL("rhel", version)
}
func (p *rhelProvider) NodePackage() string { return "nodejs" }
func (p *rhelProvider) RubyPackages(version string) ([]string, error) {
	_ = version
	return []string{"ruby", "ruby-devel"}, nil
}
func (p *rhelProvider) RubySupportsExactVersion() bool { return false }

func (p *rhelProvider) FirewallSupportsNumberedRules() bool { return false }

func newRHELProvider(profile Profile) Provider {
	return &rhelProvider{
		profile: profile,
		provider: rhel.NewProvider(rhel.ProviderOptions{
			WebUser:            profile.WebUser,
			WebGroup:           profile.WebGroup,
			DefaultProjectRoot: profile.DefaultProjectRoot,
		}),
	}
}

// For returns the platform provider for a detected Info value.
func For(info *Info) (Provider, error) {
	if info == nil {
		return nil, fmt.Errorf("platform info is nil")
	}
	switch info.Family {
	case "debian":
		return newDebianProvider(info.Profile), nil
	case "rhel":
		if info.Profile.SupportLevel == SupportUnsupported {
			return nil, &UnsupportedError{Profile: info.Profile}
		}
		return newRHELProvider(info.Profile), nil
	default:
		return nil, &UnsupportedError{Profile: info.Profile}
	}
}

// Current detects the platform and returns its provider.
func Current() (Provider, *Info, error) {
	info, _, err := Detect()
	if err != nil {
		return nil, nil, err
	}
	provider, err := For(info)
	if err != nil {
		return nil, info, err
	}
	return provider, info, nil
}

// DebianDefaults returns the Debian-family provider using built-in defaults.
func DebianDefaults() Provider {
	return newDebianProvider(Profile{
		Family:             "debian",
		NginxLayout:        NginxSitesAvailableEnabled,
		NginxConfigDir:     "/etc/nginx/sites-available",
		WebUser:            "www-data",
		WebGroup:           "www-data",
		DefaultProjectRoot: "/var/www",
		PHPFPMStrategy:     "php{version}-fpm",
		PackageManager:     "apt",
		ServiceManager:     "systemd",
		FirewallStrategy:   "ufw",
		SupportLevel:       SupportOfficial,
	})
}

// RHELDefaults returns the RHEL-family provider using built-in defaults.
func RHELDefaults() Provider {
	return newRHELProvider(Profile{
		Family:             "rhel",
		DistroID:           "rocky",
		NginxLayout:        NginxConfD,
		NginxConfigDir:     "/etc/nginx/conf.d",
		WebUser:            "nginx",
		WebGroup:           "nginx",
		DefaultProjectRoot: "/var/www",
		PHPFPMStrategy:     "remi-php{mm}-php-fpm",
		PackageManager:     "dnf",
		ServiceManager:     "systemd",
		FirewallStrategy:   "firewalld",
		SupportLevel:       SupportOfficial,
	})
}
