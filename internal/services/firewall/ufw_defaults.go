package firewall

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	defaultUFWBeforeRules  = "/etc/ufw/before.rules"
	defaultUFWBefore6Rules = "/etc/ufw/before6.rules"
	defaultUFWDefaultFile  = "/etc/default/ufw"
)

var (
	ufwDefaultRE = regexp.MustCompile(`(?i)([a-z]+) \((incoming|outgoing|routed)\)`)
	// ufwEchoRE matches an INPUT-chain echo-request rule in before.rules or
	// before6.rules. The FORWARD chain rule is left alone: blocking ping to this
	// host should not block routed traffic. Groups: prefix, target, trailing space.
	ufwEchoRE = regexp.MustCompile(`^(\s*-A\s+ufw6?-before-input\s(?:[^#]*\s)?--icmp-type\s+echo-request(?:\s[^#]*)?\s-j\s+)(ACCEPT|DROP)(\s*)$`)
)

func (s *Service) beforeRulesPath() string {
	if s.ufwBeforeRules != "" {
		return s.ufwBeforeRules
	}
	return defaultUFWBeforeRules
}

func (s *Service) before6RulesPath() string {
	if s.ufwBefore6Rules != "" {
		return s.ufwBefore6Rules
	}
	return defaultUFWBefore6Rules
}

func (s *Service) ufwDefaultFilePath() string {
	if s.ufwDefaultFile != "" {
		return s.ufwDefaultFile
	}
	return defaultUFWDefaultFile
}

func (s *Service) ufwDefaults(ctx context.Context) (*Defaults, error) {
	res, err := s.runner.RunQuery(ctx, "ufw", "status", "verbose")
	if err != nil {
		return nil, fmt.Errorf("ufw status: %w", err)
	}
	defaults, ok := parseUFWDefaults(res.Stdout)
	if !ok {
		// An inactive firewall prints no Default line. Its policy is still
		// stored in /etc/default/ufw.
		defaults, err = parseUFWDefaultFile(s.ufwDefaultFilePath())
		if err != nil {
			return nil, err
		}
	}
	defaults.Ping = s.ufwPing()
	return &defaults, nil
}

// parseUFWDefaults reads the "Default:" line of `ufw status verbose`, such as
// "Default: deny (incoming), allow (outgoing), disabled (routed)".
func parseUFWDefaults(output string) (Defaults, bool) {
	var d Defaults
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(line), "default:") {
			continue
		}
		for _, m := range ufwDefaultRE.FindAllStringSubmatch(line, -1) {
			value := strings.ToLower(m[1])
			switch strings.ToLower(m[2]) {
			case "incoming":
				d.Incoming = value
			case "outgoing":
				d.Outgoing = value
			case "routed":
				d.Routed = value
			}
		}
		return d, d.Incoming != "" && d.Outgoing != ""
	}
	return d, false
}

// parseUFWDefaultFile reads the DEFAULT_*_POLICY settings from /etc/default/ufw.
func parseUFWDefaultFile(path string) (Defaults, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Defaults{}, fmt.Errorf("reading UFW default policy: %w", err)
	}
	var d Defaults
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = strings.ToUpper(strings.Trim(strings.TrimSpace(value), `"'`))
		switch strings.TrimSpace(key) {
		case "DEFAULT_INPUT_POLICY":
			d.Incoming = ufwFilePolicy(value)
		case "DEFAULT_OUTPUT_POLICY":
			d.Outgoing = ufwFilePolicy(value)
		case "DEFAULT_FORWARD_POLICY":
			d.Routed = ufwFilePolicy(value)
			if d.Routed == PolicyDeny {
				d.Routed = "disabled"
			}
		}
	}
	if d.Incoming == "" || d.Outgoing == "" {
		return Defaults{}, fmt.Errorf("could not read the default policy from %s", path)
	}
	return d, nil
}

func ufwFilePolicy(value string) string {
	switch value {
	case "ACCEPT":
		return PolicyAllow
	case "DROP":
		return PolicyDeny
	case "REJECT":
		return PolicyReject
	default:
		return ""
	}
}

// ufwPing reports the echo-request state across before.rules and before6.rules.
// A missing before6.rules (IPv6 off) is ignored.
func (s *Service) ufwPing() string {
	var accept, drop int
	for _, path := range []string{s.beforeRulesPath(), s.before6RulesPath()} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		a, d := countEchoRules(string(data))
		accept += a
		drop += d
	}
	return pingState(accept, drop)
}

func pingState(accept, drop int) string {
	switch {
	case accept > 0 && drop > 0:
		return PingMixed
	case accept > 0:
		return PingAllow
	case drop > 0:
		return PingBlock
	default:
		return PingUnknown
	}
}

func countEchoRules(content string) (accept, drop int) {
	for _, line := range strings.Split(content, "\n") {
		m := ufwEchoRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if m[2] == "ACCEPT" {
			accept++
		} else {
			drop++
		}
	}
	return accept, drop
}

// rewriteEchoRules switches every INPUT echo-request rule to target
// ("ACCEPT" or "DROP") and returns the number of rules found and changed.
func rewriteEchoRules(content, target string) (string, int, int) {
	lines := strings.Split(content, "\n")
	found, changed := 0, 0
	for i, line := range lines {
		m := ufwEchoRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		found++
		if m[2] == target {
			continue
		}
		lines[i] = m[1] + target + m[3]
		changed++
	}
	return strings.Join(lines, "\n"), found, changed
}

type ufwFileEdit struct {
	path     string
	original []byte
	updated  string
	mode     os.FileMode
}

// ufwSetPing edits the echo-request rules in before.rules and before6.rules,
// keeps a .bak of each changed file, then reloads UFW. If anything fails the
// files are put back.
func (s *Service) ufwSetPing(ctx context.Context, mode string) error {
	target := "ACCEPT"
	if mode == PingBlock {
		target = "DROP"
	}

	var edits []ufwFileEdit
	totalFound := 0
	for _, path := range []string{s.beforeRulesPath(), s.before6RulesPath()} {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) && path == s.before6RulesPath() {
				continue // IPv6 is not managed by UFW on this host.
			}
			return fmt.Errorf("reading %s: %w", path, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}
		updated, found, changed := rewriteEchoRules(string(data), target)
		totalFound += found
		if changed > 0 {
			edits = append(edits, ufwFileEdit{path: path, original: data, updated: updated, mode: info.Mode().Perm()})
		}
	}
	if totalFound == 0 {
		return fmt.Errorf("no ICMP echo-request rules found in %s; cannot change ping", s.beforeRulesPath())
	}

	if s.dryRun {
		for _, e := range edits {
			fmt.Printf("[dry-run] would set echo-request to %s in %s\n", target, e.path)
		}
		_, err := s.runner.Run(ctx, "ufw", "reload")
		return err
	}

	for i, e := range edits {
		if err := writeWithBackup(e); err != nil {
			restoreEdits(edits[:i])
			return err
		}
	}
	if _, err := s.runner.Run(ctx, "ufw", "reload"); err != nil {
		restoreEdits(edits)
		return fmt.Errorf("ufw reload: %w", err)
	}
	return nil
}

func writeWithBackup(e ufwFileEdit) error {
	if err := os.WriteFile(e.path+".bak", e.original, e.mode); err != nil {
		return fmt.Errorf("writing backup %s.bak: %w", e.path, err)
	}
	return writeFileAtomic(e.path, []byte(e.updated), e.mode)
}

// restoreEdits puts the original contents back. It is best effort.
func restoreEdits(edits []ufwFileEdit) {
	for _, e := range edits {
		_ = writeFileAtomic(e.path, e.original, e.mode)
	}
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
