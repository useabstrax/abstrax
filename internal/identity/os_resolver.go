package identity

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	executil "abstrax/internal/exec"
)

// OSResolver resolves accounts using getent and id.
type OSResolver struct {
	runner *executil.Runner
}

// NewOSResolver creates a Resolver backed by system commands.
func NewOSResolver(runner *executil.Runner) *OSResolver {
	return &OSResolver{runner: runner}
}

// Lookup resolves a Linux user account.
func (r *OSResolver) Lookup(ctx context.Context, username string) (*Account, error) {
	res, err := r.runner.RunSilent(ctx, "id", username)
	if err != nil || res.ExitCode != 0 {
		return nil, &NotFoundError{Username: username}
	}

	account := &Account{Username: username}
	parseIDOutput(res.Stdout, account)

	ent, err := r.runner.RunSilent(ctx, "getent", "passwd", username)
	if err != nil || ent.ExitCode != 0 {
		return nil, &NotFoundError{Username: username}
	}

	parts := strings.Split(ent.Stdout, ":")
	if len(parts) < 7 {
		return nil, fmt.Errorf("unexpected passwd entry for user %q", username)
	}

	uid, err := strconv.Atoi(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid uid for user %q", username)
	}
	gid, err := strconv.Atoi(parts[3])
	if err != nil {
		return nil, fmt.Errorf("invalid gid for user %q", username)
	}

	home := parts[5]
	if home == "" {
		return nil, fmt.Errorf("user %q has no home directory configured", username)
	}

	account.UID = uid
	account.GID = gid
	account.Home = resolveRealPath(home)

	group, err := r.primaryGroup(ctx, parts[3])
	if err != nil {
		return nil, err
	}
	account.PrimaryGroup = group

	return account, nil
}

// ListHomes returns resolved home directories for all passwd entries.
func (r *OSResolver) ListHomes(ctx context.Context) ([]HomeEntry, error) {
	res, err := r.runner.RunSilent(ctx, "getent", "passwd")
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}

	var homes []HomeEntry
	for _, line := range strings.Split(res.Stdout, "\n") {
		parts := strings.Split(line, ":")
		if len(parts) < 6 || parts[0] == "" || parts[5] == "" {
			continue
		}
		homes = append(homes, HomeEntry{
			Username: parts[0],
			Home:     resolveRealPath(parts[5]),
		})
	}
	return homes, nil
}

func (r *OSResolver) primaryGroup(ctx context.Context, gid string) (string, error) {
	res, err := r.runner.RunSilent(ctx, "getent", "group", gid)
	if err != nil || res.ExitCode != 0 {
		return "", fmt.Errorf("primary group %q not found", gid)
	}
	parts := strings.Split(res.Stdout, ":")
	if len(parts) < 1 || parts[0] == "" {
		return "", fmt.Errorf("invalid group entry for gid %q", gid)
	}
	return parts[0], nil
}

func parseIDOutput(out string, account *Account) {
	for _, field := range strings.Fields(out) {
		if strings.HasPrefix(field, "uid=") {
			uid, _ := strconv.Atoi(extractID(field))
			account.UID = uid
		} else if strings.HasPrefix(field, "gid=") {
			name := extractName(field)
			if name != "" {
				account.PrimaryGroup = name
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

func resolveRealPath(path string) string {
	clean := filepath.Clean(path)
	real, err := filepath.EvalSymlinks(clean)
	if err != nil {
		if _, statErr := os.Stat(clean); statErr == nil {
			return clean
		}
		return clean
	}
	return filepath.Clean(real)
}
