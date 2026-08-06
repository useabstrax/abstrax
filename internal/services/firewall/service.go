// Package firewall manages the system firewall using UFW or firewalld.
package firewall

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	executil "abstrax/internal/exec"
	"abstrax/internal/platform"
	"abstrax/internal/services/pkgmanager"
	"abstrax/internal/services/sshcfg"
)

// Service manages the firewall.
type Service struct {
	runner  *executil.Runner
	backend string
}

// New creates a Service with automatic backend detection.
func New(dryRun, verbose bool) *Service {
	return &Service{
		runner:  executil.New(dryRun, verbose),
		backend: detectBackend(),
	}
}

func detectBackend() string {
	info, _, err := platform.Detect()
	if err == nil {
		switch info.Profile.FirewallStrategy {
		case "firewalld":
			if executil.Exists("firewall-cmd") {
				return "firewalld"
			}
		case "ufw":
			if executil.Exists("ufw") {
				return "ufw"
			}
		}
	}
	if executil.Exists("ufw") {
		return "ufw"
	}
	if executil.Exists("firewall-cmd") {
		return "firewalld"
	}
	return ""
}

func (s *Service) requireBackend() error {
	if s.backend != "" {
		return nil
	}
	provider := platform.Resolve()
	pkg := provider.FirewallPackage()
	if pkg == "" {
		switch provider.Profile().Family {
		case "debian":
			pkg = "ufw"
		case "rhel":
			pkg = "firewalld"
		}
	}
	if pkg != "" {
		return fmt.Errorf("no supported firewall backend found; install %s with `sudo abstrax firewall install`, then retry", pkg)
	}
	return fmt.Errorf("no supported firewall backend found (ufw or firewalld)")
}

// Install installs the platform firewall package (ufw on Debian-family, firewalld
// on RHEL-family). It does not enable the firewall; use Enable for that.
func (s *Service) Install(ctx context.Context, opts InstallOptions) (*InstallResult, error) {
	provider := platform.Resolve()
	pkg := provider.FirewallPackage()
	if pkg == "" {
		return nil, fmt.Errorf("no supported firewall package for this platform")
	}

	result := &InstallResult{
		Package: pkg,
		Backend: backendForPackage(pkg),
	}

	if s.backend != "" {
		result.AlreadyInstalled = true
		result.Backend = s.backend
		return result, nil
	}

	fmt.Printf("Installing %s...\n", pkg)
	mgr, _, err := pkgmanager.NewFromPlatform(opts.DryRun, false)
	if err != nil {
		return nil, err
	}
	if err := mgr.Install(ctx, pkgmanager.InstallOptions{Name: pkg, DryRun: opts.DryRun}); err != nil {
		return nil, fmt.Errorf("installing %s: %w", pkg, err)
	}

	s.backend = detectBackend()
	if s.backend == "" && !opts.DryRun {
		return nil, fmt.Errorf("installed %s but no firewall command was found on PATH", pkg)
	}
	if s.backend == "" && opts.DryRun {
		s.backend = backendForPackage(pkg)
	}
	result.Backend = s.backend
	return result, nil
}

func backendForPackage(pkg string) string {
	switch pkg {
	case "firewalld":
		return "firewalld"
	case "ufw":
		return "ufw"
	default:
		return ""
	}
}

// ensureBackendInstalled installs the platform firewall package when the CLI
// binary (ufw / firewall-cmd) is missing, then re-detects the backend.
func (s *Service) ensureBackendInstalled(ctx context.Context, dryRun bool) error {
	_, err := s.Install(ctx, InstallOptions{DryRun: dryRun})
	return err
}

// Backend returns the active firewall backend name.
func (s *Service) Backend() string {
	return s.backend
}

// GetStatus returns the current firewall status.
func (s *Service) GetStatus(ctx context.Context) (*Status, error) {
	if err := s.requireBackend(); err != nil {
		return nil, err
	}
	if s.backend == "firewalld" {
		return s.firewalldStatus(ctx)
	}
	return s.ufwStatus(ctx)
}

func (s *Service) ufwStatus(ctx context.Context) (*Status, error) {
	res, err := s.runner.RunSilent(ctx, "ufw", "status", "numbered")
	if err != nil {
		return nil, fmt.Errorf("ufw status: %w", err)
	}
	status := &Status{Backend: "ufw"}
	status.Active = strings.Contains(res.Stdout, "Status: active")
	status.Rules = parseUFWRules(res.Stdout)
	return status, nil
}

func (s *Service) firewalldStatus(ctx context.Context) (*Status, error) {
	status := &Status{Backend: "firewalld"}
	res, err := s.runner.RunSilent(ctx, "firewall-cmd", "--state")
	if err == nil && strings.Contains(strings.ToLower(res.Stdout), "running") {
		status.Active = true
	}

	listRes, err := s.runner.RunSilent(ctx, "firewall-cmd", "--list-all")
	if err == nil {
		status.Rules = parseFirewalldRules(listRes.Stdout)
	}
	return status, nil
}

// Enable enables the firewall.
func (s *Service) Enable(ctx context.Context, opts EnableOptions) (SSHProtectResult, error) {
	if err := s.ensureBackendInstalled(ctx, opts.DryRun); err != nil {
		return SSHProtectResult{}, err
	}

	sshPort := opts.SSHPort
	if sshPort == 0 {
		sshPort = 22
	}
	if sshPort == 22 {
		if configured, err := sshcfg.SSHPort(); err == nil {
			sshPort = configured
		}
	}

	// firewalld needs to be running before firewall-cmd can add SSH protection rules.
	if s.backend == "firewalld" {
		if _, err := s.runner.Run(ctx, "systemctl", "enable", "--now", "firewalld"); err != nil {
			return SSHProtectResult{}, fmt.Errorf("starting firewalld: %w", err)
		}
	}

	protect, err := s.ensureClientSSHAllow(ctx)
	if err != nil {
		return protect, err
	}

	if s.backend == "firewalld" {
		if opts.AllowSSH {
			if err := s.firewalldAllowPort(ctx, strconv.Itoa(sshPort), "tcp"); err != nil {
				return protect, fmt.Errorf("allowing SSH port: %w", err)
			}
		}
		return protect, nil
	}

	if opts.AllowSSH {
		if _, err := s.runner.Run(ctx, "ufw", "allow",
			fmt.Sprintf("%d/tcp", sshPort)); err != nil {
			return protect, fmt.Errorf("allowing SSH port: %w", err)
		}
	}
	_, err = s.runner.Run(ctx, "ufw", "--force", "enable")
	return protect, err
}

// Disable disables the firewall.
func (s *Service) Disable(ctx context.Context) error {
	if err := s.requireBackend(); err != nil {
		return err
	}
	if s.backend == "firewalld" {
		_, err := s.runner.Run(ctx, "systemctl", "disable", "--now", "firewalld")
		return err
	}
	_, err := s.runner.Run(ctx, "ufw", "--force", "disable")
	return err
}

// Allow adds an allow rule.
func (s *Service) Allow(ctx context.Context, opts AllowOptions) (SSHProtectResult, error) {
	protect, err := s.ensureClientSSHAllow(ctx)
	if err != nil {
		return protect, err
	}
	return protect, s.addRule(ctx, "allow", opts)
}

// Deny adds a deny rule.
func (s *Service) Deny(ctx context.Context, opts AllowOptions) (SSHProtectResult, error) {
	protect, err := s.ensureClientSSHAllow(ctx)
	if err != nil {
		return protect, err
	}
	return protect, s.addRule(ctx, "deny", opts)
}

// AllowIP allows traffic from an IP or CIDR.
func (s *Service) AllowIP(ctx context.Context, opts AllowOptions) (SSHProtectResult, error) {
	protect, err := s.ensureClientSSHAllow(ctx)
	if err != nil {
		return protect, err
	}
	return protect, s.allowIP(ctx, opts)
}

// DenyIP denies traffic from an IP or CIDR.
func (s *Service) DenyIP(ctx context.Context, opts AllowOptions) (SSHProtectResult, error) {
	protect, err := s.ensureClientSSHAllow(ctx)
	if err != nil {
		return protect, err
	}
	return protect, s.denyIP(ctx, opts)
}

// RuleList returns the current rules.
func (s *Service) RuleList(ctx context.Context) ([]Rule, error) {
	status, err := s.GetStatus(ctx)
	if err != nil {
		return nil, err
	}
	return status.Rules, nil
}

// RuleRemove removes a rule by list ID.
// On UFW this deletes the numbered rule. On firewalld it removes the service or
// port that Abstrax assigned to that ID in `firewall rule list`.
func (s *Service) RuleRemove(ctx context.Context, id string) error {
	if err := s.requireBackend(); err != nil {
		return err
	}
	if s.backend == "firewalld" {
		return s.firewalldRuleRemove(ctx, id)
	}
	_, err := s.runner.Run(ctx, "ufw", "--force", "delete", id)
	return err
}

// RemoveService removes a firewalld service permanently and reloads.
func (s *Service) RemoveService(ctx context.Context, service string) error {
	if err := s.requireBackend(); err != nil {
		return err
	}
	if s.backend != "firewalld" {
		return fmt.Errorf("firewall remove service is only supported with firewalld; use `abstrax firewall rule remove <id>` on UFW")
	}
	return s.firewalldRemoveService(ctx, service)
}

// RemovePort removes a firewalld port permanently and reloads.
func (s *Service) RemovePort(ctx context.Context, port, proto string) error {
	if err := s.requireBackend(); err != nil {
		return err
	}
	if s.backend != "firewalld" {
		return fmt.Errorf("firewall remove port is only supported with firewalld; use `abstrax firewall rule remove <id>` on UFW")
	}
	if proto == "" {
		proto = "tcp"
	}
	return s.firewalldRemovePort(ctx, port, proto)
}

func (s *Service) firewalldRuleRemove(ctx context.Context, id string) error {
	rules, err := s.RuleList(ctx)
	if err != nil {
		return err
	}
	var match *Rule
	for i := range rules {
		if rules[i].ID == id {
			match = &rules[i]
			break
		}
	}
	if match == nil {
		return fmt.Errorf("firewall rule %q not found; run `abstrax firewall rule list` to see current IDs", id)
	}
	switch match.Kind {
	case RuleKindService:
		return s.firewalldRemoveService(ctx, match.Target)
	case RuleKindPort:
		port, proto := splitPortProto(match.Target)
		return s.firewalldRemovePort(ctx, port, proto)
	default:
		return fmt.Errorf("cannot remove firewalld rule %q (kind %q); use `abstrax firewall remove service <name>` or `abstrax firewall remove port <port[/proto]>`", id, match.Kind)
	}
}

func (s *Service) firewalldRemoveService(ctx context.Context, service string) error {
	service = strings.TrimSpace(service)
	if service == "" {
		return fmt.Errorf("service name is required")
	}
	if _, err := s.runner.Run(ctx, "firewall-cmd", "--permanent", "--remove-service="+service); err != nil {
		return err
	}
	_, err := s.runner.Run(ctx, "firewall-cmd", "--reload")
	return err
}

func (s *Service) firewalldRemovePort(ctx context.Context, port, proto string) error {
	port = strings.TrimSpace(port)
	if port == "" {
		return fmt.Errorf("port is required")
	}
	if proto == "" {
		proto = "tcp"
	}
	arg := fmt.Sprintf("--remove-port=%s/%s", port, proto)
	if _, err := s.runner.Run(ctx, "firewall-cmd", "--permanent", arg); err != nil {
		return err
	}
	_, err := s.runner.Run(ctx, "firewall-cmd", "--reload")
	return err
}

func splitPortProto(target string) (port, proto string) {
	parts := strings.SplitN(strings.TrimSpace(target), "/", 2)
	port = parts[0]
	proto = "tcp"
	if len(parts) == 2 && parts[1] != "" {
		proto = parts[1]
	}
	return port, proto
}

func (s *Service) allowIP(ctx context.Context, opts AllowOptions) error {
	if s.backend == "firewalld" {
		return s.firewalldAllowIP(ctx, opts)
	}
	args := []string{"allow", "from", opts.From}
	if opts.To != "" {
		args = append(args, "to", opts.To)
	}
	if opts.Port != "" {
		args = append(args, "port", opts.Port)
	}
	_, err := s.runner.Run(ctx, "ufw", args...)
	return err
}

func (s *Service) denyIP(ctx context.Context, opts AllowOptions) error {
	if s.backend == "firewalld" {
		return s.firewalldDenyIP(ctx, opts)
	}
	args := []string{"deny", "from", opts.From}
	if opts.To != "" {
		args = append(args, "to", opts.To)
	}
	_, err := s.runner.Run(ctx, "ufw", args...)
	return err
}

func (s *Service) addRule(ctx context.Context, action string, opts AllowOptions) error {
	if err := s.requireBackend(); err != nil {
		return err
	}
	if s.backend == "firewalld" {
		if action != "allow" {
			return fmt.Errorf("firewalld deny rules for ports are not implemented; use rich rules via firewall-cmd")
		}
		return s.firewalldAllow(ctx, opts)
	}

	port := opts.Port
	if opts.Protocol != "" {
		port = fmt.Sprintf("%s/%s", port, opts.Protocol)
	}

	args := []string{action}
	if opts.From != "" {
		args = append(args, "from", opts.From)
		if port != "" {
			args = append(args, "to", "any", "port", port)
		}
	} else if port != "" {
		args = append(args, port)
	} else {
		return fmt.Errorf("must specify a port or IP")
	}

	if opts.Comment != "" {
		args = append(args, "comment", opts.Comment)
	}

	_, err := s.runner.Run(ctx, "ufw", args...)
	return err
}

func (s *Service) firewalldAllow(ctx context.Context, opts AllowOptions) error {
	port := strings.TrimSpace(opts.Port)
	proto := opts.Protocol
	if proto == "" {
		proto = "tcp"
	}

	// Map common web ports to firewalld services.
	switch port {
	case "80", "http":
		return s.firewalldAddService(ctx, "http")
	case "443", "https":
		return s.firewalldAddService(ctx, "https")
	}

	if port == "" {
		return fmt.Errorf("must specify a port or service")
	}
	return s.firewalldAllowPort(ctx, port, proto)
}

func (s *Service) firewalldAddService(ctx context.Context, service string) error {
	if _, err := s.runner.Run(ctx, "firewall-cmd", "--permanent", "--add-service="+service); err != nil {
		return err
	}
	_, err := s.runner.Run(ctx, "firewall-cmd", "--reload")
	return err
}

func (s *Service) firewalldAllowPort(ctx context.Context, port, proto string) error {
	arg := fmt.Sprintf("--add-port=%s/%s", port, proto)
	if _, err := s.runner.Run(ctx, "firewall-cmd", "--permanent", arg); err != nil {
		return err
	}
	_, err := s.runner.Run(ctx, "firewall-cmd", "--reload")
	return err
}

func (s *Service) firewalldAllowIP(ctx context.Context, opts AllowOptions) error {
	port := opts.Port
	proto := opts.Protocol
	if proto == "" {
		proto = "tcp"
	}
	rich := fmt.Sprintf(`rule family="ipv4" source address="%s" `, opts.From)
	if port != "" {
		rich += fmt.Sprintf(`port port="%s" protocol="%s" accept`, port, proto)
	} else {
		rich += "accept"
	}
	if _, err := s.runner.Run(ctx, "firewall-cmd", "--permanent", "--add-rich-rule="+rich); err != nil {
		return err
	}
	_, err := s.runner.Run(ctx, "firewall-cmd", "--reload")
	return err
}

func (s *Service) firewalldDenyIP(ctx context.Context, opts AllowOptions) error {
	rich := fmt.Sprintf(`rule family="ipv4" source address="%s" reject`, opts.From)
	if _, err := s.runner.Run(ctx, "firewall-cmd", "--permanent", "--add-rich-rule="+rich); err != nil {
		return err
	}
	_, err := s.runner.Run(ctx, "firewall-cmd", "--reload")
	return err
}

func parseUFWRules(output string) []Rule {
	var rules []Rule
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "[") {
			continue
		}

		closeBracket := strings.Index(line, "]")
		if closeBracket < 0 {
			continue
		}

		id := strings.Trim(line[1:closeBracket], " ")
		rest := strings.TrimSpace(line[closeBracket+1:])

		r := Rule{ID: id}
		parts := strings.Fields(rest)
		if len(parts) >= 1 {
			r.Port = parts[0]
		}
		if len(parts) >= 2 {
			r.Action = parts[1]
		}
		if len(parts) >= 4 && strings.EqualFold(parts[2], "IN") {
			from := parts[3]
			if from != "Anywhere" && !strings.HasPrefix(from, "(") {
				r.From = from
			}
		}
		rules = append(rules, r)
	}
	return rules
}

func parseFirewalldRules(output string) []Rule {
	var rules []Rule
	id := 1
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "services:") {
			services := strings.Fields(strings.TrimPrefix(line, "services:"))
			for _, svc := range services {
				rules = append(rules, Rule{
					ID:     strconv.Itoa(id),
					Action: "ALLOW",
					Port:   svc,
					Kind:   RuleKindService,
					Target: svc,
				})
				id++
			}
		}
		if strings.HasPrefix(line, "ports:") {
			ports := strings.Fields(strings.TrimPrefix(line, "ports:"))
			for _, port := range ports {
				rules = append(rules, Rule{
					ID:     strconv.Itoa(id),
					Action: "ALLOW",
					Port:   port,
					Kind:   RuleKindPort,
					Target: port,
				})
				id++
			}
		}
	}
	return rules
}
