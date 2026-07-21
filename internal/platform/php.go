package platform

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"abstrax/internal/platform/rhel"
)

var phpVersionRE = regexp.MustCompile(`^([0-9]+)\.([0-9]+)$`)

// NormalizePHPVersion trims a leading "php" prefix and validates major.minor form.
func NormalizePHPVersion(version string) (string, error) {
	v := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(version), "php"))
	v = strings.TrimSpace(v)
	if v == "" {
		return "", fmt.Errorf("PHP version is required")
	}
	m := phpVersionRE.FindStringSubmatch(v)
	if m == nil {
		return "", fmt.Errorf("invalid PHP version %q; expected major.minor (for example 8.3)", version)
	}
	return m[1] + "." + m[2], nil
}

// RemiShortCode converts "8.3" to "83" for Remi SCL package naming.
func RemiShortCode(version string) (string, error) {
	v, err := NormalizePHPVersion(version)
	if err != nil {
		return "", err
	}
	parts := strings.Split(v, ".")
	return parts[0] + parts[1], nil
}

// SupportedRemiPHPVersions lists PHP versions Abstrax will install via Remi SCL on RHEL-family.
var SupportedRemiPHPVersions = rhel.SupportedRemiPHPVersions

// SupportsRemiPHPVersion reports whether version is in the Remi SCL support matrix.
func SupportsRemiPHPVersion(version string) bool {
	v, err := NormalizePHPVersion(version)
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

// RemiModuleName returns the Remi module/stream name for a PHP version (for example "php83").
func RemiModuleName(version string) (string, error) {
	code, err := RemiShortCode(version)
	if err != nil {
		return "", err
	}
	return "php" + code, nil
}

// ParsePHPMajorMinor returns major and minor integers for a PHP version string.
func ParsePHPMajorMinor(version string) (major, minor int, err error) {
	v, err := NormalizePHPVersion(version)
	if err != nil {
		return 0, 0, err
	}
	parts := strings.Split(v, ".")
	major, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, err
	}
	return major, minor, nil
}
