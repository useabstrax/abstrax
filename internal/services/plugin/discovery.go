package plugin

import (
	"fmt"
	"os"
	"path/filepath"

	"abstrax/internal/validate"
)

// PluginBinaryName returns the executable name for a plugin command.
func PluginBinaryName(command string) string {
	return "abstrax-" + command
}

// Discoverer finds installed plugin binaries.
type Discoverer struct {
	paths *Paths
}

// NewDiscoverer creates a Discoverer with the given paths.
func NewDiscoverer(paths *Paths) *Discoverer {
	return &Discoverer{paths: paths}
}

// FindBinary locates an executable plugin binary for command.
func (d *Discoverer) FindBinary(command string) (string, error) {
	if err := validate.PluginName(command); err != nil {
		return "", err
	}

	binaryName := PluginBinaryName(command)
	searchDirs := d.searchDirs()

	for _, dir := range searchDirs {
		path := filepath.Join(dir, binaryName)
		if isExecutable(path) {
			return path, nil
		}
	}

	for _, dir := range pathEntries() {
		path := filepath.Join(dir, binaryName)
		if isExecutable(path) {
			return path, nil
		}
	}

	return "", fmt.Errorf("%w: %s", ErrNotFound, command)
}

func (d *Discoverer) searchDirs() []string {
	var dirs []string
	dirs = append(dirs, d.paths.SystemPluginDirs...)
	if d.paths.UserPluginDir != "" {
		dirs = append(dirs, d.paths.UserPluginDir)
	}
	if d.paths.InstallDir != "" {
		seen := make(map[string]struct{})
		var unique []string
		for _, dir := range dirs {
			if _, ok := seen[dir]; ok {
				continue
			}
			seen[dir] = struct{}{}
			unique = append(unique, dir)
		}
		if _, ok := seen[d.paths.InstallDir]; !ok {
			unique = append([]string{d.paths.InstallDir}, unique...)
		}
		return unique
	}
	return dirs
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}

func pathEntries() []string {
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		return nil
	}
	cwd, _ := os.Getwd()
	var entries []string
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" || dir == "." || dir == cwd {
			continue
		}
		entries = append(entries, dir)
	}
	return entries
}
