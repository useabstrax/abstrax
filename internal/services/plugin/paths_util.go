package plugin

import (
	"os"
	"os/user"
)

func userHomeDir() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return u.HomeDir, nil
}

func isRoot() bool {
	return os.Geteuid() == 0
}

// EffectivePaths returns system paths when running as root, otherwise user paths.
func EffectivePaths() (*Paths, error) {
	if isRoot() {
		return SystemPaths(), nil
	}
	return DefaultPaths()
}
