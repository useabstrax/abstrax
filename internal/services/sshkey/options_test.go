package sshkey

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestKeyRestrictionsFormats(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	svc := &Service{
		now:        func() time.Time { return now },
		sshVersion: func() (string, error) { return "OpenSSH_9.6p1 Ubuntu-3", nil },
	}

	got, err := svc.keyRestrictions(AddOptions{
		Expires: "2026-11-07T23:59:00Z",
		From:    "140.82.112.0/20,143.55.64.0/20",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `expiry-time="202611072359Z",from="140.82.112.0/20,143.55.64.0/20"`
	if got.prefix != want {
		t.Fatalf("prefix = %q", got.prefix)
	}

	expiresOnly, err := svc.keyRestrictions(AddOptions{Expires: "2026-11-07T23:59:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	if expiresOnly.prefix != `expiry-time="202611072359Z"` {
		t.Fatalf("expires prefix = %q", expiresOnly.prefix)
	}

	fromOnly, err := svc.keyRestrictions(AddOptions{From: "203.0.113.5"})
	if err != nil {
		t.Fatal(err)
	}
	if fromOnly.prefix != `from="203.0.113.5"` {
		t.Fatalf("from prefix = %q", fromOnly.prefix)
	}

	plain, err := svc.keyRestrictions(AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if plain.prefix != "" {
		t.Fatalf("plain prefix = %q", plain.prefix)
	}
}

func TestKeyRestrictionsValidation(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	svc := &Service{
		now:        func() time.Time { return now },
		sshVersion: func() (string, error) { return "OpenSSH_9.6p1", nil },
	}

	if _, err := svc.keyRestrictions(AddOptions{Expires: "2020-01-01T00:00:00Z"}); err == nil {
		t.Fatal("expected past expiry to fail")
	}
	if _, err := svc.keyRestrictions(AddOptions{Expires: "next week"}); err == nil {
		t.Fatal("expected bad time to fail")
	}
	if _, err := svc.keyRestrictions(AddOptions{From: "not-a-cidr"}); err == nil {
		t.Fatal("expected bad CIDR to fail")
	}
	if _, err := svc.keyRestrictions(AddOptions{From: `10.0.0.0/8"`}); err == nil {
		t.Fatal("expected a quote in --from to fail")
	}

	old := &Service{
		now:        func() time.Time { return now },
		sshVersion: func() (string, error) { return "OpenSSH_7.9p1 Debian", nil },
	}
	_, err := old.keyRestrictions(AddOptions{Expires: "2026-11-07T23:59:00Z"})
	if err == nil || !strings.Contains(err.Error(), "OpenSSH 8.2") {
		t.Fatalf("old openssh error = %v", err)
	}
	if _, err := old.keyRestrictions(AddOptions{From: "203.0.113.5"}); err != nil {
		t.Fatalf("from on old openssh: %v", err)
	}
}

func TestParseKeyLineOptions(t *testing.T) {
	line := `expiry-time="202611072359Z",from="140.82.112.0/20,143.55.64.0/20" ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAItest deploy`
	parsed, ok := splitAuthorizedKeyLine(line)
	if !ok {
		t.Fatal("expected a key line")
	}
	if parsed.keyType != "ssh-ed25519" || parsed.comment != "deploy" {
		t.Fatalf("parsed = %+v", parsed)
	}
	if parsed.expiresAt == nil || *parsed.expiresAt != "2026-11-07T23:59:00Z" {
		t.Fatalf("expires = %v", parsed.expiresAt)
	}
	if strings.Join(parsed.from, ",") != "140.82.112.0/20,143.55.64.0/20" {
		t.Fatalf("from = %v", parsed.from)
	}

	plain, ok := splitAuthorizedKeyLine("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAItest comment here")
	if !ok || plain.expiresAt != nil || plain.from != nil || plain.comment != "comment here" {
		t.Fatalf("plain = %+v ok=%v", plain, ok)
	}
}

func TestListParsesLegacyAndRestrictedKeys(t *testing.T) {
	dir := t.TempDir()
	auth := filepath.Join(dir, ".ssh", "authorized_keys")
	if err := os.MkdirAll(filepath.Dir(auth), 0o700); err != nil {
		t.Fatal(err)
	}
	body := "" +
		"# abstrax:key id=legacy name=\"legacy\"\n" +
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAItest legacy\n" +
		"# abstrax:key id=temp name=\"temp\"\n" +
		"expiry-time=\"202001010000Z\" ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAItest2 temp\n"
	if err := os.WriteFile(auth, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	keys := parseAuthorizedKeys(strings.NewReader(body), false, now)
	if len(keys) != 2 {
		t.Fatalf("keys = %+v", keys)
	}
	if keys[0].ID != "legacy" || keys[0].ExpiresAt != nil || keys[0].Expired {
		t.Fatalf("legacy = %+v", keys[0])
	}
	if keys[1].ID != "temp" || keys[1].ExpiresAt == nil || *keys[1].ExpiresAt != "2020-01-01T00:00:00Z" || !keys[1].Expired {
		t.Fatalf("temp = %+v", keys[1])
	}
}

func TestOpensshAtLeast(t *testing.T) {
	if !opensshAtLeast("OpenSSH_8.2p1", 8, 2) {
		t.Fatal("8.2 should pass")
	}
	if opensshAtLeast("OpenSSH_8.1p1", 8, 2) {
		t.Fatal("8.1 should fail")
	}
	if !opensshAtLeast("OpenSSH_9.6p1 Ubuntu-3ubuntu13.5", 8, 2) {
		t.Fatal("9.6 should pass")
	}
}
