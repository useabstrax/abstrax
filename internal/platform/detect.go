package platform

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// Detect inspects the running system and returns a populated Info and Tools.
func Detect() (*Info, *Tools, error) {
	info := &Info{}
	tools := &Tools{}

	if err := parseOSRelease(info); err != nil {
		return nil, nil, fmt.Errorf("reading /etc/os-release: %w", err)
	}

	info.KernelVersion = kernelVersion()
	info.Architecture = architecture()
	info.PackageManager = detectPackageManager()
	info.ServiceManager = detectServiceManager()
	info.FirewallBackend = detectFirewallBackend()
	info.IsRoot = os.Getuid() == 0

	info.Supported, info.SupportNote = isSupported(info)

	detectTools(tools)

	return info, tools, nil
}

func parseOSRelease(info *Info) error {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		// Not a Linux system – still try to continue.
		info.OSName = "unknown"
		info.OSVersion = "unknown"
		info.OSPrettyName = "unknown"
		return nil
	}
	defer f.Close()

	kv := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]
		val := strings.Trim(parts[1], `"`)
		kv[key] = val
	}

	info.OSName = kv["ID"]
	info.OSVersion = kv["VERSION_ID"]
	info.OSPrettyName = kv["PRETTY_NAME"]
	return scanner.Err()
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
	// Check for systemd via the PID 1 process name.
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

func isSupported(info *Info) (bool, string) {
	switch info.OSName {
	case "ubuntu", "debian", "linuxmint", "pop", "raspbian":
		return true, ""
	case "":
		return false, "could not detect OS"
	default:
		return false, fmt.Sprintf("OS %q is not fully supported; Abstrax targets Debian and Ubuntu based systems", info.OSName)
	}
}

func detectTools(t *Tools) {
	t.Nginx = binExists("nginx")
	t.Apache2 = binExists("apache2") || binExists("httpd")
	t.Certbot = binExists("certbot")
	t.MySQL = binExists("mysql")
	t.MariaDB = binExists("mariadb")
	t.Supervisor = binExists("supervisorctl")
	t.Redis = binExists("redis-server")
	t.Memcached = binExists("memcached")
	t.UFW = binExists("ufw")
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

// RequireSupported returns an error when the current platform is not in the
// supported set.
func RequireSupported(info *Info) error {
	if !info.Supported {
		if info.SupportNote != "" {
			return fmt.Errorf("unsupported platform: %s", info.SupportNote)
		}
		return fmt.Errorf("unsupported platform: %s", info.OSName)
	}
	return nil
}
