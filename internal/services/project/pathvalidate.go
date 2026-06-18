package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"abstrax/internal/identity"
)

var forbiddenProjectRoots = []string{
	"/",
	"/home",
	"/var",
	"/var/www",
}

// ValidatedPaths holds symlink-safe resolved project paths.
type ValidatedPaths struct {
	ProjectPath  string
	PublicPath   string
	DocumentRoot string
	ApprovedRoot string
}

// PathValidateOptions configures project path validation.
type PathValidateOptions struct {
	RequestedPath string
	ProjectName   string
	PublicDir     string
	WebRoot       string
	Identity      RuntimeIdentity
	ApprovedRoots []string
	Homes         []identity.HomeEntry
}

// ValidateProjectPath validates and resolves the project path for creation.
func ValidateProjectPath(opts PathValidateOptions) (*ValidatedPaths, error) {
	if err := rejectTraversal(opts.RequestedPath); err != nil {
		return nil, err
	}

	resolved, err := resolveExistingPath(opts.RequestedPath)
	if err != nil {
		return nil, err
	}

	if err := rejectForbiddenRoots(resolved); err != nil {
		return nil, err
	}

	documentRoot, publicPath, err := resolveDocumentRoot(resolved, opts.PublicDir, opts.WebRoot)
	if err != nil {
		return nil, err
	}

	if err := ensureInside(resolved, documentRoot, "public path"); err != nil {
		return nil, err
	}

	if opts.Identity.Mode == OwnershipIsolated {
		approvedRoot, err := validateIsolatedPath(resolved, opts)
		if err != nil {
			return nil, err
		}
		if err := validateExistingOwnership(resolved, opts.Identity); err != nil {
			return nil, err
		}
		return &ValidatedPaths{
			ProjectPath:  resolved,
			PublicPath:   publicPath,
			DocumentRoot: documentRoot,
			ApprovedRoot: approvedRoot,
		}, nil
	}

	if err := validateExistingOwnershipShared(resolved); err != nil {
		return nil, err
	}

	return &ValidatedPaths{
		ProjectPath:  resolved,
		PublicPath:   publicPath,
		DocumentRoot: documentRoot,
	}, nil
}

func rejectTraversal(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("project path must be absolute, got %q", path)
	}
	clean := filepath.Clean(path)
	if strings.Contains(path, "/../") || strings.HasSuffix(path, "/..") || strings.HasPrefix(path, "../") {
		return fmt.Errorf("project path contains unsafe traversal: %q", path)
	}
	if clean != filepath.Clean(strings.TrimSuffix(path, "/")) && strings.Contains(path, "..") {
		return fmt.Errorf("project path contains unsafe traversal: %q", path)
	}
	return nil
}

func resolveExistingPath(path string) (string, error) {
	clean := filepath.Clean(path)
	current := "/"
	parts := strings.Split(strings.TrimPrefix(clean, "/"), "/")
	for i, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return "", fmt.Errorf("project path contains unsafe traversal: %q", path)
		}

		next := filepath.Join(current, part)
		info, err := os.Lstat(next)
		if err != nil {
			if os.IsNotExist(err) {
				remaining := parts[i:]
				suffix := strings.Join(remaining, "/")
				unresolved := filepath.Join(current, suffix)
				return filepath.Clean(unresolved), nil
			}
			return "", fmt.Errorf("resolving project path: %w", err)
		}

		if info.Mode()&os.ModeSymlink != 0 {
			target, err := filepath.EvalSymlinks(next)
			if err != nil {
				return "", fmt.Errorf("resolving symlink %q: %w", next, err)
			}
			next = filepath.Clean(target)
		}
		current = next
	}
	return filepath.Clean(current), nil
}

func rejectForbiddenRoots(path string) error {
	for _, forbidden := range forbiddenProjectRoots {
		if path == forbidden {
			return fmt.Errorf("project path cannot be %s", forbidden)
		}
	}
	return nil
}

func resolveDocumentRoot(projectPath, publicDir, webRoot string) (documentRoot, publicPath string, err error) {
	if webRoot != "" {
		documentRoot = filepath.Clean(webRoot)
	} else if publicDir != "" {
		documentRoot = filepath.Join(projectPath, publicDir)
		publicPath = documentRoot
	} else if publicDir == "" && webRoot == "" {
		documentRoot = projectPath
	}
	if publicPath == "" {
		publicPath = documentRoot
	}
	return documentRoot, filepath.Clean(publicPath), nil
}

func ensureInside(parent, child, label string) error {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	if child == parent {
		return nil
	}
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return fmt.Errorf("checking %s: %w", label, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("%s %q escapes project directory %q", label, child, parent)
	}
	return nil
}

func validateIsolatedPath(resolved string, opts PathValidateOptions) (string, error) {
	id := opts.Identity
	ownHome := normalizeComparablePath(id.Home)
	if resolved == ownHome {
		return "", fmt.Errorf("project path cannot be the home directory of user %s (%s)", id.User, id.Home)
	}

	// Allow paths inside the selected user's home before checking other passwd
	// entries. Some systems include accounts with home "/" that would otherwise
	// match every path on the filesystem.
	if isStrictChild(ownHome, resolved) {
		return "", nil
	}

	for _, entry := range opts.Homes {
		if entry.Username == id.User {
			continue
		}
		home := normalizeComparablePath(entry.Home)
		if home == "" || home == ownHome {
			continue
		}
		if isStrictChild(home, resolved) || normalizeComparablePath(resolved) == home {
			return "", fmt.Errorf(
				"The path %s is inside another user's home directory and cannot be used for user %s.\n\nChoose a path inside %s's home directory, use an approved shared project root, or select the matching project user.",
				resolved, id.User, id.User,
			)
		}
	}

	parent, err := nearestExistingParent(resolved)
	if err != nil {
		return "", err
	}
	if err := rejectForeignHomeParent(parent, opts.Homes, id.User, ownHome); err != nil {
		return "", err
	}

	approvedRoot, err := matchApprovedRoot(resolved, opts.ApprovedRoots)
	if err != nil {
		return "", err
	}
	if approvedRoot == resolved {
		return "", fmt.Errorf("project path cannot be the approved root itself (%s)", approvedRoot)
	}
	return approvedRoot, nil
}

func matchApprovedRoot(path string, roots []string) (string, error) {
	path = normalizeComparablePath(path)
	var matched string
	for _, root := range roots {
		root = normalizeComparablePath(root)
		if root == "" {
			continue
		}
		if path == root {
			return root, nil
		}
		if isStrictChild(root, path) {
			if matched == "" || len(root) > len(matched) {
				matched = root
			}
		}
	}
	if matched == "" {
		return "", fmt.Errorf(
			"project path %q is outside user home and not inside an approved shared project root; configure projects.approved_roots in /etc/abstrax/config.json",
			path,
		)
	}
	return matched, nil
}

func nearestExistingParent(path string) (string, error) {
	current := filepath.Clean(path)
	for current != "/" && current != "." {
		if _, err := os.Lstat(current); err == nil {
			resolved, err := resolveExistingPath(current)
			if err != nil {
				return "", err
			}
			return resolved, nil
		}
		current = filepath.Dir(current)
	}
	return "/", nil
}

func rejectForeignHomeParent(parent string, homes []identity.HomeEntry, selectedUser, ownHome string) error {
	parent = normalizeComparablePath(parent)
	for _, entry := range homes {
		if entry.Username == selectedUser {
			continue
		}
		home := normalizeComparablePath(entry.Home)
		if home == "" || home == ownHome {
			continue
		}
		if parent == home || isStrictChild(home, parent) {
			return fmt.Errorf(
				"The path cannot be created because parent directory %s is inside another user's home directory.\n\nChoose a path inside %s's home directory, use an approved shared project root, or select the matching project user.",
				parent, selectedUser,
			)
		}
	}
	return nil
}

func validateExistingOwnership(path string, id RuntimeIdentity) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspecting project path: %w", err)
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("unable to read ownership for %q", path)
	}

	if int(stat.Uid) == id.UID {
		return nil
	}

	owner := lookupUsername(int(stat.Uid))
	if isDirEmpty(path) {
		return fmt.Errorf(
			"project directory %q exists but is owned by %s; Abstrax will not change ownership of existing directories owned by another user",
			path, owner,
		)
	}
	return fmt.Errorf(
		"project directory %q exists and is owned by %s; Abstrax will not recursively change ownership of another user's files",
		path, owner,
	)
}

func validateExistingOwnershipShared(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspecting project path: %w", err)
	}
	_ = info
	return nil
}

func isDirEmpty(path string) bool {
	entries, err := os.ReadDir(path)
	return err == nil && len(entries) == 0
}

func lookupUsername(uid int) string {
	// Best-effort for error messages; tests may not have NSS lookups.
	return fmt.Sprintf("uid %d", uid)
}

func isStrictChild(parent, child string) bool {
	parent = normalizeComparablePath(parent)
	child = normalizeComparablePath(child)
	if parent == child {
		return false
	}
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func normalizeComparablePath(path string) string {
	clean := filepath.Clean(path)
	real, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return clean
	}
	return filepath.Clean(real)
}
