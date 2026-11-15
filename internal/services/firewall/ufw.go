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
	from, sourcePort, sourceProto := parseUFWFrom(fromField)
	if proto == "" {
		proto = sourceProto
	}

	rule := Rule{
		Number:     m[1],
		Action:     action,
		Direction:  direction,
		Port:       port,
		Protocol:   proto,
		SourcePort: sourcePort,
		From:       from,
		To:         to,
		Comment:    comment,
		IPv6:       toV6 || fromV6,
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

// parseUFWFrom splits the From column. UFW prints a source port in that
// column as "<addr> <port>/<proto>", for example "10.1.2.3 3306/tcp".
func parseUFWFrom(field string) (addr, sourcePort, proto string) {
	field = strings.TrimSpace(field)
	if field == "" || strings.EqualFold(field, "anywhere") {
		return "", "", ""
	}
	parts := strings.Fields(field)
	if len(parts) >= 2 {
		last := parts[len(parts)-1]
		if p, pr, ok := splitUFWPort(last); ok {
			return strings.Join(parts[:len(parts)-1], " "), p, pr
		}
	}
	return field, "", ""
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
	// A source port used to be stored inside From ("10.1.2.3 3306/tcp") with
	// an empty protocol. Keep that hash input so existing rule IDs still match.
	from := r.From
	proto := r.Protocol
	if r.SourcePort != "" {
		spec := r.SourcePort
		if proto != "" && !strings.Contains(spec, "/") {
			spec += "/" + strings.ToLower(proto)
		}
		from = strings.TrimSpace(from + " " + spec)
		proto = ""
	}
	spec := strings.Join([]string{
		strings.ToLower(r.Action),
		dir,
		r.Port,
		strings.ToLower(proto),
		strings.ToLower(from),
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

	if r.SourcePort != "" {
		if r.From != "" {
			args = append(args, "from", r.From)
		} else {
			args = append(args, "from", "any")
		}
		args = append(args, "port", r.SourcePort)
		if r.Protocol != "" {
			args = append(args, "proto", r.Protocol)
		}
		if r.To != "" {
			args = append(args, "to", r.To)
		}
		if r.Port != "" {
			args = append(args, "port", r.Port)
		}
		return args
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

func (s *Service) ufwRuleRemove(ctx context.Context, id string, opts GuardOptions) (*SSHCheck, error) {
	status, err := s.ufwStatus(ctx)
	if err != nil {
		return nil, err
	}
	match, byNumber := findUFWRule(status.Rules, id)
	if match == nil {
		return nil, fmt.Errorf("rule not found, it may already have been removed")
	}
	check, err := s.guardSSH(opts.Force, status.Active, withoutIndex(status.Rules, indexOfRule(status.Rules, match)))
	if err != nil {
		return nil, err
	}
	if byNumber {
		fmt.Fprintf(os.Stderr, "WARNING: rule numbers can change when rules are added or removed; prefer the stable id from `abstrax firewall rule list`.\n")
		_, err = s.run(ctx, "ufw", "--force", "delete", match.Number)
		return check, err
	}
	_, err = s.run(ctx, "ufw", ufwDeleteArgs(*match)...)
	return check, err
}

// parseUFWAdded reads `ufw show added`. Each rule is marked pending because
// UFW is not applying it yet. IDs use the same hash as `ufw status numbered`.
func parseUFWAdded(output string) []Rule {
	var rules []Rule
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "ufw ") {
			continue
		}
		rule, ok := parseUFWAddedLine(line)
		if !ok {
			continue
		}
		rule.Pending = true
		rule.ID = ufwStableID(rule)
		rules = append(rules, rule)
	}
	return rules
}

func parseUFWAddedLine(line string) (Rule, bool) {
	tokens := tokenizeUFW(strings.TrimSpace(strings.TrimPrefix(line, "ufw")))
	i := 0
	for i < len(tokens) {
		switch strings.ToLower(tokens[i]) {
		case "route", "prepend", "--force", "--dry-run":
			i++
		case "insert":
			i++
			if i < len(tokens) {
				i++
			}
		default:
			goto action
		}
	}
action:
	if i >= len(tokens) {
		return Rule{}, false
	}
	action := strings.ToLower(tokens[i])
	switch action {
	case "allow", "deny", "reject", "limit":
		i++
	default:
		return Rule{}, false
	}

	rule := Rule{Action: strings.ToUpper(action), Direction: "in"}
	if i < len(tokens) {
		switch strings.ToLower(tokens[i]) {
		case "in", "out", "fwd":
			rule.Direction = strings.ToLower(tokens[i])
			i++
		}
	}
	if i < len(tokens) && (tokens[i] == "log" || tokens[i] == "log-all") {
		i++
	}
	if i < len(tokens) && strings.EqualFold(tokens[i], "on") {
		i++
		if i < len(tokens) {
			i++
		}
	}

	keywords := map[string]bool{
		"from": true, "to": true, "port": true, "proto": true, "comment": true,
	}
	if i < len(tokens) && !keywords[strings.ToLower(tokens[i])] {
		spec := tokens[i]
		i++
		if p, pr, ok := splitUFWPort(spec); ok {
			rule.Port, rule.Protocol = p, pr
		} else {
			// Application profile. status numbered stores the name in Port.
			rule.Port = spec
		}
	}

	sawTo := false
	sawSourcePort := false
	for i < len(tokens) {
		switch strings.ToLower(tokens[i]) {
		case "from":
			i++
			if i < len(tokens) {
				if !strings.EqualFold(tokens[i], "any") {
					rule.From = tokens[i]
				}
				i++
			}
		case "to":
			sawTo = true
			i++
			if i < len(tokens) {
				if !strings.EqualFold(tokens[i], "any") {
					rule.To = tokens[i]
				}
				i++
			}
		case "port":
			i++
			if i < len(tokens) {
				spec := tokens[i]
				i++
				if p, pr, ok := splitUFWPort(spec); ok {
					rule.Port = p
					if pr != "" {
						rule.Protocol = pr
					}
				} else {
					rule.Port = spec
				}
				if !sawTo && rule.From != "" {
					sawSourcePort = true
				}
			}
		case "proto":
			i++
			if i < len(tokens) {
				rule.Protocol = strings.ToLower(tokens[i])
				i++
			}
		case "comment":
			i++
			if i < len(tokens) {
				rule.Comment = strings.Join(tokens[i:], " ")
			}
			i = len(tokens)
		default:
			i++
		}
	}

	// `from <addr> port <port>` is a source port. status numbered keeps that
	// in the From column (`10.1.2.3 3306/tcp`) and leaves Port empty.
	if sawSourcePort {
		spec := rule.Port
		if rule.Protocol != "" && spec != "" && !strings.Contains(spec, "/") {
			spec += "/" + rule.Protocol
		}
		if spec != "" {
			rule.From = strings.TrimSpace(rule.From + " " + spec)
		}
		rule.Port = ""
		rule.Protocol = ""
	}
	if ufwAddrIsIPv6(rule.From) || ufwAddrIsIPv6(rule.To) {
		rule.IPv6 = true
	}
	return rule, true
}

func tokenizeUFW(s string) []string {
	var tokens []string
	var b strings.Builder
	var quote rune
	flush := func() {
		if b.Len() == 0 {
			return
		}
		tokens = append(tokens, b.String())
		b.Reset()
	}
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				tokens = append(tokens, b.String())
				b.Reset()
				quote = 0
				continue
			}
			b.WriteRune(r)
		case r == '\'' || r == '"':
			flush()
			quote = r
		case r == ' ' || r == '\t':
			flush()
		default:
			b.WriteRune(r)
		}
	}
	flush()
	return tokens
}

func ufwAddrIsIPv6(addr string) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return false
	}
	// A source-port From column is "<addr> <port>/<proto>".
	addr = strings.Fields(addr)[0]
	if ip := net.ParseIP(addr); ip != nil {
		return ip.To4() == nil
	}
	ip, _, err := net.ParseCIDR(addr)
	return err == nil && ip.To4() == nil
}
