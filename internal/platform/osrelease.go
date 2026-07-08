package platform

import (
	"bufio"
	"io"
	"strings"
)

// OSRelease holds parsed fields from /etc/os-release.
type OSRelease struct {
	ID         string
	IDLike     string
	Name       string
	PrettyName string
	VersionID  string
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
		ID:         strings.ToLower(kv["ID"]),
		IDLike:     strings.ToLower(kv["ID_LIKE"]),
		Name:       kv["NAME"],
		PrettyName: kv["PRETTY_NAME"],
		VersionID:  kv["VERSION_ID"],
	}, nil
}

func classifyRelease(rel OSRelease) Profile {
	profile := Profile{
		DistroID:         rel.ID,
		DistroName:       rel.PrettyName,
		VersionID:        rel.VersionID,
		Family:           detectFamily(rel),
		NginxLayout:      NginxLayoutUnknown,
		PHPFPMStrategy:   "unknown",
		FirewallStrategy: "unknown",
	}

	if profile.DistroName == "" {
		profile.DistroName = rel.Name
	}
	if profile.DistroName == "" && rel.ID != "" {
		profile.DistroName = rel.ID
	}

	profile.SupportLevel, profile.SupportNote = classifySupport(rel, profile.Family)
	applyDebianFamilyDefaults(&profile)

	return profile
}

func detectFamily(rel OSRelease) string {
	id := normalizeDistroID(rel.ID)
	switch id {
	case "ubuntu", "debian", "linuxmint", "pop", "raspbian", "raspberrypi":
		return "debian"
	}

	like := rel.IDLike
	if strings.Contains(like, "debian") || strings.Contains(like, "ubuntu") {
		return "debian"
	}

	return "unknown"
}

func normalizeDistroID(id string) string {
	switch strings.ToLower(id) {
	case "raspberry_pi", "raspberry-pi":
		return "raspberrypi"
	default:
		return strings.ToLower(id)
	}
}

func classifySupport(rel OSRelease, family string) (SupportLevel, string) {
	if family != "debian" {
		if rel.ID == "" {
			return SupportUnsupported, "could not detect operating system from /etc/os-release"
		}
		return SupportUnsupported, unsupportedMessage(rel)
	}

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

func unsupportedMessage(rel OSRelease) string {
	name := rel.PrettyName
	if name == "" {
		name = rel.ID
	}
	return "operating system " + quote(name) + " is not supported; Abstrax currently supports Debian/Ubuntu-based distributions"
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

func applyDebianFamilyDefaults(profile *Profile) {
	if profile.Family != "debian" {
		return
	}
	profile.NginxLayout = NginxSitesAvailableEnabled
	profile.WebUser = "www-data"
	profile.WebGroup = "www-data"
	profile.DefaultProjectRoot = "/var/www"
	profile.PHPFPMStrategy = "php{version}-fpm"
}
