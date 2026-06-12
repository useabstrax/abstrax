package selfupdate

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func releaseArch() (string, error) {
	switch runtime.GOARCH {
	case "amd64":
		return "amd64", nil
	case "arm64":
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported architecture %q; Abstrax supports amd64 and arm64", runtime.GOARCH)
	}
}

func releaseAssetURLs(version, arch string) (archiveURL, checksumsURL, archiveName string) {
	v := normalizeVersion(version)
	archiveName = fmt.Sprintf("%s_%s_linux_%s.tar.gz", binaryName, v, arch)
	checksumsName := fmt.Sprintf("%s_%s_checksums.txt", binaryName, v)
	base := fmt.Sprintf("https://github.com/%s/%s/releases/download/v%s", githubOwner, githubRepo, v)
	return base + "/" + archiveName, base + "/" + checksumsName, archiveName
}

func downloadFile(ctx context.Context, client *http.Client, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "abstrax-cli")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("download failed (%d) for %s: %s", resp.StatusCode, url, strings.TrimSpace(string(body)))
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("writing %s: %w", dest, err)
	}
	return nil
}

func verifyChecksum(checksumsPath, archiveName, archivePath string) error {
	f, err := os.Open(checksumsPath)
	if err != nil {
		return err
	}
	defer f.Close()

	var expected string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasSuffix(line, " "+archiveName) {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				expected = fields[0]
				break
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if expected == "" {
		return fmt.Errorf("checksum for %s not found in checksums file", archiveName)
	}

	data, err := os.ReadFile(archivePath)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	actual := hex.EncodeToString(sum[:])
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("checksum mismatch for %s", archiveName)
	}
	return nil
}

func extractBinary(archivePath, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("reading gzip archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar archive: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg || filepath.Base(hdr.Name) != binaryName {
			continue
		}

		out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		return nil
	}

	return fmt.Errorf("binary %q not found in archive", binaryName)
}

func currentExecutable() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(path)
}

func installBinary(ctx context.Context, version string, dryRun, verbose bool) (installPath string, err error) {
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("self-update is only supported on Linux")
	}

	arch, err := releaseArch()
	if err != nil {
		return "", err
	}

	installPath, err = currentExecutable()
	if err != nil {
		return "", fmt.Errorf("locating current binary: %w", err)
	}

	archiveURL, checksumsURL, archiveName := releaseAssetURLs(version, arch)
	if verbose {
		fmt.Printf("[verbose] archive: %s\n", archiveURL)
		fmt.Printf("[verbose] checksums: %s\n", checksumsURL)
		fmt.Printf("[verbose] install path: %s\n", installPath)
	}

	if dryRun {
		return installPath, nil
	}

	tmpDir, err := os.MkdirTemp("", "abstrax-update-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	client := &http.Client{Timeout: 2 * time.Minute}
	archivePath := filepath.Join(tmpDir, archiveName)
	checksumsPath := filepath.Join(tmpDir, "checksums.txt")
	newBinaryPath := filepath.Join(tmpDir, binaryName)

	if err := downloadFile(ctx, client, archiveURL, archivePath); err != nil {
		return "", err
	}
	if err := downloadFile(ctx, client, checksumsURL, checksumsPath); err != nil {
		return "", err
	}
	if err := verifyChecksum(checksumsPath, archiveName, archivePath); err != nil {
		return "", err
	}
	if err := extractBinary(archivePath, newBinaryPath); err != nil {
		return "", err
	}

	if err := replaceExecutable(newBinaryPath, installPath); err != nil {
		return "", err
	}

	return installPath, nil
}

func replaceExecutable(src, dest string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	mode := srcInfo.Mode().Perm() | 0o111
	if err := os.Chmod(src, mode); err != nil {
		return err
	}

	// Same-filesystem rename is atomic and works even when dest is running.
	if err := os.Rename(src, dest); err == nil {
		return nil
	}

	// Cross-filesystem move: write a temp file next to dest, then rename over it.
	// Writing directly to dest fails with ETXTBSY when dest is the running binary.
	destDir := filepath.Dir(dest)
	tmpDest := filepath.Join(destDir, fmt.Sprintf(".%s.new.%d", filepath.Base(dest), os.Getpid()))

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(tmpDest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("writing updated binary to %s: %w", tmpDest, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmpDest)
		return fmt.Errorf("writing updated binary to %s: %w", tmpDest, err)
	}
	if err := out.Close(); err != nil {
		os.Remove(tmpDest)
		return fmt.Errorf("writing updated binary to %s: %w", tmpDest, err)
	}

	if err := os.Rename(tmpDest, dest); err != nil {
		os.Remove(tmpDest)
		return fmt.Errorf("installing updated binary to %s: %w", dest, err)
	}
	return nil
}
