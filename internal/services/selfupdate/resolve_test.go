package selfupdate

import (
	"sort"
	"testing"

	"github.com/Masterminds/semver/v3"
)

func mustVer(t *testing.T, s string) *semver.Version {
	t.Helper()
	v, err := semver.NewVersion(s)
	if err != nil {
		t.Fatalf("semver.NewVersion(%q): %v", s, err)
	}
	return v
}

func publishedVersions(t *testing.T, versions ...string) []*semver.Version {
	t.Helper()
	out := make([]*semver.Version, 0, len(versions))
	for _, v := range versions {
		out = append(out, mustVer(t, v))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LessThan(out[j])
	})
	return out
}

func TestResolveVersionLatestCompatible(t *testing.T) {
	current := mustVer(t, "1.0.1")
	published := publishedVersions(t, "1.0.0", "1.0.2", "1.1.0", "2.0.0")

	out, err := resolveVersion(resolveInput{
		current:   current,
		published: published,
	})
	if err != nil {
		t.Fatalf("resolveVersion: %v", err)
	}
	if out.target == nil || out.target.String() != "1.1.0" {
		t.Fatalf("target = %v, want 1.1.0", out.target)
	}
	if !out.breakingUpdateAvailable || out.breakingVersion.String() != "2.0.0" {
		t.Fatalf("breaking = %v/%v, want true/2.0.0", out.breakingUpdateAvailable, out.breakingVersion)
	}
}

func TestResolveVersionAlreadyUpToDateWithBreakingAvailable(t *testing.T) {
	current := mustVer(t, "1.1.0")
	published := publishedVersions(t, "1.0.0", "1.1.0", "2.0.0")

	out, err := resolveVersion(resolveInput{
		current:   current,
		published: published,
	})
	if err != nil {
		t.Fatalf("resolveVersion: %v", err)
	}
	if !out.alreadyUpToDate {
		t.Fatal("expected already up to date")
	}
	if out.target != nil {
		t.Fatalf("target = %v, want nil", out.target)
	}
	if !out.breakingUpdateAvailable || out.breakingVersion.String() != "2.0.0" {
		t.Fatalf("breaking = %v/%v, want true/2.0.0", out.breakingUpdateAvailable, out.breakingVersion)
	}
}

func TestResolveVersionAllowBreaking(t *testing.T) {
	current := mustVer(t, "1.0.1")
	published := publishedVersions(t, "1.1.0", "2.0.0")

	out, err := resolveVersion(resolveInput{
		current:       current,
		allowBreaking: true,
		published:     published,
	})
	if err != nil {
		t.Fatalf("resolveVersion: %v", err)
	}
	if out.target == nil || out.target.String() != "2.0.0" {
		t.Fatalf("target = %v, want 2.0.0", out.target)
	}
}

func TestResolveVersionExplicitMissing(t *testing.T) {
	current := mustVer(t, "1.0.0")
	published := publishedVersions(t, "1.0.0", "1.1.0")

	_, err := resolveVersion(resolveInput{
		current:   current,
		requested: "9.9.9",
		published: published,
	})
	if err == nil {
		t.Fatal("expected error for missing version")
	}
}

func TestResolveVersionExplicitMajor(t *testing.T) {
	current := mustVer(t, "1.0.1")
	published := publishedVersions(t, "1.1.0", "2.0.0")

	out, err := resolveVersion(resolveInput{
		current:   current,
		requested: "2.0.0",
		published: published,
	})
	if err != nil {
		t.Fatalf("resolveVersion: %v", err)
	}
	if out.target == nil || out.target.String() != "2.0.0" {
		t.Fatalf("target = %v, want 2.0.0", out.target)
	}
}

func TestParseCurrentVersion(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"1.0.1", "1.0.1"},
		{"v1.0.1", "1.0.1"},
		{"dev", "0.0.0"},
		{"v0.1.0-2-g7167f13", "0.1.0"},
	}

	for _, tc := range tests {
		v, err := parseCurrentVersion(tc.in)
		if err != nil {
			t.Fatalf("parseCurrentVersion(%q): %v", tc.in, err)
		}
		if v.String() != tc.want {
			t.Fatalf("parseCurrentVersion(%q) = %s, want %s", tc.in, v, tc.want)
		}
	}
}

func TestReleaseAssetURLs(t *testing.T) {
	archiveURL, checksumsURL, archiveName := releaseAssetURLs("1.2.3", "amd64")
	if archiveName != "abstrax_1.2.3_linux_amd64.tar.gz" {
		t.Fatalf("archiveName = %q", archiveName)
	}
	wantArchive := "https://github.com/useabstrax/abstrax/releases/download/v1.2.3/abstrax_1.2.3_linux_amd64.tar.gz"
	if archiveURL != wantArchive {
		t.Fatalf("archiveURL = %q, want %q", archiveURL, wantArchive)
	}
	wantChecksums := "https://github.com/useabstrax/abstrax/releases/download/v1.2.3/abstrax_1.2.3_checksums.txt"
	if checksumsURL != wantChecksums {
		t.Fatalf("checksumsURL = %q, want %q", checksumsURL, wantChecksums)
	}
}
