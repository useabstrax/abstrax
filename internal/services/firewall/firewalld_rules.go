package firewall

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

var (
	richFamilyRE   = regexp.MustCompile(`family="([^"]*)"`)
	richSourceRE   = regexp.MustCompile(`source address="([^"]*)"`)
	richPortRE     = regexp.MustCompile(`port port="([^"]*)"`)
	richProtocolRE = regexp.MustCompile(`protocol="([^"]*)"`)
)

func parseFirewalldRules(output string) []Rule {
	var rules []Rule
	inRich := false
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "services:") {
			inRich = false
			for _, svc := range strings.Fields(strings.TrimPrefix(trimmed, "services:")) {
				rules = append(rules, Rule{
					Action: "ALLOW",
					Port:   svc,
					Kind:   RuleKindService,
					Target: svc,
				})
			}
			continue
		}
		if strings.HasPrefix(trimmed, "ports:") {
			inRich = false
			for _, spec := range strings.Fields(strings.TrimPrefix(trimmed, "ports:")) {
				port, proto := splitPortProto(spec)
				rules = append(rules, Rule{
					Action:   "ALLOW",
					Port:     port,
					Protocol: proto,
					Kind:     RuleKindPort,
					Target:   spec,
				})
			}
			continue
		}
		if strings.HasPrefix(trimmed, "rich rules:") {
			inRich = true
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "rich rules:"))
			if strings.HasPrefix(rest, "rule ") {
				rules = append(rules, parseRichRule(rest))
			}
			continue
		}
		if inRich && strings.HasPrefix(trimmed, "rule ") {
			rules = append(rules, parseRichRule(trimmed))
			continue
		}
		if inRich && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			inRich = false
		}
	}

	for i := range rules {
		rules[i].Number = strconv.Itoa(i + 1)
		rules[i].ID = firewalldStableID(rules[i])
	}
	return rules
}

func parseRichRule(text string) Rule {
	text = strings.Join(strings.Fields(text), " ")
	rule := Rule{
		Kind:   RuleKindRich,
		Target: text,
		Action: "ALLOW",
	}
	if m := richFamilyRE.FindStringSubmatch(text); m != nil && strings.EqualFold(m[1], "ipv6") {
		rule.IPv6 = true
	}
	if m := richSourceRE.FindStringSubmatch(text); m != nil {
		rule.From = m[1]
	}
	if m := richPortRE.FindStringSubmatch(text); m != nil {
		rule.Port = m[1]
	}
	if m := richProtocolRE.FindStringSubmatch(text); m != nil {
		rule.Protocol = m[1]
	}
	verdict := ""
	if fields := strings.Fields(text); len(fields) > 0 {
		verdict = strings.ToLower(fields[len(fields)-1])
	}
	switch verdict {
	case "reject", "drop":
		rule.Action = "DENY"
	default:
		rule.Action = "ALLOW"
	}
	return rule
}

// firewalldStableID is the first 8 hex characters of SHA-256 over the rule
// kind and its normalised fields. List position is not part of the hash.
func firewalldStableID(r Rule) string {
	spec := strings.Join([]string{
		string(r.Kind),
		strings.ToLower(r.Action),
		r.Port,
		strings.ToLower(r.Protocol),
		strings.ToLower(r.From),
		strings.ToLower(r.To),
		strings.Join(strings.Fields(r.Target), " "),
	}, "|")
	sum := sha256.Sum256([]byte(spec))
	return hex.EncodeToString(sum[:])[:8]
}

func findFirewalldRule(rules []Rule, id string) (rule *Rule, byNumber bool) {
	for i := range rules {
		if rules[i].ID == id {
			return &rules[i], false
		}
	}
	if !isDecimalID(id) {
		return nil, false
	}
	for i := range rules {
		if rules[i].Number == id {
			return &rules[i], true
		}
	}
	return nil, false
}

func isDecimalID(id string) bool {
	if id == "" {
		return false
	}
	for _, c := range id {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func firewalldFamily(addr string) (string, error) {
	addr = strings.TrimSpace(addr)
	ip := net.ParseIP(addr)
	if ip == nil {
		parsed, _, err := net.ParseCIDR(addr)
		if err != nil {
			return "", fmt.Errorf("invalid IP address or CIDR %q", addr)
		}
		ip = parsed
	}
	if ip.To4() != nil {
		return "ipv4", nil
	}
	return "ipv6", nil
}

func normalizeProto(proto string) (string, error) {
	proto = strings.ToLower(strings.TrimSpace(proto))
	if proto == "" {
		return "tcp", nil
	}
	if proto != "tcp" && proto != "udp" {
		return "", fmt.Errorf("protocol must be tcp or udp")
	}
	return proto, nil
}

func canonicalRichPort(port string) string {
	switch strings.ToLower(strings.TrimSpace(port)) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return strings.TrimSpace(port)
	}
}

func firewalldRichRule(family, source, port, proto, verdict string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `rule family="%s"`, family)
	if source != "" {
		fmt.Fprintf(&b, ` source address="%s"`, source)
	}
	if port != "" {
		if proto == "" {
			proto = "tcp"
		}
		fmt.Fprintf(&b, ` port port="%s" protocol="%s"`, port, proto)
	}
	fmt.Fprintf(&b, " %s", verdict)
	return b.String()
}

func (s *Service) firewalldAllowFrom(ctx context.Context, opts AllowOptions) error {
	family, err := firewalldFamily(opts.From)
	if err != nil {
		return err
	}
	proto, err := normalizeProto(opts.Protocol)
	if err != nil {
		return err
	}
	port := canonicalRichPort(opts.Port)
	if port == "" {
		return fmt.Errorf("must specify a port or service")
	}
	// A source-limited allow is only a rich rule. Never fall back to
	// --add-port or --add-service, which would open the port to everyone.
	return s.addRichRules(ctx, []string{firewalldRichRule(family, opts.From, port, proto, "accept")})
}

func (s *Service) firewalldDeny(ctx context.Context, opts AllowOptions) error {
	proto, err := normalizeProto(opts.Protocol)
	if err != nil {
		return err
	}
	port := canonicalRichPort(opts.Port)
	if port == "" {
		return fmt.Errorf("must specify a port")
	}
	if strings.TrimSpace(opts.From) != "" {
		family, err := firewalldFamily(opts.From)
		if err != nil {
			return err
		}
		return s.addRichRules(ctx, []string{firewalldRichRule(family, opts.From, port, proto, "reject")})
	}
	return s.addRichRules(ctx, []string{
		firewalldRichRule("ipv4", "", port, proto, "reject"),
		firewalldRichRule("ipv6", "", port, proto, "reject"),
	})
}

func (s *Service) addRichRules(ctx context.Context, rules []string) error {
	for _, rule := range rules {
		if _, err := s.runner.Run(ctx, "firewall-cmd", "--permanent", "--add-rich-rule="+rule); err != nil {
			return err
		}
	}
	_, err := s.runner.Run(ctx, "firewall-cmd", "--reload")
	return err
}

func (s *Service) firewalldRemoveRichRule(ctx context.Context, rule string) error {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return fmt.Errorf("rich rule is required")
	}
	if _, err := s.runner.Run(ctx, "firewall-cmd", "--permanent", "--remove-rich-rule="+rule); err != nil {
		return err
	}
	_, err := s.runner.Run(ctx, "firewall-cmd", "--reload")
	return err
}
