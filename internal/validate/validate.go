// Package validate provides input validation helpers used by CLI commands and
// services.
package validate

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

var (
	usernameRe    = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	groupNameRe   = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	serviceNameRe = regexp.MustCompile(`^[a-zA-Z0-9_@:.-]{1,64}$`)
	packageNameRe = regexp.MustCompile(`^[a-zA-Z0-9_.+-]{1,128}$`)
	dbNameRe      = regexp.MustCompile(`^[a-zA-Z0-9_]{1,64}$`)
	cronIDRe      = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
	daemonNameRe  = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
	domainRe      = regexp.MustCompile(`^(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)
)

// Username validates a Linux username.
func Username(name string) error {
	if name == "" {
		return fmt.Errorf("username cannot be empty")
	}
	if !usernameRe.MatchString(name) {
		return fmt.Errorf("invalid username %q: must start with a letter or underscore, contain only lowercase letters, digits, underscores and hyphens, and be at most 32 characters", name)
	}
	return nil
}

// GroupName validates a Linux group name.
func GroupName(name string) error {
	if name == "" {
		return fmt.Errorf("group name cannot be empty")
	}
	if !groupNameRe.MatchString(name) {
		return fmt.Errorf("invalid group name %q", name)
	}
	return nil
}

// ServiceName validates a systemd service name.
func ServiceName(name string) error {
	if name == "" {
		return fmt.Errorf("service name cannot be empty")
	}
	if !serviceNameRe.MatchString(name) {
		return fmt.Errorf("invalid service name %q", name)
	}
	return nil
}

// PackageName validates an apt/deb package name.
func PackageName(name string) error {
	if name == "" {
		return fmt.Errorf("package name cannot be empty")
	}
	if !packageNameRe.MatchString(name) {
		return fmt.Errorf("invalid package name %q", name)
	}
	return nil
}

// DatabaseName validates a MySQL database name.
func DatabaseName(name string) error {
	if name == "" {
		return fmt.Errorf("database name cannot be empty")
	}
	if !dbNameRe.MatchString(name) {
		return fmt.Errorf("invalid database name %q: must contain only letters, digits, and underscores", name)
	}
	return nil
}

// MySQLUsername validates a MySQL username (max 32 chars on MySQL 5.7+, 16 on older).
func MySQLUsername(name string) error {
	if name == "" {
		return fmt.Errorf("mysql username cannot be empty")
	}
	if len(name) > 32 {
		return fmt.Errorf("mysql username %q exceeds 32 characters", name)
	}
	return nil
}

// CronID validates a cron job identifier.
func CronID(id string) error {
	if id == "" {
		return fmt.Errorf("cron ID cannot be empty")
	}
	if !cronIDRe.MatchString(id) {
		return fmt.Errorf("invalid cron ID %q: must contain only letters, digits, underscores and hyphens", id)
	}
	return nil
}

// DaemonName validates a supervisor daemon name.
func DaemonName(name string) error {
	if name == "" {
		return fmt.Errorf("daemon name cannot be empty")
	}
	if !daemonNameRe.MatchString(name) {
		return fmt.Errorf("invalid daemon name %q", name)
	}
	return nil
}

// Port validates a TCP/UDP port number.
func Port(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port %d is out of range (1-65535)", port)
	}
	return nil
}

// PortString validates a port given as a string.
func PortString(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("port must be a number, got %q", s)
	}
	return Port(n)
}

// IPAddress validates an IPv4 or IPv6 address.
func IPAddress(ip string) error {
	if net.ParseIP(ip) == nil {
		return fmt.Errorf("invalid IP address %q", ip)
	}
	return nil
}

// CIDRRange validates an IP address or CIDR range.
func CIDRRange(cidr string) error {
	if net.ParseIP(cidr) != nil {
		return nil
	}
	_, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("invalid IP address or CIDR range %q", cidr)
	}
	return nil
}

// Domain validates a domain name.
func Domain(d string) error {
	if d == "" {
		return fmt.Errorf("domain cannot be empty")
	}
	if !domainRe.MatchString(d) {
		return fmt.Errorf("invalid domain %q", d)
	}
	return nil
}

// FilePath performs a basic sanity check on a file path.
func FilePath(p string) error {
	if p == "" {
		return fmt.Errorf("path cannot be empty")
	}
	if strings.Contains(p, "\x00") {
		return fmt.Errorf("path contains null byte")
	}
	return nil
}

// CronExpression validates a cron schedule expression (5-field standard format).
func CronExpression(expr string) error {
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return fmt.Errorf("cron expression must have 5 fields (minute hour day month weekday), got %d", len(parts))
	}
	return nil
}

// Shell validates a login shell path.
func Shell(shell string) error {
	if shell == "" {
		return fmt.Errorf("shell cannot be empty")
	}
	if !strings.HasPrefix(shell, "/") {
		return fmt.Errorf("shell must be an absolute path, got %q", shell)
	}
	return nil
}

// ProjectName validates an Abstrax project name (alphanumeric, hyphens).
func ProjectName(name string) error {
	if name == "" {
		return fmt.Errorf("project name cannot be empty")
	}
	if !daemonNameRe.MatchString(name) {
		return fmt.Errorf("invalid project name %q: must contain only letters, digits, underscores and hyphens", name)
	}
	return nil
}
