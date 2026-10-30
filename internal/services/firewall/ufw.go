package firewall

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
)

// ufwRulePattern matches one line of `ufw status numbered`.
// The To column is non-greedy so application names that contain spaces
// (for example "Nginx Full") stay in front of the action keyword.
var ufwRulePattern = regexp.MustCompile(`(?i)^\[\s*(\d+)\]\s+(.+?)\s+(ALLOW|DENY|REJECT|LIMIT)(?:\s+(IN|OUT|FWD))?\s+(\S.*)$`)

var ufwPortPattern = regexp.MustCompile(`(?i)^(\d+(?::\d+)?)(?:/(tcp|udp))?$`)

func parseUFWRules(output string) []Rule {
	var rules []Rule
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "[") {
			continue
		}
		rule, ok := parseUFWRuleLine(line)
		if !ok {
			continue
		}
		rules = append(rules, rule)
	}
	return rules
}

func parseUFWRuleLine(line string) (Rule, bool) {
	comment := ""
	if i := strings.Index(line, " #"); i >= 0 {
		comment = strings.TrimSpace(line[i+2:])
		comment = strings.TrimPrefix(comment, "#")
		comment = strings.TrimSpace(comment)
		line = strings.TrimSpace(line[:i])
	}

	m := ufwRulePattern.FindStringSubmatch(line)
	if m == nil {
		return Rule{}, false
	}

	toField, toV6 := stripUFWMarker(m[2], "(v6)")
	action := strings.ToUpper(m[3])
	direction := strings.ToLower(m[4])
	if direction == "" {
		direction = "in"
	}
	fromField := strings.TrimSpace(m[5])
	fromField, fromOut := stripUFWMarker(fromField, "(out)")
	if fromOut && direction == "in" {
		direction = "out"
	}
	fromField, fromV6 := stripUFWMarker(fromField, "(v6)")

	to, port, proto := parseUFWTo(toField)
	from := parseUFWFrom(fromField)

	rule := Rule{
		Number:    m[1],
		Action:    action,
		Direction: direction,
		Port:      port,
		Protocol:  proto,
		From:      from,
		To:        to,
		Comment:   comment,
		IPv6:      toV6 || fromV6,
	}
	rule.ID = ufwStableID(rule)
	return rule, true
}

func stripUFWMarker(field, marker string) (string, bool) {
	field = strings.TrimSpace(field)
	if strings.HasSuffix(strings.ToLower(field), strings.ToLower(marker)) {
		trimmed := strings.TrimSpace(field[:len(field)-len(marker)])
		return trimmed, true
	}
	return field, false
}

func parseUFWTo(field string) (addr, port, proto string) {
	field = strings.TrimSpace(field)
	if field == "" || strings.EqualFold(field, "anywhere") {
		return "", "", ""
	}

	parts := strings.Fields(field)
	spec := parts[0]
	if len(parts) >= 2 {
		addr = strings.Join(parts[:len(parts)-1], " ")
		spec = parts[len(parts)-1]
		if p, pr, ok := splitUFWPort(spec); ok {
			return addr, p, pr
		}
		// The last token is not a port, so the whole field is the destination
		// (an address, or an application name with no port).
		if looksLikeAddr(field) {
			return field, "", ""
		}
		return "", field, ""
	}

	if p, pr, ok := splitUFWPort(spec); ok {
		return "", p, pr
	}
	if looksLikeAddr(spec) {
		return spec, "", ""
	}
	// Application profile such as OpenSSH or "Nginx Full".
	return "", field, ""
}

func parseUFWFrom(field string) string {
	field = strings.TrimSpace(field)
	if field == "" || strings.EqualFold(field, "anywhere") {
		return ""
	}
	return field
}

func splitUFWPort(spec string) (port, proto string, ok bool) {
	m := ufwPortPattern.FindStringSubmatch(spec)
	if m == nil {
		return "", "", false
	}
	return m[1], strings.ToLower(m[2]), true
}

func looksLikeAddr(s string) bool {
	if net.ParseIP(s) != nil {
		return true
	}
	_, _, err := net.ParseCIDR(s)
	return err == nil
}

// ufwStableID is the first 8 hex characters of SHA-256 over the normalised
// rule: action, direction, port, protocol, from, to, and v4/v6.
func ufwStableID(r Rule) string {
	ipver := "v4"
	if r.IPv6 {
		ipver = "v6"
	}
	dir := strings.ToLower(r.Direction)
	if dir == "" {
		dir = "in"
	}
	spec := strings.Join([]string{
		strings.ToLower(r.Action),
		dir,
		r.Port,
		strings.ToLower(r.Protocol),
		strings.ToLower(r.From),
		strings.ToLower(r.To),
		ipver,
	}, "|")
	sum := sha256.Sum256([]byte(spec))
	return hex.EncodeToString(sum[:])[:8]
}

func findUFWRule(rules []Rule, id string) (rule *Rule, byNumber bool) {
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

// ufwDeleteArgs builds `ufw delete ...` arguments that identify a rule by its
// specification rather than its position.
func ufwDeleteArgs(r Rule) []string {
	action := strings.ToLower(strings.TrimSpace(r.Action))
	if action == "" {
		action = "allow"
	}
	args := []string{"--force", "delete", action}
	dir := strings.ToLower(r.Direction)
	if dir == "out" || dir == "fwd" {
		args = append(args, dir)
	}

	if r.From != "" || r.To != "" {
		if r.From != "" {
			args = append(args, "from", r.From)
		} else {
			args = append(args, "from", "any")
		}
		if r.To != "" {
			args = append(args, "to", r.To)
		} else {
			args = append(args, "to", "any")
		}
		if r.Port != "" {
			args = append(args, "port", r.Port)
		}
		if r.Protocol != "" {
			args = append(args, "proto", r.Protocol)
		}
		return args
	}

	if r.Port != "" {
		spec := r.Port
		if r.Protocol != "" && !strings.Contains(r.Port, "/") {
			spec = r.Port + "/" + r.Protocol
		}
		args = append(args, spec)
	}
	return args
}

func (s *Service) ufwRuleRemove(ctx context.Context, id string) error {
	rules, err := s.RuleList(ctx)
	if err != nil {
		return err
	}
	match, byNumber := findUFWRule(rules, id)
	if match == nil {
		return fmt.Errorf("rule not found, it may already have been removed")
	}
	if byNumber {
		fmt.Fprintf(os.Stderr, "WARNING: rule numbers can change when rules are added or removed; prefer the stable id from `abstrax firewall rule list`.\n")
		_, err = s.runner.Run(ctx, "ufw", "--force", "delete", match.Number)
		return err
	}
	_, err = s.runner.Run(ctx, "ufw", ufwDeleteArgs(*match)...)
	return err
}
