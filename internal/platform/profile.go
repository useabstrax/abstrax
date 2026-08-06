package platform

// SupportLevel describes how well Abstrax supports the detected platform.
type SupportLevel string

const (
	// SupportOfficial means the distro is fully tested and supported.
	SupportOfficial SupportLevel = "official"
	// SupportCompatible means the distro is family-compatible but not officially tested.
	// Used for experimental / best-effort support (for example RHEL 9+ and CentOS Stream 9+).
	SupportCompatible SupportLevel = "compatible"
	// SupportUnsupported means Abstrax does not support this platform.
	SupportUnsupported SupportLevel = "unsupported"
)

// NginxLayout describes how virtual host configuration is organised.
type NginxLayout string

const (
	// NginxSitesAvailableEnabled is the Debian/Ubuntu sites-available/sites-enabled layout.
	NginxSitesAvailableEnabled NginxLayout = "sites-available-enabled"
	// NginxConfD is the RHEL-family conf.d layout.
	NginxConfD NginxLayout = "conf.d"
	// NginxLayoutUnknown means the layout could not be determined.
	NginxLayoutUnknown NginxLayout = "unknown"
)

// SELinuxStatus describes the detected SELinux mode.
type SELinuxStatus string

const (
	// SELinuxDisabled means SELinux is not present or is disabled.
	SELinuxDisabled SELinuxStatus = "disabled"
	// SELinuxPermissive means SELinux is permissive.
	SELinuxPermissive SELinuxStatus = "permissive"
	// SELinuxEnforcing means SELinux is enforcing.
	SELinuxEnforcing SELinuxStatus = "enforcing"
	// SELinuxUnknown means SELinux status could not be determined.
	SELinuxUnknown SELinuxStatus = "unknown"
)

// Profile describes platform capabilities and conventions used by Abstrax commands.
type Profile struct {
	DistroID           string        `json:"distro_id"`
	DistroName         string        `json:"distro_name"`
	VersionID          string        `json:"version_id"`
	VersionCodename    string        `json:"version_codename,omitempty"`
	UbuntuCodename     string        `json:"ubuntu_codename,omitempty"`
	Family             string        `json:"family"`
	PackageManager     string        `json:"package_manager"`
	ServiceManager     string        `json:"service_manager"`
	NginxLayout        NginxLayout   `json:"nginx_layout"`
	NginxConfigDir     string        `json:"nginx_config_dir,omitempty"`
	WebUser            string        `json:"web_user"`
	WebGroup           string        `json:"web_group"`
	DefaultProjectRoot string        `json:"default_project_root"`
	PHPFPMStrategy     string        `json:"php_fpm_strategy"`
	FirewallStrategy   string        `json:"firewall_strategy"`
	SELinuxStatus      SELinuxStatus `json:"selinux_status,omitempty"`
	SupportLevel       SupportLevel  `json:"support_level"`
	SupportNote        string        `json:"support_note,omitempty"`
}

// Supported returns true when Abstrax allows mutating commands on this platform.
func (p Profile) Supported() bool {
	return p.SupportLevel != SupportUnsupported
}

// OfficiallySupported returns true when the platform is in the fully supported set.
func (p Profile) OfficiallySupported() bool {
	return p.SupportLevel == SupportOfficial
}

// IsRHELFamily returns true when the profile belongs to the RHEL-compatible family.
func (p Profile) IsRHELFamily() bool {
	return p.Family == "rhel"
}

// IsDebianFamily returns true when the profile belongs to the Debian/Ubuntu family.
func (p Profile) IsDebianFamily() bool {
	return p.Family == "debian"
}
