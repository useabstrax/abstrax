package platform

// SupportLevel describes how well Abstrax supports the detected platform.
type SupportLevel string

const (
	// SupportOfficial means the distro is fully tested and supported.
	SupportOfficial SupportLevel = "official"
	// SupportCompatible means the distro is Debian/Ubuntu-derived but not officially tested.
	SupportCompatible SupportLevel = "compatible"
	// SupportUnsupported means Abstrax does not support this platform.
	SupportUnsupported SupportLevel = "unsupported"
)

// NginxLayout describes how virtual host configuration is organised.
type NginxLayout string

const (
	// NginxSitesAvailableEnabled is the Debian/Ubuntu sites-available/sites-enabled layout.
	NginxSitesAvailableEnabled NginxLayout = "sites-available-enabled"
	// NginxLayoutUnknown means the layout could not be determined.
	NginxLayoutUnknown NginxLayout = "unknown"
)

// Profile describes platform capabilities and conventions used by Abstrax commands.
type Profile struct {
	DistroID           string       `json:"distro_id"`
	DistroName         string       `json:"distro_name"`
	VersionID          string       `json:"version_id"`
	Family             string       `json:"family"`
	PackageManager     string       `json:"package_manager"`
	ServiceManager     string       `json:"service_manager"`
	NginxLayout        NginxLayout  `json:"nginx_layout"`
	WebUser            string       `json:"web_user"`
	WebGroup           string       `json:"web_group"`
	DefaultProjectRoot string       `json:"default_project_root"`
	PHPFPMStrategy     string       `json:"php_fpm_strategy"`
	FirewallStrategy   string       `json:"firewall_strategy"`
	SupportLevel       SupportLevel `json:"support_level"`
	SupportNote        string       `json:"support_note,omitempty"`
}

// Supported returns true when Abstrax allows mutating commands on this platform.
func (p Profile) Supported() bool {
	return p.SupportLevel != SupportUnsupported
}

// OfficiallySupported returns true when the platform is in the fully supported set.
func (p Profile) OfficiallySupported() bool {
	return p.SupportLevel == SupportOfficial
}
