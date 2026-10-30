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
	pluginNameRe  = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	projectNameRe = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,64}$`)
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

var cronMacros = map[string]struct{}{
	"@reboot":   {},
	"@yearly":   {},
	"@annually": {},
	"@monthly":  {},
	"@weekly":   {},
	"@daily":    {},
	"@midnight": {},
	"@hourly":   {},
}

var cronMonthNames = map[string]int{
	"JAN": 1, "FEB": 2, "MAR": 3, "APR": 4, "MAY": 5, "JUN": 6,
	"JUL": 7, "AUG": 8, "SEP": 9, "OCT": 10, "NOV": 11, "DEC": 12,
}

var cronWeekdayNames = map[string]int{
	"SUN": 0, "MON": 1, "TUE": 2, "WED": 3, "THU": 4, "FRI": 5, "SAT": 6,
}

// CronExpression validates a five-field cron schedule or a standard @ macro.
func CronExpression(expr string) error {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return fmt.Errorf("cron expression is empty")
	}
	if strings.HasPrefix(expr, "@") {
		if _, ok := cronMacros[strings.ToLower(expr)]; ok {
			return nil
		}
		return fmt.Errorf("unknown cron macro %q", expr)
	}

	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return fmt.Errorf("cron expression must have 5 fields (minute hour day month weekday), got %d", len(parts))
	}

	fields := []struct {
		name string
		min  int
		max  int
	}{
		{"minute", 0, 59},
		{"hour", 0, 23},
		{"day", 1, 31},
		{"month", 1, 12},
		{"weekday", 0, 7},
	}
	for i, field := range fields {
		if err := validateCronField(field.name, parts[i], field.min, field.max); err != nil {
			return err
		}
	}
	return nil
}

func validateCronField(name, field string, min, max int) error {
	if field == "" {
		return fmt.Errorf("%s: empty value", name)
	}
	for _, item := range strings.Split(field, ",") {
		if item == "" {
			return fmt.Errorf("%s: empty list entry in %q", name, field)
		}
		if err := validateCronItem(name, item, min, max); err != nil {
			return err
		}
	}
	return nil
}

func validateCronItem(name, item string, min, max int) error {
	body := item
	if i := strings.Index(item, "/"); i >= 0 {
		body = item[:i]
		step := item[i+1:]
		n, err := strconv.Atoi(step)
		if err != nil || n < 1 {
			return fmt.Errorf("%s: step %q must be 1 or more", name, step)
		}
		if body == "*" {
			return nil
		}
	}
	if body == "*" {
		return nil
	}
	if strings.Contains(body, "-") {
		bounds := strings.SplitN(body, "-", 2)
		if bounds[0] == "" || bounds[1] == "" {
			return fmt.Errorf("%s: invalid range %q", name, body)
		}
		start, err := cronFieldValue(name, bounds[0], min, max)
		if err != nil {
			return err
		}
		end, err := cronFieldValue(name, bounds[1], min, max)
		if err != nil {
			return err
		}
		if start > end {
			return fmt.Errorf("%s: range %s is reversed", name, body)
		}
		return nil
	}
	_, err := cronFieldValue(name, body, min, max)
	return err
}

func cronFieldValue(name, raw string, min, max int) (int, error) {
	if n, err := strconv.Atoi(raw); err == nil {
		if n < min || n > max {
			return 0, fmt.Errorf("%s: %d is out of range (%d-%d)", name, n, min, max)
		}
		return n, nil
	}
	upper := strings.ToUpper(raw)
	switch name {
	case "month":
		if n, ok := cronMonthNames[upper]; ok {
			return n, nil
		}
	case "weekday":
		if n, ok := cronWeekdayNames[upper]; ok {
			return n, nil
		}
	}
	return 0, fmt.Errorf("%s: invalid value %q", name, raw)
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

// ProjectName validates an Abstrax project name (alphanumeric, hyphens, periods).
func ProjectName(name string) error {
	if name == "" {
		return fmt.Errorf("project name cannot be empty")
	}
	if !projectNameRe.MatchString(name) {
		return fmt.Errorf("invalid project name %q: must contain only letters, digits, underscores, hyphens and periods", name)
	}
	return nil
}

// PluginName validates an Abstrax plugin command name.
func PluginName(name string) error {
	if name == "" {
		return fmt.Errorf("plugin name cannot be empty")
	}
	if !pluginNameRe.MatchString(name) {
		return fmt.Errorf("invalid plugin name %q: must start with a lowercase letter and contain only lowercase letters, digits and hyphens", name)
	}
	return nil
}
