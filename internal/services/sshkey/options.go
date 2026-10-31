package sshkey

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"abstrax/internal/validate"
)

const minExpiryOpenSSH = "8.2"

type keyRestrictions struct {
	prefix    string
	expiresAt *string
	from      []string
}

type parsedKeyLine struct {
	keyType   string
	blob      string
	comment   string
	expiresAt *string
	from      []string
}

func (s *Service) keyRestrictions(opts AddOptions) (keyRestrictions, error) {
	var out keyRestrictions
	if opts.Expires == "" && strings.TrimSpace(opts.From) == "" {
		return out, nil
	}

	var optionParts []string
	if opts.Expires != "" {
		when, err := time.Parse(time.RFC3339, opts.Expires)
		if err != nil {
			return out, fmt.Errorf("--expires must be an RFC3339 time, for example 2026-11-07T23:59:00Z")
		}
		when = when.UTC()
		if !when.After(s.currentTime()) {
			return out, fmt.Errorf("--expires must be in the future")
		}
		version, err := s.opensshVersion()
		if err != nil {
			return out, fmt.Errorf("checking OpenSSH version: %w", err)
		}
		if !opensshAtLeast(version, 8, 2) {
			return out, fmt.Errorf("expiry-time needs OpenSSH %s or later (this server has %s)", minExpiryOpenSSH, strings.TrimSpace(version))
		}
		stamp := when.Format("200601021504Z")
		optionParts = append(optionParts, fmt.Sprintf(`expiry-time=%q`, stamp))
		formatted := when.Format(time.RFC3339)
		out.expiresAt = &formatted
	}

	if strings.TrimSpace(opts.From) != "" {
		sources, err := parseFromList(opts.From)
		if err != nil {
			return out, err
		}
		optionParts = append(optionParts, fmt.Sprintf(`from=%q`, strings.Join(sources, ",")))
		out.from = sources
	}

	out.prefix = strings.Join(optionParts, ",")
	return out, nil
}

func (s *Service) opensshVersion() (string, error) {
	if s.sshVersion != nil {
		return s.sshVersion()
	}
	cmd := exec.Command("ssh", "-V")
	out, err := cmd.CombinedOutput()
	text := string(out)
	if strings.Contains(text, "OpenSSH_") {
		return text, nil
	}
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(text))
	}
	return "", fmt.Errorf("could not read the OpenSSH version from %q", strings.TrimSpace(text))
}

var opensshVersionRe = regexp.MustCompile(`OpenSSH_(\d+)\.(\d+)`)

func opensshAtLeast(version string, major, minor int) bool {
	m := opensshVersionRe.FindStringSubmatch(version)
	if m == nil {
		return false
	}
	gotMajor, _ := strconv.Atoi(m[1])
	gotMinor, _ := strconv.Atoi(m[2])
	if gotMajor != major {
		return gotMajor > major
	}
	return gotMinor >= minor
}

func parseFromList(raw string) ([]string, error) {
	items := strings.Split(raw, ",")
	if len(items) == 0 {
		return nil, fmt.Errorf("--from requires an IP address or CIDR")
	}
	var sources []string
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			return nil, fmt.Errorf("--from contains an empty address")
		}
		if strings.ContainsAny(item, `"'`) || strings.Contains(item, ",") {
			return nil, fmt.Errorf("--from address %q contains a quote or comma", item)
		}
		if err := validate.CIDRRange(item); err != nil {
			return nil, fmt.Errorf("--from: %w", err)
		}
		sources = append(sources, item)
	}
	return sources, nil
}

func splitAuthorizedKeyLine(line string) (parsedKeyLine, bool) {
	fields := strings.Fields(line)
	typeIdx := -1
	for i, field := range fields {
		if isSSHKeyType(field) {
			typeIdx = i
			break
		}
	}
	if typeIdx < 0 || typeIdx+1 >= len(fields) {
		return parsedKeyLine{}, false
	}
	parsed := parsedKeyLine{
		keyType: fields[typeIdx],
		blob:    fields[typeIdx+1],
		comment: strings.Join(fields[typeIdx+2:], " "),
	}
	if typeIdx > 0 {
		expiresAt, from := parseSSHKeyOptions(strings.Join(fields[:typeIdx], ""))
		parsed.expiresAt = expiresAt
		parsed.from = from
	}
	return parsed, true
}

func isSSHKeyType(field string) bool {
	return strings.HasPrefix(field, "ssh-") || strings.HasPrefix(field, "ecdsa-") || strings.HasPrefix(field, "sk-")
}

func parseSSHKeyOptions(options string) (expiresAt *string, from []string) {
	for _, part := range splitOptionList(options) {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		value = strings.Trim(value, `"`)
		switch key {
		case "expiry-time":
			if when, ok := parseExpiryStamp(value); ok {
				formatted := when.UTC().Format(time.RFC3339)
				expiresAt = &formatted
			}
		case "from":
			for _, item := range strings.Split(value, ",") {
				item = strings.TrimSpace(item)
				if item != "" {
					from = append(from, item)
				}
			}
		}
	}
	return expiresAt, from
}

func splitOptionList(options string) []string {
	var parts []string
	var b strings.Builder
	inQuote := false
	for _, r := range options {
		switch {
		case r == '"':
			inQuote = !inQuote
			b.WriteRune(r)
		case r == ',' && !inQuote:
			parts = append(parts, b.String())
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		parts = append(parts, b.String())
	}
	return parts
}

func parseExpiryStamp(value string) (time.Time, bool) {
	for _, layout := range []string{"200601021504Z", "20060102150405Z", "20060102Z"} {
		if when, err := time.Parse(layout, value); err == nil {
			return when.UTC(), true
		}
	}
	return time.Time{}, false
}
