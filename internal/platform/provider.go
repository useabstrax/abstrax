package platform

import (
	"fmt"

	"abstrax/internal/platform/debian"
)

// Provider exposes platform-specific paths and naming conventions.
type Provider interface {
	Profile() Profile
	Paths() debian.Paths
	WebUser() string
	WebGroup() string
	DefaultProjectRoot() string
	PHPFPMServiceName(version string) string
	PHPFPMBinary(version string) string
	PHPFPMPoolDir(version string) string
	PHPFPMDefaultSocket(version string) string
	PHPFPMProjectSocket(version, poolSuffix string) string
	PHPPackageNames(version string, extensions []string) []string
	NginxSitesAvailable() string
	NginxSitesEnabled() string
	NginxConfPath() string
	NginxSitesEnabledInclude() string
	SudoGroup() string
}

type debianProvider struct {
	profile  Profile
	provider *debian.Provider
}

func (p *debianProvider) Profile() Profile           { return p.profile }
func (p *debianProvider) Paths() debian.Paths        { return p.provider.Paths() }
func (p *debianProvider) WebUser() string            { return p.provider.WebUser() }
func (p *debianProvider) WebGroup() string           { return p.provider.WebGroup() }
func (p *debianProvider) DefaultProjectRoot() string { return p.provider.DefaultProjectRoot() }
func (p *debianProvider) PHPFPMServiceName(version string) string {
	return p.provider.PHPFPMServiceName(version)
}
func (p *debianProvider) PHPFPMBinary(version string) string { return p.provider.PHPFPMBinary(version) }
func (p *debianProvider) PHPFPMPoolDir(version string) string {
	return p.provider.PHPFPMPoolDir(version)
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
func (p *debianProvider) NginxSitesAvailable() string { return p.provider.NginxSitesAvailable() }
func (p *debianProvider) NginxSitesEnabled() string   { return p.provider.NginxSitesEnabled() }
func (p *debianProvider) NginxConfPath() string       { return p.provider.NginxConfPath() }
func (p *debianProvider) NginxSitesEnabledInclude() string {
	return p.provider.NginxSitesEnabledInclude()
}
func (p *debianProvider) SudoGroup() string { return p.provider.SudoGroup() }

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

// For returns the platform provider for a detected Info value.
func For(info *Info) (Provider, error) {
	if info == nil {
		return nil, fmt.Errorf("platform info is nil")
	}
	switch info.Family {
	case "debian":
		return newDebianProvider(info.Profile), nil
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
