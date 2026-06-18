package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MkdirResult records directories created during project setup.
type MkdirResult struct {
	Created []string
}

// mkdirProjectTree creates the project root directory.
func mkdirProjectTree(paths *ValidatedPaths, id RuntimeIdentity, mode os.FileMode) (*MkdirResult, error) {
	if id.Mode == OwnershipShared {
		if err := os.MkdirAll(paths.ProjectPath, mode); err != nil {
			return nil, fmt.Errorf("creating project path: %w", err)
		}
		return &MkdirResult{}, nil
	}

	result := &MkdirResult{}
	created, err := mkdirNewTree(paths.ProjectPath, mode)
	if err != nil {
		return result, err
	}
	result.Created = append(result.Created, created...)
	if err := lchownTree(result.Created, id.UID, id.GID); err != nil {
		return result, err
	}
	return result, nil
}

func mkdirNewTree(target string, mode os.FileMode) ([]string, error) {
	target = filepath.Clean(target)
	if _, err := os.Lstat(target); err == nil {
		return nil, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	var created []string
	current := string(os.PathSeparator)
	clean := filepath.Clean(target)
	parts := strings.Split(strings.TrimPrefix(clean, string(os.PathSeparator)), string(os.PathSeparator))
	for _, part := range parts {
		current = filepath.Join(current, part)
		if _, err := os.Lstat(current); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return created, err
		}
		if err := os.Mkdir(current, mode); err != nil {
			return created, fmt.Errorf("creating %q: %w", current, err)
		}
		created = append(created, current)
	}
	return created, nil
}

func lchownTree(paths []string, uid, gid int) error {
	for _, p := range paths {
		if err := os.Lchown(p, uid, gid); err != nil {
			return fmt.Errorf("setting ownership on %q: %w", p, err)
		}
	}
	return nil
}

// removeCreatedDirs removes only directories Abstrax created during a failed operation.
func removeCreatedDirs(paths []string) {
	for i := len(paths) - 1; i >= 0; i-- {
		_ = os.Remove(paths[i])
	}
}
