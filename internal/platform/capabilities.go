package platform

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// DatabaseEngine identifies the MySQL-compatible database server Abstrax installs.
type DatabaseEngine string

const (
	DatabaseMySQL   DatabaseEngine = "mysql"
	DatabaseMariaDB DatabaseEngine = "mariadb"
)

// DatabaseAuthPlugin is the SQL auth plugin used when setting the root password.
type DatabaseAuthPlugin string

const (
	// DatabaseAuthCachingSHA2 is MySQL 8's default password plugin (Ubuntu mysql-server).
	DatabaseAuthCachingSHA2 DatabaseAuthPlugin = "caching_sha2_password"
	// DatabaseAuthNativePassword is MariaDB-compatible password auth (Debian/RHEL mariadb-server).
	DatabaseAuthNativePassword DatabaseAuthPlugin = "mysql_native_password"
)

var nodeMajorRE = regexp.MustCompile(`^[0-9]{1,2}$`)

// ValidateNodeMajor returns a sanitised Node.js major version string, or an error.
func ValidateNodeMajor(version string) (string, error) {
	major := strings.TrimSpace(strings.TrimPrefix(version, "v"))
	if i := strings.Index(major, "."); i >= 0 {
		major = major[:i]
	}
	if !nodeMajorRE.MatchString(major) {
		return "", fmt.Errorf("invalid Node.js major version %q", version)
	}
	n, err := strconv.Atoi(major)
	if err != nil || n < 12 || n > 30 {
		return "", fmt.Errorf("unsupported Node.js major version %q", version)
	}
	return major, nil
}

// NodeSourceSetupURL returns the NodeSource setup script URL for a family and major version.
func NodeSourceSetupURL(family, major string) (string, error) {
	maj, err := ValidateNodeMajor(major)
	if err != nil {
		return "", err
	}
	switch family {
	case "debian":
		return fmt.Sprintf("https://deb.nodesource.com/setup_%s.x", maj), nil
	case "rhel":
		return fmt.Sprintf("https://rpm.nodesource.com/setup_%s.x", maj), nil
	default:
		return "", fmt.Errorf("no NodeSource setup URL for family %q", family)
	}
}

// EnsureEPEL installs/enables EPEL on supported RHEL-family distros.
// It is a no-op on Debian-family systems. Prefer EnsureRepository with
// RepoOptions for callers that honour --enable-required-repos.
func EnsureEPEL(ctx context.Context, provider Provider, install func(ctx context.Context, name string) error, run func(ctx context.Context, name string, args ...string) error) error {
	if provider == nil || !provider.RequiresEPELForCertbot() {
		return nil
	}
	return EnsureRepository(ctx, provider, RepoEPEL, RepoOptions{}, DefaultRepoEnabler(install, run))
}

// EnsureEPELWithOptions is like EnsureEPEL but honours RepoOptions (including
// --enable-required-repos for RHEL/Oracle).
func EnsureEPELWithOptions(ctx context.Context, provider Provider, opts RepoOptions, enabler RepoEnabler) error {
	if provider == nil || !provider.RequiresEPELForCertbot() {
		return nil
	}
	return EnsureRepository(ctx, provider, RepoEPEL, opts, enabler)
}
