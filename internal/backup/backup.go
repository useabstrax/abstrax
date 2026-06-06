// Package backup provides safe file backup helpers. Commands that modify system
// configuration files should call backup.File before writing changes.
package backup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// File creates a timestamped backup of the given path alongside the original.
// For example /etc/ssh/sshd_config becomes
// /etc/ssh/sshd_config.abstrax-bak.20240101T120000.
//
// Returns the backup path, or an error. If the source file does not exist the
// function returns ("", nil) - nothing to back up.
func File(src string) (string, error) {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return "", nil
	}

	stamp := time.Now().Format("20060102T150405")
	dst := src + ".abstrax-bak." + stamp

	if err := copyFile(src, dst); err != nil {
		return "", fmt.Errorf("backup %s: %w", src, err)
	}

	return dst, nil
}

// Dir creates a timestamped backup of a directory by copying it recursively.
func Dir(src string) (string, error) {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return "", nil
	}

	stamp := time.Now().Format("20060102T150405")
	dst := src + ".abstrax-bak." + stamp

	if err := copyDir(src, dst); err != nil {
		return "", fmt.Errorf("backup dir %s: %w", src, err)
	}

	return dst, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target)
	})
}
