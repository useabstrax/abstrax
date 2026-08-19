package plugin

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveInstallBinaryRawPassthrough(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "abstrax-raw")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveInstallBinary(path, "raw")
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("got %q, want original path %q", got, path)
	}
}

func TestResolveInstallBinaryExtractsArchive(t *testing.T) {
	dir := t.TempDir()
	content := []byte("#!/bin/sh\necho ok\n")
	archive := filepath.Join(dir, "plugin.tar.gz")
	writePluginTarGz(t, archive, "abstrax-composer", content)

	got, err := resolveInstallBinary(archive, "composer")
	if err != nil {
		t.Fatal(err)
	}
	if got == archive {
		t.Fatal("expected extracted binary path, got archive path")
	}
	defer os.Remove(got)

	extracted, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(extracted, content) {
		t.Fatalf("extracted content mismatch")
	}
}

func TestResolveInstallBinaryPrefixedArchiveName(t *testing.T) {
	dir := t.TempDir()
	content := []byte("plugin-bytes")
	archive := filepath.Join(dir, "plugin.tar.gz")
	writePluginTarGz(t, archive, "./abstrax-deploy", content)

	got, err := resolveInstallBinary(archive, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(got)
	extracted, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(extracted, content) {
		t.Fatal("extracted content mismatch")
	}
}

func TestResolveInstallBinaryMissingPlugin(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "plugin.tar.gz")
	writePluginTarGz(t, archive, "README.md", []byte("not a plugin"))

	_, err := resolveInstallBinary(archive, "composer")
	if err == nil {
		t.Fatal("expected missing binary error")
	}
	if !strings.Contains(err.Error(), "does not contain abstrax-composer") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInstallBinaryFromArchive(t *testing.T) {
	dir := t.TempDir()
	pluginBin := buildTestPlugin(t, dir, "composer")
	binary, err := os.ReadFile(pluginBin)
	if err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(dir, "abstrax-composer.tar.gz")
	writePluginTarGz(t, archivePath, "abstrax-composer", binary)
	archiveData, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(archiveData)
	checksum := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/plugin.tar.gz" {
			http.NotFound(w, r)
			return
		}
		w.Write(archiveData)
	}))
	defer srv.Close()

	paths := testPaths(dir)
	svc := NewWithPaths(paths, srv.URL)
	result, err := svc.installBinary(context.Background(), installBinaryOpts{
		name:       "composer",
		version:    "0.2.0",
		publisher:  "useabstrax",
		trustLevel: TrustOfficial,
		source:     SourceRegistry,
		binaryURL:  srv.URL + "/plugin.tar.gz",
		sha256:     checksum,
	})
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}
	if result.Version != "0.2.0" {
		t.Fatalf("version %q, want 0.2.0", result.Version)
	}

	installed, err := os.ReadFile(result.BinaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(installed, binary) {
		t.Fatal("installed file is not the extracted plugin binary")
	}
}

func writePluginTarGz(t *testing.T, dest, name string, content []byte) {
	t.Helper()
	f, err := os.Create(dest)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{
		Name:     name,
		Mode:     0755,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
