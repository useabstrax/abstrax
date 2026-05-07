// Package sshkey manages SSH authorized_keys files for Linux users.
package sshkey

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"abstrax/internal/backup"
	executil "abstrax/internal/exec"
)

const managedPrefix = "# abstrax:key"

// Service provides SSH key management.
type Service struct {
	runner *executil.Runner
}

// New creates a Service.
func New(dryRun, verbose bool) *Service {
	return &Service{runner: executil.New(dryRun, verbose)}
}

// Add inserts an SSH public key into a user's authorized_keys file.
func (s *Service) Add(ctx context.Context, opts AddOptions) (*KeyInfo, error) {
	keyData := strings.TrimSpace(opts.Key)
	if keyData == "" {
		return nil, fmt.Errorf("key data is empty")
	}

	authFile, err := authorizedKeysPath(opts.Username)
	if err != nil {
		return nil, err
	}

	if err := ensureSSHDir(opts.Username, authFile); err != nil {
		return nil, err
	}

	fp, err := fingerprint(keyData)
	if err != nil {
		return nil, fmt.Errorf("invalid SSH key: %w", err)
	}

	keyID := opts.Name
	if keyID == "" {
		keyID = shortFingerprint(fp)
	}

	// Check for duplicate unless --force.
	if !opts.Force {
		existing, _ := s.List(ctx, ListOptions{Username: opts.Username})
		for _, k := range existing {
			if k.Fingerprint == fp {
				return &k, fmt.Errorf("key with fingerprint %s already exists (use --force to overwrite)", fp)
			}
		}
	}

	if _, err := backup.File(authFile); err != nil {
		return nil, fmt.Errorf("backing up authorized_keys: %w", err)
	}

	name := opts.Name
	if name == "" {
		name = keyID
	}

	marker := fmt.Sprintf(`%s id=%s name="%s"`, managedPrefix, keyID, name)
	if opts.Comment != "" {
		marker += fmt.Sprintf(` comment="%s"`, opts.Comment)
	}

	entry := marker + "\n" + keyData + "\n"

	f, err := os.OpenFile(authFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("opening authorized_keys: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(entry); err != nil {
		return nil, fmt.Errorf("writing authorized_keys: %w", err)
	}

	return &KeyInfo{
		ID:          keyID,
		Name:        name,
		Fingerprint: fp,
		Managed:     true,
	}, nil
}

// Remove removes a managed key by ID.
func (s *Service) Remove(ctx context.Context, opts RemoveOptions) error {
	authFile, err := authorizedKeysPath(opts.Username)
	if err != nil {
		return err
	}

	if _, err := backup.File(authFile); err != nil {
		return fmt.Errorf("backing up authorized_keys: %w", err)
	}

	keys, err := s.List(ctx, ListOptions{Username: opts.Username})
	if err != nil {
		return err
	}

	found := false
	for _, k := range keys {
		if k.ID == opts.KeyID || (opts.Fingerprint != "" && k.Fingerprint == opts.Fingerprint) {
			if !k.Managed && !opts.Force {
				return fmt.Errorf("key %s is not managed by Abstrax; use --force to remove unmanaged keys", k.ID)
			}
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("key %q not found for user %s", opts.KeyID, opts.Username)
	}

	return rewriteWithoutKey(authFile, opts.KeyID, opts.Fingerprint)
}

// List returns the authorized keys for a user.
func (s *Service) List(_ context.Context, opts ListOptions) ([]KeyInfo, error) {
	authFile, err := authorizedKeysPath(opts.Username)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(authFile)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return parseAuthorizedKeys(f, opts.ManagedOnly), nil
}

// Info returns a single key by ID or fingerprint.
func (s *Service) Info(ctx context.Context, username, keyID string) (*KeyInfo, error) {
	keys, err := s.List(ctx, ListOptions{Username: username})
	if err != nil {
		return nil, err
	}
	for _, k := range keys {
		if k.ID == keyID {
			return &k, nil
		}
	}
	return nil, fmt.Errorf("key %q not found for user %s", keyID, username)
}

func authorizedKeysPath(username string) (string, error) {
	u, err := user.Lookup(username)
	if err != nil {
		return "", fmt.Errorf("user %s not found: %w", username, err)
	}
	return filepath.Join(u.HomeDir, ".ssh", "authorized_keys"), nil
}

func ensureSSHDir(username, authFile string) error {
	sshDir := filepath.Dir(authFile)

	u, err := user.Lookup(username)
	if err != nil {
		return fmt.Errorf("user %s not found: %w", username, err)
	}

	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return fmt.Errorf("creating .ssh dir: %w", err)
	}

	// Ensure correct ownership.
	_ = exec.Command("chown", "-R", u.Username+":"+u.Username, sshDir).Run()

	// Ensure authorized_keys exists with correct permissions.
	f, err := os.OpenFile(authFile, os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("creating authorized_keys: %w", err)
	}
	f.Close()

	_ = exec.Command("chmod", "600", authFile).Run()
	_ = exec.Command("chown", u.Username+":"+u.Username, authFile).Run()

	return nil
}

func fingerprint(keyData string) (string, error) {
	// Write to a temp file and use ssh-keygen to get fingerprint.
	tmp, err := os.CreateTemp("", "abstrax-key-*.pub")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.WriteString(keyData); err != nil {
		tmp.Close()
		return "", err
	}
	tmp.Close()

	out, err := exec.Command("ssh-keygen", "-lf", tmp.Name()).Output()
	if err != nil {
		return "", fmt.Errorf("ssh-keygen: %w", err)
	}

	parts := strings.Fields(string(out))
	if len(parts) >= 2 {
		return parts[1], nil
	}
	return "", fmt.Errorf("unexpected ssh-keygen output")
}

func shortFingerprint(fp string) string {
	parts := strings.Split(fp, ":")
	if len(parts) >= 2 {
		last := parts[len(parts)-1]
		if len(last) > 8 {
			return last[:8]
		}
		return last
	}
	// SHA256 format: SHA256:xxxx...
	if idx := strings.Index(fp, ":"); idx >= 0 {
		rest := fp[idx+1:]
		if len(rest) > 8 {
			return rest[:8]
		}
		return rest
	}
	return fp
}

func parseAuthorizedKeys(f *os.File, managedOnly bool) []KeyInfo {
	var keys []KeyInfo
	scanner := bufio.NewScanner(f)
	lineNum := 0
	var pendingMarker *KeyInfo

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, managedPrefix) {
			pendingMarker = parseMarker(trimmed)
			pendingMarker.Line = lineNum
			continue
		}

		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			pendingMarker = nil
			continue
		}

		// This should be a key line.
		parts := strings.Fields(trimmed)
		if len(parts) < 2 {
			pendingMarker = nil
			continue
		}

		info := KeyInfo{Line: lineNum}
		if pendingMarker != nil {
			info.ID = pendingMarker.ID
			info.Name = pendingMarker.Name
			info.Managed = true
			pendingMarker = nil
		} else {
			if managedOnly {
				continue
			}
			info.ID = fmt.Sprintf("line-%d", lineNum)
		}

		info.Type = parts[0]
		if len(parts) >= 3 {
			info.Comment = parts[2]
		}

		// Get fingerprint if ssh-keygen available.
		if fp, err := fingerprint(trimmed); err == nil {
			info.Fingerprint = fp
		}

		keys = append(keys, info)
	}

	return keys
}

func parseMarker(line string) *KeyInfo {
	k := &KeyInfo{Managed: true}
	rest := strings.TrimPrefix(line, managedPrefix)

	for _, part := range strings.Fields(rest) {
		if strings.HasPrefix(part, "id=") {
			k.ID = strings.TrimPrefix(part, "id=")
		} else if strings.HasPrefix(part, "name=") {
			k.Name = strings.Trim(strings.TrimPrefix(part, "name="), `"`)
		}
	}
	if k.ID == "" {
		k.ID = "unknown"
	}
	if k.Name == "" {
		k.Name = k.ID
	}
	return k
}

func rewriteWithoutKey(authFile, keyID, fp string) error {
	f, err := os.Open(authFile)
	if err != nil {
		return err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	skipNext := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, managedPrefix) {
			marker := parseMarker(trimmed)
			if marker.ID == keyID {
				skipNext = true
				continue
			}
		}

		if skipNext {
			skipNext = false
			// Also skip fingerprint match.
			if fp != "" && strings.Contains(line, fp) {
				continue
			}
			continue
		}

		lines = append(lines, line)
	}

	return os.WriteFile(authFile, []byte(strings.Join(lines, "\n")+"\n"), 0600)
}
