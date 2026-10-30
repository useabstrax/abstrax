package sshcfg

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"abstrax/internal/platform"
)

// UseFirewall supplies the firewall open and close operations.
// sshcfg cannot import the firewall service directly.
func (s *Service) UseFirewall(open, close func(ctx context.Context, port int) error) {
	s.openFirewallFn = open
	s.closeFirewallFn = close
}

const defaultSocketDropIn = "/etc/systemd/system/ssh.socket.d/abstrax-port.conf"

// SetPort changes the SSH port without dropping the current listener until asked.
func (s *Service) SetPort(ctx context.Context, opts SetPortOptions) (*SetPortResult, error) {
	if opts.Port < 1 || opts.Port > 65535 {
		return nil, fmt.Errorf("port %d is out of range", opts.Port)
	}

	oldPorts := s.currentPorts()
	if len(oldPorts) == 0 {
		oldPorts = []int{22}
	}

	if opts.AllowFirewall {
		if err := s.openFirewall(ctx, opts.Port, opts.DryRun); err != nil {
			return nil, fmt.Errorf("opening firewall port %d: %w", opts.Port, err)
		}
	}

	if !opts.DryRun && s.selinuxEnforcing() {
		if err := s.labelSSHPort(ctx, opts.Port); err != nil {
			return nil, err
		}
	}

	snap, err := s.snapshot()
	if err != nil {
		return nil, err
	}

	both := append([]int{}, oldPorts...)
	if !containsPort(both, opts.Port) {
		both = append(both, opts.Port)
	}
	socket := s.socketActive(ctx)
	if err := s.applyListen(both, socket); err != nil {
		s.restore(snap)
		return nil, err
	}
	if err := s.activate(ctx, opts, socket); err != nil {
		s.restore(snap)
		_ = s.reloadRestored(ctx, socket)
		return nil, fmt.Errorf("SSH did not stay reachable on port %d: %w", opts.Port, err)
	}

	if opts.CloseOldPort {
		if err := s.applyListen([]int{opts.Port}, socket); err != nil {
			s.restore(snap)
			_ = s.reloadRestored(ctx, socket)
			return nil, err
		}
		if err := s.activate(ctx, opts, socket); err != nil {
			s.restore(snap)
			_ = s.reloadRestored(ctx, socket)
			return nil, fmt.Errorf("closing the old SSH port failed: %w", err)
		}
		if opts.AllowFirewall {
			for _, old := range oldPorts {
				if old == opts.Port {
					continue
				}
				if err := s.closeFirewall(ctx, old, opts.DryRun); err != nil {
					return nil, fmt.Errorf("closing firewall port %d: %w", old, err)
				}
			}
		}
	}

	kept := !opts.CloseOldPort && len(otherPorts(oldPorts, opts.Port)) > 0
	msg := fmt.Sprintf("SSH port set to %d.", opts.Port)
	if kept {
		msg += fmt.Sprintf(" Port %s is still open so you can test a connection, then re-run with --close-old-port.", formatPorts(otherPorts(oldPorts, opts.Port)))
	}
	if !opts.AllowFirewall {
		msg += " The firewall was not changed."
	}
	return &SetPortResult{
		Port:            opts.Port,
		PreviousPorts:   oldPorts,
		OldPortKept:     kept,
		FirewallChanged: opts.AllowFirewall,
		Message:         msg,
	}, nil
}

func (s *Service) applyListen(ports []int, socket bool) error {
	if err := s.writePorts(ports); err != nil {
		return err
	}
	if socket {
		return s.writeSocketDropIn(ports)
	}
	return nil
}

func (s *Service) activate(ctx context.Context, opts SetPortOptions, socket bool) error {
	if opts.DryRun {
		return nil
	}
	if err := s.validateSSHD(ctx); err != nil {
		return err
	}
	if socket {
		if err := s.restartSocket(ctx); err != nil {
			return err
		}
	} else if err := s.reloadSSH(ctx); err != nil {
		return err
	}
	return s.checkBanner(opts.Port)
}

func (s *Service) currentPorts() []int {
	if ports := portsFromFile(s.managedConfigPath()); len(ports) > 0 {
		return ports
	}
	return portsFromFile(s.mainConfig())
}

func (s *Service) managedConfigPath() string {
	if s.managedPath != "" {
		return s.managedPath
	}
	return platform.Resolve().Paths().AbstraxSSHConfig
}

func (s *Service) mainConfig() string {
	if s.mainConfigPath != "" {
		return s.mainConfigPath
	}
	return sshdConfigPath
}

func (s *Service) socketDropInPath() string {
	if s.socketDropIn != "" {
		return s.socketDropIn
	}
	return defaultSocketDropIn
}

func portsFromFile(path string) []int {
	entries, err := readConfigFile(path)
	if err != nil {
		return nil
	}
	var ports []int
	for _, entry := range entries {
		if !strings.EqualFold(entry.Key, "port") {
			continue
		}
		n, err := strconv.Atoi(entry.Value)
		if err == nil {
			ports = append(ports, n)
		}
	}
	return ports
}

func (s *Service) writePorts(ports []int) error {
	path := s.managedConfigPath()
	entries, _ := readConfigFile(path)
	var kept []ConfigEntry
	for _, entry := range entries {
		if strings.EqualFold(entry.Key, "port") {
			continue
		}
		kept = append(kept, entry)
	}
	for _, port := range ports {
		kept = append(kept, ConfigEntry{Key: "Port", Value: strconv.Itoa(port)})
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return writeConfigFile(path, kept)
}

func (s *Service) writeSocketDropIn(ports []int) error {
	path := s.socketDropInPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	var sb strings.Builder
	sb.WriteString("# Managed by Abstrax - do not edit manually\n[Socket]\nListenStream=\n")
	for _, port := range ports {
		sb.WriteString(fmt.Sprintf("ListenStream=%d\n", port))
	}
	return os.WriteFile(path, []byte(sb.String()), 0644)
}

type configSnapshot struct {
	managed   []byte
	managedOK bool
	dropIn    []byte
	dropInOK  bool
}

func (s *Service) snapshot() (configSnapshot, error) {
	var snap configSnapshot
	data, err := os.ReadFile(s.managedConfigPath())
	if err == nil {
		snap.managed = data
		snap.managedOK = true
	} else if !os.IsNotExist(err) {
		return snap, err
	}
	data, err = os.ReadFile(s.socketDropInPath())
	if err == nil {
		snap.dropIn = data
		snap.dropInOK = true
	} else if !os.IsNotExist(err) {
		return snap, err
	}
	return snap, nil
}

func (s *Service) restore(snap configSnapshot) {
	restoreFile(s.managedConfigPath(), snap.managed, snap.managedOK)
	restoreFile(s.socketDropInPath(), snap.dropIn, snap.dropInOK)
}

func restoreFile(path string, data []byte, existed bool) {
	if !existed {
		_ = os.Remove(path)
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	_ = os.WriteFile(path, data, 0644)
}

func (s *Service) openFirewall(ctx context.Context, port int, _ bool) error {
	if s.openFirewallFn == nil {
		return fmt.Errorf("firewall opener is not configured")
	}
	return s.openFirewallFn(ctx, port)
}

func (s *Service) closeFirewall(ctx context.Context, port int, _ bool) error {
	if s.closeFirewallFn == nil {
		return fmt.Errorf("firewall closer is not configured")
	}
	return s.closeFirewallFn(ctx, port)
}

func (s *Service) selinuxEnforcing() bool {
	if s.selinuxEnforcingFn != nil {
		return s.selinuxEnforcingFn()
	}
	out, err := exec.Command("getenforce").Output()
	return err == nil && strings.EqualFold(strings.TrimSpace(string(out)), "Enforcing")
}

func (s *Service) labelSSHPort(ctx context.Context, port int) error {
	if s.labelFn != nil {
		return s.labelFn(ctx, port)
	}
	portStr := strconv.Itoa(port)
	_, err := s.runner.Run(ctx, "semanage", "port", "-a", "-t", "ssh_port_t", "-p", "tcp", portStr)
	if err != nil {
		_, err = s.runner.Run(ctx, "semanage", "port", "-m", "-t", "ssh_port_t", "-p", "tcp", portStr)
	}
	if err != nil {
		return fmt.Errorf("labeling SSH port %d for SELinux: %w", port, err)
	}
	return nil
}

func (s *Service) socketActive(ctx context.Context) bool {
	if s.socketActiveFn != nil {
		return s.socketActiveFn(ctx)
	}
	res, err := s.runner.RunSilent(ctx, "systemctl", "is-active", "ssh.socket")
	return err == nil && strings.TrimSpace(res.Stdout) == "active"
}

func (s *Service) validateSSHD(ctx context.Context) error {
	if s.validateFn != nil {
		return s.validateFn(ctx)
	}
	return s.validateAndReloadSSH(ctx)
}

func (s *Service) validateAndReloadSSH(ctx context.Context) error {
	res, err := s.runner.RunSilent(ctx, "sshd", "-t")
	if err != nil || res.ExitCode != 0 {
		detail := strings.TrimSpace(res.Stderr)
		if detail == "" {
			detail = "sshd -t failed"
		}
		return fmt.Errorf("%s", detail)
	}
	return nil
}

func (s *Service) reloadRestored(ctx context.Context, socket bool) error {
	if socket {
		return s.restartSocket(ctx)
	}
	return s.reloadSSH(ctx)
}

func (s *Service) reloadSSH(ctx context.Context) error {
	if s.reloadFn != nil {
		return s.reloadFn(ctx)
	}
	return s.Reload(ctx, ReloadOptions{})
}

func (s *Service) restartSocket(ctx context.Context) error {
	if s.restartSocketFn != nil {
		return s.restartSocketFn(ctx)
	}
	if _, err := s.runner.Run(ctx, "systemctl", "daemon-reload"); err != nil {
		return err
	}
	_, err := s.runner.Run(ctx, "systemctl", "restart", "ssh.socket")
	return err
}

func (s *Service) checkBanner(port int) error {
	if s.bannerFn != nil {
		return s.bannerFn(port)
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 3*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(string(buf[:n]), "SSH-2.0") {
		return fmt.Errorf("unexpected banner %q", strings.TrimSpace(string(buf[:n])))
	}
	return nil
}

func containsPort(ports []int, port int) bool {
	for _, candidate := range ports {
		if candidate == port {
			return true
		}
	}
	return false
}

func otherPorts(ports []int, port int) []int {
	var out []int
	for _, candidate := range ports {
		if candidate != port {
			out = append(out, candidate)
		}
	}
	return out
}

func formatPorts(ports []int) string {
	parts := make([]string, len(ports))
	for i, port := range ports {
		parts[i] = strconv.Itoa(port)
	}
	return strings.Join(parts, ", ")
}
