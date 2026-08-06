package platform

import (
	"bufio"
	"io"
	"strings"
)

// OSRelease holds parsed fields from /etc/os-release.
type OSRelease struct {
	ID              string
	IDLike          string
	Name            string
	PrettyName      string
	VersionID       string
	VersionCodename string
	UbuntuCodename  string
}

func parseOSReleaseReader(r io.Reader) (OSRelease, error) {
	kv := make(map[string]string)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		kv[parts[0]] = strings.Trim(parts[1], `"`)
	}
	if err := scanner.Err(); err != nil {
		return OSRelease{}, err
	}

	return OSRelease{
		ID:              strings.ToLower(kv["ID"]),
		IDLike:          strings.ToLower(kv["ID_LIKE"]),
		Name:            kv["NAME"],
		PrettyName:      kv["PRETTY_NAME"],
		VersionID:       kv["VERSION_ID"],
		VersionCodename: strings.ToLower(kv["VERSION_CODENAME"]),
		UbuntuCodename:  strings.ToLower(kv["UBUNTU_CODENAME"]),
	}, nil
}

func classifyRelease(rel OSRelease) Profile {
	profile := Profile{
		DistroID:         rel.ID,
		DistroName:       rel.PrettyName,
		VersionID:        rel.VersionID,
		VersionCodename:  rel.VersionCodename,
		UbuntuCodename:   rel.UbuntuCodename,
		Family:           detectFamily(rel),
		NginxLayout:      NginxLayoutUnknown,
		PHPFPMStrategy:   "unknown",
		FirewallStrategy: "unknown",
		SELinuxStatus:    SELinuxUnknown,
	}

	if profile.DistroName == "" {
		profile.DistroName = rel.Name
	}
	if profile.DistroName == "" && rel.ID != "" {
		profile.DistroName = rel.ID
	}

	profile.SupportLevel, profile.SupportNote = classifySupport(rel, profile.Family)
	applyFamilyDefaults(&profile)

	return profile
}

func detectFamily(rel OSRelease) string {
	id := normalizeDistroID(rel.ID)
	switch id {
	case "ubuntu", "debian", "linuxmint", "pop", "raspbian", "raspberrypi":
		return "debian"
	case "rocky", "almalinux", "rhel", "centos", "ol", "oracle":
		return "rhel"
	}

	like := rel.IDLike
	if strings.Contains(like, "debian") || strings.Contains(like, "ubuntu") {
		return "debian"
	}
	// Generic RHEL-compatible derivatives via ID_LIKE, excluding Fedora itself.
	if id != "fedora" && (strings.Contains(like, "rhel") || strings.Contains(like, "centos")) {
		return "rhel"
	}

	return "unknown"
}

func normalizeDistroID(id string) string {
	switch strings.ToLower(id) {
	case "raspberry_pi", "raspberry-pi":
		return "raspberrypi"
	case "oraclelinux", "oracle":
		return "ol"
	default:
		return strings.ToLower(id)
	}
}

func classifySupport(rel OSRelease, family string) (SupportLevel, string) {
	switch family {
	case "debian":
		return classifyDebianSupport(rel)
	case "rhel":
		return classifyRHELSupport(rel)
	default:
		if rel.ID == "" {
			return SupportUnsupported, "could not detect operating system from /etc/os-release"
		}
		return SupportUnsupported, unsupportedMessage(rel)
	}
}

func classifyDebianSupport(rel OSRelease) (SupportLevel, string) {
	id := normalizeDistroID(rel.ID)
	switch id {
	case "ubuntu":
		if ubuntuVersionAtLeast(rel.VersionID, 20, 4) {
			return SupportOfficial, ""
		}
		return SupportCompatible, "Ubuntu " + rel.VersionID + " is Debian-family but only Ubuntu 20.04+ is officially supported"
	case "debian":
		if debianVersionAtLeast(rel.VersionID, 11) {
			return SupportOfficial, ""
		}
		return SupportCompatible, "Debian " + rel.VersionID + " is Debian-family but only Debian 11+ is officially supported"
	case "linuxmint", "pop", "raspbian", "raspberrypi":
		return SupportOfficial, ""
	case "":
		return SupportCompatible, "Debian/Ubuntu-derived system detected via ID_LIKE; this distribution is not officially tested"
	default:
		return SupportCompatible, "Debian/Ubuntu-derived distribution " + quote(id) + " is not officially tested; commands may work on apt/systemd-based systems"
	}
}

func classifyRHELSupport(rel OSRelease) (SupportLevel, string) {
	id := normalizeDistroID(rel.ID)
	majorOK := debianVersionAtLeast(rel.VersionID, 9)

	switch id {
	case "rocky":
		if majorOK {
			return SupportOfficial, ""
		}
		return SupportUnsupported, rhelUnsupportedVersionNote(rel, "Rocky Linux")
	case "almalinux":
		if majorOK {
			return SupportOfficial, ""
		}
		return SupportUnsupported, rhelUnsupportedVersionNote(rel, "AlmaLinux")
	case "rhel":
		if majorOK {
			return SupportCompatible, "Red Hat Enterprise Linux " + rel.VersionID + " is experimental; Rocky Linux 9+ and AlmaLinux 9+ are the officially supported RHEL-family targets"
		}
		return SupportUnsupported, rhelUnsupportedVersionNote(rel, "Red Hat Enterprise Linux")
	case "centos":
		if majorOK {
			return SupportCompatible, "CentOS Stream " + rel.VersionID + " is experimental; Rocky Linux 9+ and AlmaLinux 9+ are the officially supported RHEL-family targets"
		}
		return SupportUnsupported, rhelUnsupportedVersionNote(rel, "CentOS")
	case "ol":
		if majorOK {
			return SupportCompatible, "Oracle Linux " + rel.VersionID + " is experimental; Rocky Linux 9+ and AlmaLinux 9+ are the officially supported RHEL-family targets"
		}
		return SupportUnsupported, rhelUnsupportedVersionNote(rel, "Oracle Linux")
	default:
		if majorOK {
			return SupportCompatible, "RHEL-compatible distribution " + quote(id) + " is experimental; Rocky Linux 9+ and AlmaLinux 9+ are the officially supported targets"
		}
		return SupportUnsupported, rhelUnsupportedVersionNote(rel, rel.PrettyName)
	}
}

func rhelUnsupportedVersionNote(rel OSRelease, label string) string {
	if label == "" {
		label = rel.ID
	}
	version := rel.VersionID
	if version == "" {
		version = "unknown"
	}
	return label + " " + version + " is not supported; Abstrax officially supports Rocky Linux 9+ and AlmaLinux 9+ (RHEL 9+, CentOS Stream 9+, and Oracle Linux 9+ are experimental)"
}

func unsupportedMessage(rel OSRelease) string {
	name := rel.PrettyName
	if name == "" {
		name = rel.ID
	}
	return "operating system " + quote(name) + " is not supported; Abstrax currently supports Debian/Ubuntu-based and RHEL-compatible distributions"
}

func quote(s string) string {
	if s == "" {
		return "unknown"
	}
	return "\"" + s + "\""
}

func ubuntuVersionAtLeast(versionID string, major, minor int) bool {
	maj, min, ok := parseVersionID(versionID)
	if !ok {
		return false
	}
	if maj > major {
		return true
	}
	return maj == major && min >= minor
}

func debianVersionAtLeast(versionID string, major int) bool {
	maj, _, ok := parseVersionID(versionID)
	if !ok {
		return false
	}
	return maj >= major
}

func parseVersionID(versionID string) (major, minor int, ok bool) {
	versionID = strings.TrimSpace(versionID)
	if versionID == "" {
		return 0, 0, false
	}

	parts := strings.Split(versionID, ".")
	majorPart := parts[0]
	minorPart := "0"
	if len(parts) > 1 {
		minorPart = parts[1]
	}

	major, ok = parseVersionComponent(majorPart)
	if !ok {
		return 0, 0, false
	}
	minor, ok = parseVersionComponent(minorPart)
	if !ok {
		return major, 0, true
	}
	return major, minor, true
}

func parseVersionComponent(part string) (int, bool) {
	part = strings.TrimSpace(part)
	if part == "" {
		return 0, false
	}
	n := 0
	for _, r := range part {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, true
}

func applyFamilyDefaults(profile *Profile) {
	applyDebianFamilyDefaults(profile)
	applyRHELFamilyDefaults(profile)
}

func applyDebianFamilyDefaults(profile *Profile) {
	if profile.Family != "debian" {
		return
	}
	profile.NginxLayout = NginxSitesAvailableEnabled
	profile.NginxConfigDir = "/etc/nginx/sites-available"
	profile.WebUser = "www-data"
	profile.WebGroup = "www-data"
	profile.DefaultProjectRoot = "/var/www"
	profile.PHPFPMStrategy = "php{version}-fpm"
	profile.PackageManager = "apt"
	profile.ServiceManager = "systemd"
	profile.FirewallStrategy = "ufw"
}

func applyRHELFamilyDefaults(profile *Profile) {
	if profile.Family != "rhel" {
		return
	}
	profile.NginxLayout = NginxConfD
	profile.NginxConfigDir = "/etc/nginx/conf.d"
	profile.WebUser = "nginx"
	profile.WebGroup = "nginx"
	profile.DefaultProjectRoot = "/var/www"
	profile.PHPFPMStrategy = "remi-php{mm}-php-fpm"
	profile.PackageManager = "dnf"
	profile.ServiceManager = "systemd"
	profile.FirewallStrategy = "firewalld"
}
