package plugin

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const maxPluginBinarySize = 256 << 20

var gzipMagic = []byte{0x1f, 0x8b}

// resolveInstallBinary returns the executable to install from a verified download.
// Gzip-compressed tar archives are unpacked; a raw binary is used as-is.
// When the returned path differs from downloadPath, the caller must remove it.
func resolveInstallBinary(downloadPath, pluginName string) (string, error) {
	gzip, err := fileHasGzipMagic(downloadPath)
	if err != nil {
		return "", err
	}
	if !gzip {
		return downloadPath, nil
	}

	dest, err := os.CreateTemp("", "abstrax-plugin-bin-*")
	if err != nil {
		return "", err
	}
	destPath := dest.Name()
	if err := dest.Close(); err != nil {
		os.Remove(destPath)
		return "", err
	}

	if err := extractPluginBinary(downloadPath, destPath, pluginName); err != nil {
		os.Remove(destPath)
		return "", err
	}
	return destPath, nil
}

func fileHasGzipMagic(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	var hdr [2]byte
	n, err := io.ReadFull(f, hdr[:])
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return n == 2 && hdr[0] == gzipMagic[0] && hdr[1] == gzipMagic[1], nil
}

func extractPluginBinary(archivePath, destPath, pluginName string) error {
	want := PluginBinaryName(pluginName)

	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("reading plugin archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	found := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading plugin archive: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) != want {
			continue
		}
		if found {
			return fmt.Errorf("plugin archive contains more than one %s", want)
		}
		if hdr.Size < 0 || hdr.Size > maxPluginBinarySize {
			return fmt.Errorf("plugin binary in archive is too large")
		}

		out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		written, copyErr := io.Copy(out, io.LimitReader(tr, hdr.Size))
		closeErr := out.Close()
		if copyErr != nil {
			return fmt.Errorf("extracting %s: %w", want, copyErr)
		}
		if closeErr != nil {
			return closeErr
		}
		if written != hdr.Size {
			return fmt.Errorf("extracting %s: short read", want)
		}
		found = true
	}

	if !found {
		return fmt.Errorf("plugin archive does not contain %s", want)
	}
	return nil
}
