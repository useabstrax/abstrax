// Package user provides user management operations for Linux systems.
package user

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	executil "abstrax/internal/exec"
)

// Service provides user management methods.
type Service struct {
	runner *executil.Runner
}

// New creates a Service.
func New(dryRun, verbose bool) *Service {
	return &Service{runner: executil.New(dryRun, verbose)}
}

// Add creates a Linux user. The operation is idempotent - if the user already
// exists a success result is returned with AlreadyExisted=true.
func (s *Service) Add(ctx context.Context, opts AddOptions) (*AddResult, error) {
	if exists, _ := s.exists(ctx, opts.Username); exists {
		info, err := s.Info(ctx, opts.Username)
		if err != nil {
			return nil, err
		}
		return &AddResult{
			Username:       opts.Username,
			UID:            info.UID,
			Home:           info.Home,
			Shell:          info.Shell,
			Groups:         info.Groups,
			Sudo:           info.IsSudo,
			Created:        false,
			AlreadyExisted: true,
		}, nil
	}

	args := []string{}

	if opts.System {
		args = append(args, "--system")
	}
	if opts.DisabledPassword {
		args = append(args, "--disabled-password")
	}
	if opts.Comment != "" {
		args = append(args, "--gecos", opts.Comment)
	}
	if opts.Shell != "" {
		args = append(args, "--shell", opts.Shell)
	}
	if opts.UID != "" {
		args = append(args, "--uid", opts.UID)
	}
	if opts.NoCreateHome {
		args = append(args, "--no-create-home")
	}

	// adduser on Debian/Ubuntu requires --disabled-password to avoid
	// interactive password prompts when no password is being set.
	if !opts.DisabledPassword && opts.Password == "" {
		args = append(args, "--disabled-password")
	}

	// Provide an empty GECOS field to suppress the interactive prompt
	// for full name / room / phone etc.
	if opts.Comment == "" {
		args = append(args, "--gecos", "")
	}
	if len(opts.Groups) > 0 {
		args = append(args, "--add-extra-groups")
		args = append(args, strings.Join(opts.Groups, ","))
	}

	args = append(args, opts.Username)

	if _, err := s.runner.Run(ctx, "adduser", args...); err != nil {
		return nil, fmt.Errorf("creating user %s: %w", opts.Username, err)
	}

	if opts.GrantSudo {
		if err := s.addToGroup(ctx, opts.Username, "sudo"); err != nil {
			return nil, fmt.Errorf("granting sudo to %s: %w", opts.Username, err)
		}
	}

	if opts.Password != "" {
		if _, err := s.runner.Run(ctx, "bash", "-c",
			fmt.Sprintf("echo '%s:%s' | chpasswd", opts.Username, opts.Password)); err != nil {
			return nil, fmt.Errorf("setting password for %s: %w", opts.Username, err)
		}
	}

	info, err := s.Info(ctx, opts.Username)
	if err != nil {
		return nil, err
	}

	return &AddResult{
		Username: info.Username,
		UID:      info.UID,
		Home:     info.Home,
		Shell:    info.Shell,
		Groups:   info.Groups,
		Sudo:     info.IsSudo,
		Created:  true,
	}, nil
}

// Remove removes a Linux user.
func (s *Service) Remove(ctx context.Context, opts RemoveOptions) error {
	if exists, _ := s.exists(ctx, opts.Username); !exists {
		return fmt.Errorf("user %s does not exist", opts.Username)
	}

	if opts.KillProcesses {
		// Best effort - ignore errors.
		_, _ = s.runner.Run(ctx, "pkill", "-u", opts.Username)
	}

	args := []string{}
	if opts.DeleteHome {
		args = append(args, "--remove-home")
	}
	args = append(args, opts.Username)

	if _, err := s.runner.Run(ctx, "deluser", args...); err != nil {
		return fmt.Errorf("removing user %s: %w", opts.Username, err)
	}

	if opts.RemoveCron {
		_, _ = s.runner.Run(ctx, "crontab", "-r", "-u", opts.Username)
	}

	return nil
}

// GrantSudo adds a user to the sudo group.
func (s *Service) GrantSudo(ctx context.Context, username string, dryRun bool) error {
	return s.addToGroup(ctx, username, "sudo")
}

// RevokeSudo removes a user from the sudo group.
func (s *Service) RevokeSudo(ctx context.Context, username string, dryRun bool) error {
	return s.removeFromGroup(ctx, username, "sudo")
}

// SetGroups sets a user's supplementary groups, replacing existing ones.
func (s *Service) SetGroups(ctx context.Context, opts ModifyGroupsOptions) error {
	if len(opts.Groups) == 0 {
		return fmt.Errorf("at least one group must be specified")
	}
	_, err := s.runner.Run(ctx, "usermod", "-G", strings.Join(opts.Groups, ","), opts.Username)
	if err != nil {
		return fmt.Errorf("setting groups for %s: %w", opts.Username, err)
	}
	return nil
}

// AddGroups appends groups to a user's supplementary group list.
func (s *Service) AddGroups(ctx context.Context, opts ModifyGroupsOptions) error {
	for _, g := range opts.Groups {
		if err := s.addToGroup(ctx, opts.Username, g); err != nil {
			return err
		}
	}
	return nil
}

// RemoveGroups removes groups from a user's supplementary group list.
func (s *Service) RemoveGroups(ctx context.Context, opts ModifyGroupsOptions) error {
	for _, g := range opts.Groups {
		if err := s.removeFromGroup(ctx, opts.Username, g); err != nil {
			return err
		}
	}
	return nil
}

// SetShell changes a user's login shell.
func (s *Service) SetShell(ctx context.Context, opts SetShellOptions) error {
	_, err := s.runner.Run(ctx, "usermod", "-s", opts.Shell, opts.Username)
	if err != nil {
		return fmt.Errorf("setting shell for %s: %w", opts.Username, err)
	}
	return nil
}

// Lock disables a user account.
func (s *Service) Lock(ctx context.Context, opts LockOptions) error {
	_, err := s.runner.Run(ctx, "usermod", "--lock", opts.Username)
	if err != nil {
		return fmt.Errorf("locking user %s: %w", opts.Username, err)
	}
	return nil
}

// Unlock enables a user account.
func (s *Service) Unlock(ctx context.Context, opts LockOptions) error {
	_, err := s.runner.Run(ctx, "usermod", "--unlock", opts.Username)
	if err != nil {
		return fmt.Errorf("unlocking user %s: %w", opts.Username, err)
	}
	return nil
}

// Info returns information about a user.
func (s *Service) Info(ctx context.Context, username string) (*UserInfo, error) {
	res, err := s.runner.RunSilent(ctx, "id", username)
	if err != nil {
		return nil, fmt.Errorf("user %s not found", username)
	}

	info := &UserInfo{Username: username}
	parseIDOutput(res.Stdout, info)

	// Check if sudo group member.
	for _, g := range info.Groups {
		if g == "sudo" || g == "wheel" {
			info.IsSudo = true
			break
		}
	}

	// Get home and shell from getent.
	if ent, err := s.runner.RunSilent(ctx, "getent", "passwd", username); err == nil {
		parts := strings.Split(ent.Stdout, ":")
		if len(parts) >= 7 {
			uid, _ := strconv.Atoi(parts[2])
			info.UID = parts[2]
			info.GID = parts[3]
			info.Comment = parts[4]
			info.Home = parts[5]
			info.Shell = parts[6]
			info.IsSystem = uid < 1000 && uid != 0
		}
	}

	// Check if locked (passwd -S).
	if res, err := s.runner.RunSilent(ctx, "passwd", "-S", username); err == nil {
		parts := strings.Fields(res.Stdout)
		if len(parts) >= 2 && parts[1] == "L" {
			info.Locked = true
		}
	}

	return info, nil
}

// List returns all users optionally filtered by type.
func (s *Service) List(ctx context.Context, opts ListOptions) ([]UserInfo, error) {
	res, err := s.runner.RunSilent(ctx, "getent", "passwd")
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}

	var users []UserInfo
	for _, line := range strings.Split(res.Stdout, "\n") {
		parts := strings.Split(line, ":")
		if len(parts) < 7 {
			continue
		}
		uid, _ := strconv.Atoi(parts[2])
		isSystem := uid < 1000

		if opts.Regular && isSystem {
			continue
		}
		if opts.System && !isSystem {
			continue
		}

		u := UserInfo{
			Username: parts[0],
			UID:      parts[2],
			GID:      parts[3],
			Comment:  parts[4],
			Home:     parts[5],
			Shell:    strings.TrimSpace(parts[6]),
			IsSystem: isSystem,
		}

		// Get groups.
		if grpRes, err := s.runner.RunSilent(ctx, "id", "-Gn", u.Username); err == nil {
			u.Groups = strings.Fields(grpRes.Stdout)
			for _, g := range u.Groups {
				if g == "sudo" || g == "wheel" {
					u.IsSudo = true
					break
				}
			}
		}

		if opts.Sudo && !u.IsSudo {
			continue
		}

		users = append(users, u)
	}

	return users, nil
}

// exists checks whether a user exists.
func (s *Service) exists(ctx context.Context, username string) (bool, error) {
	res, err := s.runner.RunSilent(ctx, "id", username)
	if err != nil {
		return false, nil
	}
	return res.ExitCode == 0, nil
}

func (s *Service) addToGroup(ctx context.Context, username, group string) error {
	_, err := s.runner.Run(ctx, "usermod", "-aG", group, username)
	if err != nil {
		return fmt.Errorf("adding %s to group %s: %w", username, group, err)
	}
	return nil
}

func (s *Service) removeFromGroup(ctx context.Context, username, group string) error {
	_, err := s.runner.Run(ctx, "gpasswd", "-d", username, group)
	if err != nil {
		return fmt.Errorf("removing %s from group %s: %w", username, group, err)
	}
	return nil
}

// parseIDOutput parses `id username` output into a UserInfo.
// Example: uid=1000(mike) gid=1000(mike) groups=1000(mike),4(adm),27(sudo)
func parseIDOutput(out string, info *UserInfo) {
	for _, field := range strings.Fields(out) {
		if strings.HasPrefix(field, "uid=") {
			info.UID = extractID(field)
		} else if strings.HasPrefix(field, "groups=") {
			raw := strings.TrimPrefix(field, "groups=")
			for _, g := range strings.Split(raw, ",") {
				name := extractName(g)
				if name != "" {
					info.Groups = append(info.Groups, name)
				}
			}
		}
	}
}

func extractID(s string) string {
	eq := strings.Index(s, "=")
	if eq < 0 {
		return ""
	}
	rest := s[eq+1:]
	paren := strings.Index(rest, "(")
	if paren < 0 {
		return rest
	}
	return rest[:paren]
}

func extractName(s string) string {
	open := strings.Index(s, "(")
	close := strings.Index(s, ")")
	if open < 0 || close < 0 || close < open {
		return ""
	}
	return s[open+1 : close]
}

// SudoGroup attempts to detect the correct sudo group for the current OS.
// Defaults to "sudo" (Debian/Ubuntu).
func SudoGroup() string {
	// Check /etc/group for the sudo group first.
	for _, candidate := range []string{"sudo", "wheel", "admin"} {
		cmd := exec.Command("getent", "group", candidate)
		if err := cmd.Run(); err == nil {
			return candidate
		}
	}
	return "sudo"
}
