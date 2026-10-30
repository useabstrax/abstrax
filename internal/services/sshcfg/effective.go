package sshcfg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	// legacyManagedFile is the previous drop-in name. sshd reads it after
	// cloud-init files, so the next write moves it to the managed path.
	legacyManagedFile = "99-abstrax.conf"
	managedDropInName = "00-abstrax.conf"
)

func (s *Service) applyDirective(ctx context.Context, key, value string, dryRun bool) error {
	if err := s.writeDirective(key, value, dryRun); err != nil {
		return err
	}
	if dryRun || s.dryRun {
		return nil
	}
	if err := s.validateAndReload(ctx); err != nil {
		return err
	}
	return s.verifyEffective(ctx, key, value)
}

// writeDirective records key in the managed drop-in and, when that file would
// not be the first value sshd reads, in sshd_config as well.
func (s *Service) writeDirective(key, value string, dryRun bool) error {
	if err := s.migrateLegacy(dryRun); err != nil {
		return err
	}
	if err := s.ensureMainConfig(key, value, dryRun); err != nil {
		return err
	}
	return s.writeManagedDirective(key, value, dryRun)
}

func (s *Service) legacyConfigPath() string {
	managed := s.managedConfigPath()
	legacy := filepath.Join(filepath.Dir(managed), legacyManagedFile)
	if filepath.Clean(legacy) == filepath.Clean(managed) {
		return ""
	}
	return legacy
}

func (s *Service) migrateLegacy(dryRun bool) error {
	legacy := s.legacyConfigPath()
	if legacy == "" {
		return nil
	}
	if _, err := os.Stat(legacy); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}

	managed := s.managedConfigPath()
	if dryRun || s.dryRun {
		fmt.Printf("[dry-run] would move %s to %s\n", legacy, managed)
		return nil
	}

	legacyEntries, err := readConfigFile(legacy)
	if err != nil {
		return err
	}
	current, err := readConfigFile(managed)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(managed), 0755); err != nil {
		return fmt.Errorf("creating sshd_config.d: %w", err)
	}
	if err := writeConfigFile(managed, mergeManaged(legacyEntries, current)); err != nil {
		return err
	}
	return os.Remove(legacy)
}

func mergeManaged(legacy, current []ConfigEntry) []ConfigEntry {
	if len(current) == 0 {
		return append([]ConfigEntry(nil), legacy...)
	}
	out := append([]ConfigEntry(nil), current...)
	for _, entry := range legacy {
		if strings.EqualFold(entry.Key, "Port") {
			if !hasEntryValue(out, "Port", entry.Value) {
				out = append(out, entry)
			}
			continue
		}
		if !hasEntryKey(out, entry.Key) {
			out = append(out, entry)
		}
	}
	return out
}

func hasEntryKey(entries []ConfigEntry, key string) bool {
	for _, entry := range entries {
		if strings.EqualFold(entry.Key, key) {
			return true
		}
	}
	return false
}

func hasEntryValue(entries []ConfigEntry, key, value string) bool {
	for _, entry := range entries {
		if strings.EqualFold(entry.Key, key) && entry.Value == value {
			return true
		}
	}
	return false
}

func (s *Service) ensureMainConfig(key, value string, dryRun bool) error {
	first, err := s.managedReadFirst(key)
	if err != nil || first {
		return err
	}

	path := s.mainConfig()
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	current, ok := globalDirectiveValue(string(data), key)
	if ok && strings.EqualFold(current, value) {
		return nil
	}
	if dryRun || s.dryRun {
		fmt.Printf("[dry-run] would set %s %s in %s\n", key, value, path)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(rewriteGlobalDirective(string(data), key, value)), 0644)
}

// managedReadFirst reports whether an Include that loads the managed drop-in
// appears before this keyword in sshd_config. sshd keeps the first value.
func (s *Service) managedReadFirst(key string) (bool, error) {
	data, err := os.ReadFile(s.mainConfig())
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if isMatchBlock(trimmed) {
			return false, nil
		}
		if patterns, ok := includePatterns(trimmed); ok {
			for _, pattern := range patterns {
				resolved := resolveIncludePattern(pattern, s.mainConfig())
				if patternMatches(resolved, s.managedConfigPath()) {
					return true, nil
				}
			}
			continue
		}
		if k, _, ok := parseDirective(trimmed); ok && strings.EqualFold(k, key) {
			return false, nil
		}
	}
	return false, nil
}

func (s *Service) writeManagedDirective(key, value string, dryRun bool) error {
	path := s.managedConfigPath()
	if dryRun || s.dryRun {
		fmt.Printf("[dry-run] would set %s %s in %s\n", key, value, path)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating sshd_config.d: %w", err)
	}
	entries, err := readConfigFile(path)
	if err != nil {
		return err
	}
	return writeConfigFile(path, upsertEntry(entries, key, value))
}

func upsertEntry(entries []ConfigEntry, key, value string) []ConfigEntry {
	for i, entry := range entries {
		if strings.EqualFold(entry.Key, key) {
			entries[i].Key = key
			entries[i].Value = value
			return entries
		}
	}
	return append(entries, ConfigEntry{Key: key, Value: value})
}

func (s *Service) verifyEffective(ctx context.Context, key, value string) error {
	got, err := s.effectiveKeyword(ctx, key)
	if err != nil {
		return err
	}
	if strings.EqualFold(got, value) {
		return nil
	}
	if got == "" {
		got = "unset"
	}
	if src := s.firstSourcePath(key); src != "" {
		return fmt.Errorf("%s is still %s: %s sets it first", key, got, src)
	}
	return fmt.Errorf("%s is still %s", key, got)
}

func (s *Service) effectiveKeyword(ctx context.Context, key string) (string, error) {
	if s.runner == nil {
		return "", fmt.Errorf("sshd -T is unavailable")
	}
	res, err := s.runner.RunSilent(ctx, "sshd", "-T")
	if err != nil || res.ExitCode != 0 {
		detail := strings.TrimSpace(res.Stderr)
		if detail == "" && err != nil {
			detail = err.Error()
		}
		if detail == "" {
			detail = "sshd -T failed"
		}
		return "", fmt.Errorf("sshd -T failed: %s", detail)
	}
	return firstDumpValue(res.Stdout, key), nil
}

func firstDumpValue(out, key string) string {
	want := normalizeKeyword(key)
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || normalizeKeyword(fields[0]) != want {
			continue
		}
		return strings.Join(fields[1:], " ")
	}
	return ""
}

func normalizeKeyword(key string) string {
	return strings.ToLower(strings.ReplaceAll(key, "-", ""))
}

func (s *Service) firstSourcePath(key string) string {
	var path string
	_ = s.walkDirectives(func(candidate, k, _ string) {
		if path == "" && strings.EqualFold(k, key) {
			path = candidate
		}
	})
	return path
}

type directiveHit struct {
	path  string
	key   string
	value string
}

func (s *Service) settingsFor(key string) []directiveHit {
	var hits []directiveHit
	_ = s.walkDirectives(func(path, k, value string) {
		if strings.EqualFold(k, key) {
			hits = append(hits, directiveHit{path: path, key: k, value: value})
		}
	})
	return hits
}

// effectiveValues returns the first value sshd would keep for each keyword.
func (s *Service) effectiveValues() map[string]string {
	vals := map[string]string{}
	_ = s.walkDirectives(func(_ string, key, value string) {
		k := normalizeKeyword(key)
		if _, seen := vals[k]; seen {
			return
		}
		vals[k] = value
	})
	return vals
}

type visitFunc func(path, key, value string)

func (s *Service) walkDirectives(visit visitFunc) error {
	seen := map[string]bool{}
	return s.walkFile(s.mainConfig(), seen, visit)
}

func (s *Service) walkFile(path string, seen map[string]bool, visit visitFunc) error {
	clean := filepath.Clean(path)
	if seen[clean] {
		return nil
	}
	seen[clean] = true

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if isMatchBlock(trimmed) {
			return nil
		}
		if patterns, ok := includePatterns(trimmed); ok {
			for _, pattern := range patterns {
				matches, err := filepath.Glob(resolveIncludePattern(pattern, path))
				if err != nil {
					return err
				}
				sort.Strings(matches)
				for _, match := range matches {
					if err := s.walkFile(match, seen, visit); err != nil {
						return err
					}
				}
			}
			continue
		}
		key, value, ok := parseDirective(trimmed)
		if ok {
			visit(path, key, value)
		}
	}
	return nil
}

func includePatterns(line string) ([]string, bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 || !strings.EqualFold(fields[0], "Include") {
		return nil, false
	}
	patterns := make([]string, 0, len(fields)-1)
	for _, field := range fields[1:] {
		patterns = append(patterns, strings.Trim(field, `"'`))
	}
	return patterns, true
}

func resolveIncludePattern(pattern, configPath string) string {
	pattern = strings.Trim(pattern, `"'`)
	if filepath.IsAbs(pattern) {
		return filepath.Clean(pattern)
	}
	return filepath.Clean(filepath.Join(filepath.Dir(configPath), pattern))
}

func patternMatches(pattern, candidate string) bool {
	pattern = filepath.Clean(pattern)
	candidate = filepath.Clean(candidate)
	ok, err := filepath.Match(filepath.Base(pattern), filepath.Base(candidate))
	if err != nil || !ok {
		return false
	}
	return filepath.Dir(pattern) == filepath.Dir(candidate)
}

func isMatchBlock(line string) bool {
	fields := strings.Fields(line)
	return len(fields) > 0 && strings.EqualFold(fields[0], "Match")
}

func parseDirective(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	sep := strings.IndexAny(line, " \t=")
	if sep <= 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:sep])
	value := strings.TrimLeft(strings.TrimSpace(line[sep+1:]), "= \t")
	if key == "" || value == "" {
		return "", "", false
	}
	return key, value, true
}

func globalDirectiveValue(content, key string) (string, bool) {
	body := strings.TrimRight(content, "\n")
	if body == "" {
		return "", false
	}
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if isMatchBlock(trimmed) {
			return "", false
		}
		if k, value, ok := parseDirective(trimmed); ok && strings.EqualFold(k, key) {
			return value, true
		}
	}
	return "", false
}

func rewriteGlobalDirective(content, key, value string) string {
	body := strings.TrimRight(content, "\n")
	var lines []string
	if body != "" {
		lines = strings.Split(body, "\n")
	}
	replaced := false
	insertAt := len(lines)
	for i, line := range lines {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if isMatchBlock(trimmed) {
			insertAt = i
			break
		}
		if k, _, ok := parseDirective(trimmed); ok && strings.EqualFold(k, key) {
			lines[i] = key + " " + value
			replaced = true
			break
		}
	}
	if !replaced {
		lines = insertLine(lines, insertAt, key+" "+value)
	}
	return strings.Join(lines, "\n") + "\n"
}

func insertLine(lines []string, at int, line string) []string {
	if at < 0 || at > len(lines) {
		at = len(lines)
	}
	lines = append(lines, "")
	copy(lines[at+1:], lines[at:])
	lines[at] = line
	return lines
}
