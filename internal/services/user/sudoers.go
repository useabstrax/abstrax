package user

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	defaultSudoersDir = "/etc/sudoers.d"
	sudoersHeader     = "# Managed by Abstrax - do not edit manually"
	sudoersFilePrefix = "abstrax-"
)

// sudoersPath returns the drop-in path for a user's passwordless sudo rule.
// Filenames must not contain '.' or '~' or sudo ignores them.
func (s *Service) sudoersPath(username string) string {
	dir := s.sudoersDir
	if dir == "" {
		dir = defaultSudoersDir
	}
	return filepath.Join(dir, sudoersFilePrefix+username)
}

// sudoersContent builds the NOPASSWD sudoers drop-in body for username.
func sudoersContent(username string) string {
	return fmt.Sprintf("%s\n%s ALL=(ALL) NOPASSWD:ALL\n", sudoersHeader, username)
}

// writePasswordlessSudo installs a validated sudoers.d drop-in granting
// passwordless sudo to username. Ubuntu/Debian %sudo and RHEL-family %wheel
// require a password by default, which makes sudo unusable for accounts
// created without one.
func (s *Service) writePasswordlessSudo(username string) error {
	path := s.sudoersPath(username)
	if s.dryRun {
		fmt.Printf("[dry-run] would write passwordless sudoers: %s\n", path)
		return nil
	}

	dir := filepath.Dir(path)
	content := sudoersContent(username)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating sudoers.d: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".abstrax-"+username+".*")
	if err != nil {
		return fmt.Errorf("creating temporary sudoers file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := tmp.Chmod(0440); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("setting sudoers permissions: %w", err)
	}
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing sudoers content: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temporary sudoers file: %w", err)
	}

	if err := validateSudoersFile(tmpPath); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("installing sudoers drop-in: %w", err)
	}
	return nil
}

// removePasswordlessSudo removes the Abstrax-managed sudoers drop-in if present.
func (s *Service) removePasswordlessSudo(username string) error {
	path := s.sudoersPath(username)
	if s.dryRun {
		fmt.Printf("[dry-run] would remove passwordless sudoers: %s\n", path)
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing sudoers drop-in %s: %w", path, err)
	}
	return nil
}

func validateSudoersFile(path string) error {
	cmd := exec.Command("visudo", "-cf", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("invalid sudoers drop-in: %s", msg)
	}
	return nil
}
