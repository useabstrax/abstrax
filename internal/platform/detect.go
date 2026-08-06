package platform

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

var osReleasePath = "/etc/os-release"

// Detect inspects the running system and returns a populated Info and Tools.
func Detect() (*Info, *Tools, error) {
	info := &Info{}
	tools := &Tools{}

	rel, err := readOSRelease(osReleasePath)
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", osReleasePath, err)
	}

	profile := classifyRelease(rel)
	info.OSName = rel.ID
	info.OSVersion = rel.VersionID
	info.OSPrettyName = rel.PrettyName
	if info.OSPrettyName == "" {
		info.OSPrettyName = rel.Name
	}
	info.Profile = profile
	info.Family = profile.Family

	info.KernelVersion = kernelVersion()
	info.Architecture = architecture()
	info.PackageManager = detectPackageManager()
	info.ServiceManager = detectServiceManager()
	info.FirewallBackend = detectFirewallBackend()
	info.IsRoot = os.Getuid() == 0

	// Enrich profile with detected managers.
	info.Profile.PackageManager = info.PackageManager
	info.Profile.ServiceManager = info.ServiceManager
	info.Profile.FirewallStrategy = firewallStrategy(info.FirewallBackend, profile.Family)
	info.Profile.SELinuxStatus = detectSELinuxStatus()
	if info.Profile.DistroName == "" {
		info.Profile.DistroName = info.OSPrettyName
	}
	if info.Profile.DistroID == "" {
		info.Profile.DistroID = info.OSName
	}
	if info.Profile.VersionID == "" {
		info.Profile.VersionID = info.OSVersion
	}

	info.Supported = profile.Supported()
	info.SupportNote = profile.SupportNote

	detectTools(tools)

	return info, tools, nil
}

// ProfileFromOSRelease builds a platform profile from /etc/os-release content.
// This is primarily used in tests.
func ProfileFromOSRelease(content string) Profile {
	rel, err := parseOSReleaseReader(strings.NewReader(content))
	if err != nil {
		return Profile{
			Family:       "unknown",
			SupportLevel: SupportUnsupported,
			SupportNote:  "malformed /etc/os-release",
		}
	}
	return classifyRelease(rel)
}

func readOSRelease(path string) (OSRelease, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return OSRelease{}, nil
		}
		return OSRelease{}, err
	}
	defer f.Close()
	return parseOSReleaseReader(f)
}

func firewallStrategy(backend, family string) string {
	switch family {
	case "debian":
		if backend == "ufw" {
			return "ufw"
		}
		// Prefer the family default even when ufw is missing so doctor
		// and profiles report the intended strategy.
		return "ufw"
	case "rhel":
		if backend == "firewalld" {
			return "firewalld"
		}
		// Prefer the family default even when firewall-cmd is missing so doctor
		// and profiles report the intended strategy.
		return "firewalld"
	default:
		return "unknown"
	}
}

func detectSELinuxStatus() SELinuxStatus {
	if binExists("getenforce") {
		out, err := runQuiet("getenforce")
		if err == nil {
			switch strings.ToLower(strings.TrimSpace(out)) {
			case "enforcing":
				return SELinuxEnforcing
			case "permissive":
				return SELinuxPermissive
			case "disabled":
				return SELinuxDisabled
			}
		}
	}

	data, err := os.ReadFile("/sys/fs/selinux/enforce")
	if err != nil {
		if _, statErr := os.Stat("/etc/selinux/config"); statErr == nil {
			return SELinuxUnknown
		}
		return SELinuxDisabled
	}
	switch strings.TrimSpace(string(data)) {
	case "1":
		return SELinuxEnforcing
	case "0":
		return SELinuxPermissive
	default:
		return SELinuxUnknown
	}
}

func kernelVersion() string {
	out, err := runQuiet("uname", "-r")
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(out)
}

func architecture() string {
	out, err := runQuiet("uname", "-m")
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(out)
}

func detectPackageManager() string {
	switch {
	case binExists("apt"):
		return "apt"
	case binExists("dnf"):
		return "dnf"
	case binExists("yum"):
		return "yum"
	case binExists("apk"):
		return "apk"
	case binExists("pacman"):
		return "pacman"
	default:
		return "unknown"
	}
}

func detectServiceManager() string {
	if _, err := os.Stat("/run/systemd/private"); err == nil {
		return "systemd"
	}
	if _, err := os.Stat("/proc/1/comm"); err == nil {
		data, _ := os.ReadFile("/proc/1/comm")
		if strings.TrimSpace(string(data)) == "systemd" {
			return "systemd"
		}
	}
	if binExists("systemctl") {
		return "systemd"
	}
	if binExists("service") {
		return "sysvinit"
	}
	return "unknown"
}

func detectFirewallBackend() string {
	switch {
	case binExists("ufw"):
		return "ufw"
	case binExists("firewall-cmd"):
		return "firewalld"
	case binExists("iptables"):
		return "iptables"
	default:
		return "none"
	}
}

func detectTools(t *Tools) {
	t.Nginx = binExists("nginx")
	t.Apache2 = binExists("apache2") || binExists("httpd")
	t.Certbot = binExists("certbot")
	t.MySQL = binExists("mysql")
	t.MariaDB = binExists("mariadb")
	t.Supervisor = binExists("supervisorctl")
	t.Redis = binExists("redis-server") || binExists("redis")
	t.Memcached = binExists("memcached")
	t.UFW = binExists("ufw")
	t.Firewalld = binExists("firewall-cmd")
	t.Curl = binExists("curl")
	t.Git = binExists("git")
}

func binExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func runQuiet(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	return string(out), err
}

// RequireRoot returns an error if the process is not running as root (uid 0).
func RequireRoot() error {
	if syscall.Getuid() != 0 {
		return fmt.Errorf("this command requires root privileges; please run with sudo")
	}
	return nil
}

// RequireSupported returns an error when the current platform is unsupported.
func RequireSupported(info *Info) error {
	if info == nil {
		return fmt.Errorf("platform detection failed")
	}
	if info.Supported {
		return nil
	}
	return &UnsupportedError{Profile: info.Profile}
}

// ParseOSRelease is exported for tests that need raw key/value parsing.
func ParseOSRelease(r interface{ Read([]byte) (int, error) }) (map[string]string, error) {
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
	return kv, scanner.Err()
}
