package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ensureWebTraverseAccess grants nginx (www-data) traverse permission into an
// isolated project using the standard "other execute" bit on directories along
// the path. This avoids filesystem ACLs: www-data can reach the public
// directory but cannot list or read private home contents.
func ensureWebTraverseAccess(paths *ValidatedPaths, id RuntimeIdentity) error {
	if paths == nil {
		return nil
	}

	for _, dir := range webTraverseDirs(paths, id.Home) {
		if err := addOtherTraverse(dir); err != nil {
			return fmt.Errorf("granting web server traverse on %q: %w", dir, err)
		}
	}

	if isDeployStylePublicPath(paths.ProjectPath, paths.PublicPath) {
		for _, dir := range []string{
			filepath.Join(paths.ProjectPath, "releases"),
			filepath.Join(paths.ProjectPath, "shared"),
		} {
			if err := addOtherTraverse(dir); err != nil {
				return fmt.Errorf("granting web server traverse on %q: %w", dir, err)
			}
		}
	}

	return nil
}

func webTraverseDirs(paths *ValidatedPaths, userHome string) []string {
	project := filepath.Clean(paths.ProjectPath)
	end := filepath.Clean(paths.PublicPath)
	if end != project {
		end = filepath.Dir(end)
	} else {
		end = project
	}

	var chain []string
	current := end
	for {
		chain = append([]string{current}, chain...)
		if current == "/" {
			break
		}
		current = filepath.Dir(current)
	}

	if userHome != "" && isStrictChild(userHome, project) {
		var filtered []string
		home := filepath.Clean(userHome)
		for _, p := range chain {
			if p == home || isStrictChild(home, p) {
				filtered = append(filtered, p)
			}
		}
		return existingDirs(filtered)
	}

	if paths.ApprovedRoot != "" {
		root := filepath.Clean(paths.ApprovedRoot)
		var filtered []string
		for _, p := range chain {
			if p == root || isStrictChild(root, p) {
				filtered = append(filtered, p)
			}
		}
		parents := parentsFromRoot(root)
		return existingDirs(append(parents, filtered...))
	}

	return existingDirs(chain)
}

func parentsFromRoot(root string) []string {
	var parents []string
	current := filepath.Dir(root)
	for current != "/" && current != "." {
		parents = append([]string{current}, parents...)
		current = filepath.Dir(current)
	}
	return parents
}

func existingDirs(paths []string) []string {
	var dirs []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil || !info.IsDir() {
			continue
		}
		dirs = append(dirs, p)
	}
	return dirs
}

func addOtherTraverse(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}

	mode := info.Mode().Perm()
	if mode&0001 != 0 {
		return nil
	}
	return os.Chmod(path, mode|0001)
}

func isDeployStylePublicPath(projectPath, publicPath string) bool {
	if projectPath == "" || publicPath == "" {
		return false
	}
	rel, err := filepath.Rel(projectPath, publicPath)
	if err != nil {
		return false
	}
	rel = filepath.Clean(rel)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	return len(parts) > 0 && parts[0] == "current"
}
