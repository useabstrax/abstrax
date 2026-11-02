package firewall

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"abstrax/internal/services/sshcfg"
)

// inactiveFirewallSSHWarning is printed when a change would remove SSH access
// but the firewall is not active yet.
const inactiveFirewallSSHWarning = "WARNING: the firewall is inactive. firewall enable will need --allow-ssh.\n"

func sshBlockedError(port int) error {
	return fmt.Errorf("This would block SSH (port %d), and you could lose access to this server. Add another rule that allows SSH first, or pass --force to do it anyway.", port)
}

// listeningSSHPort returns the injected port in tests, otherwise sshd's port.
func (s *Service) listeningSSHPort() (int, error) {
	if s.sshPort != nil {
		return s.sshPort()
	}
	return sshcfg.SSHPort()
}

// guardSSH refuses a change that would leave an active firewall with no TCP
// path to the SSH port. An inactive firewall is allowed through with a warning.
// force skips the check. A nil check means the check did not run.
func (s *Service) guardSSH(force, active bool, next []Rule) (*SSHCheck, error) {
	if force {
		return nil, nil
	}
	port, err := s.listeningSSHPort()
	if err != nil {
		return nil, err
	}
	check := &SSHCheck{
		Port:    port,
		Blocked: !sshTCPReachable(next, port),
	}
	if !check.Blocked {
		return check, nil
	}
	if active {
		return nil, sshBlockedError(port)
	}
	fmt.Fprint(os.Stderr, inactiveFirewallSSHWarning)
	return check, nil
}

// sshTCPReachable reports whether any rule still allows inbound TCP to port.
// Rules are evaluated in order. A deny from anywhere stops the search, because
// UFW uses the first match. An allow limited to one address still counts.
func sshTCPReachable(rules []Rule, port int) bool {
	for _, r := range rules {
		if !ruleMatchesSSHTCP(r, port) {
			continue
		}
		if isDenyAction(r.Action) {
			if sourceIsAnywhere(r.From) {
				return false
			}
			continue
		}
		if isAllowAction(r.Action) {
			return true
		}
	}
	return false
}

// insertDenyBeforeAllows places a new deny ahead of every allow. UFW keeps
// going until the first match, so a later allow would not save SSH.
func insertDenyBeforeAllows(rules []Rule, deny Rule) []Rule {
	out := make([]Rule, 0, len(rules)+1)
	placed := false
	for _, r := range rules {
		if !placed && isAllowAction(r.Action) {
			out = append(out, deny)
			placed = true
		}
		out = append(out, r)
	}
	if !placed {
		out = append(out, deny)
	}
	return out
}

func simulatedDenyRule(opts AllowOptions) Rule {
	port, proto := normalizeRulePort(opts.Port, opts.Protocol)
	return Rule{
		Action:    "DENY",
		Direction: "in",
		Port:      port,
		Protocol:  proto,
		From:      strings.TrimSpace(opts.From),
	}
}

func normalizeRulePort(port, proto string) (string, string) {
	port = strings.TrimSpace(port)
	proto = strings.ToLower(strings.TrimSpace(proto))
	if i := strings.Index(port, "/"); i >= 0 && proto == "" {
		proto = strings.ToLower(strings.TrimSpace(port[i+1:]))
		port = strings.TrimSpace(port[:i])
	}
	return canonicalRichPort(port), proto
}

func withoutIndex(rules []Rule, idx int) []Rule {
	if idx < 0 || idx >= len(rules) {
		return append([]Rule(nil), rules...)
	}
	out := append([]Rule(nil), rules[:idx]...)
	return append(out, rules[idx+1:]...)
}

func withoutService(rules []Rule, service string) []Rule {
	var out []Rule
	for _, r := range rules {
		if r.Kind == RuleKindService && strings.EqualFold(r.Target, service) {
			continue
		}
		out = append(out, r)
	}
	return out
}

func withoutPort(rules []Rule, port, proto string) []Rule {
	if proto == "" {
		proto = "tcp"
	}
	var out []Rule
	for _, r := range rules {
		if r.Kind == RuleKindPort && r.Port == port && strings.EqualFold(r.Protocol, proto) {
			continue
		}
		out = append(out, r)
	}
	return out
}

func indexOfRule(rules []Rule, match *Rule) int {
	for i := range rules {
		if &rules[i] == match {
			return i
		}
	}
	return -1
}

func ruleMatchesSSHTCP(r Rule, port int) bool {
	if !isInbound(r) || !includesTCP(r.Protocol) {
		return false
	}
	if portSpecIncludes(r.Port, port) {
		return true
	}
	if port == 22 && (isBuiltinSSHName(r.Port) || (r.Kind == RuleKindService && isBuiltinSSHName(r.Target))) {
		return true
	}
	return false
}

func isInbound(r Rule) bool {
	switch strings.ToLower(r.Direction) {
	case "", "in":
		return true
	default:
		return false
	}
}

func includesTCP(proto string) bool {
	switch strings.ToLower(strings.TrimSpace(proto)) {
	case "", "tcp":
		return true
	default:
		return false
	}
}

func portSpecIncludes(spec string, port int) bool {
	spec = strings.TrimSpace(spec)
	if spec == "" || port <= 0 {
		return false
	}
	if strings.Contains(spec, ":") {
		parts := strings.SplitN(spec, ":", 2)
		start, err1 := strconv.Atoi(parts[0])
		end, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil || start > end {
			return false
		}
		return port >= start && port <= end
	}
	n, err := strconv.Atoi(spec)
	if err != nil {
		return false
	}
	return n == port
}

func isBuiltinSSHName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "ssh", "openssh":
		return true
	default:
		return false
	}
}

func isAllowAction(action string) bool {
	switch strings.ToUpper(action) {
	case "ALLOW", "LIMIT":
		return true
	default:
		return false
	}
}

func isDenyAction(action string) bool {
	switch strings.ToUpper(action) {
	case "DENY", "REJECT", "DROP":
		return true
	default:
		return false
	}
}

func sourceIsAnywhere(from string) bool {
	switch strings.ToLower(strings.TrimSpace(from)) {
	case "", "any", "anywhere", "0.0.0.0/0", "::/0":
		return true
	default:
		return false
	}
}
