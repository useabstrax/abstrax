package ssl

import (
	"testing"
)

func TestCertbotInstallPackages(t *testing.T) {
	pkgs := certbotInstallPackages()
	if len(pkgs) != 2 {
		t.Fatalf("len(pkgs) = %d, want 2: %#v", len(pkgs), pkgs)
	}
	if pkgs[0] != certbotPackage || pkgs[1] != certbotNginxPackage {
		t.Fatalf("unexpected packages: %#v", pkgs)
	}
}

func TestInstallCommand(t *testing.T) {
	if InstallCommand() != "abstrax ssl install" {
		t.Fatalf("InstallCommand() = %q", InstallCommand())
	}
}

func TestBuildCertbotAddArgs(t *testing.T) {
	args := buildCertbotAddArgs(AddOptions{
		Email:        "admin@example.com",
		Domains:      []string{"example.com", " www.example.com "},
		RedirectHTTP: true,
		Staging:      true,
	})

	want := []string{
		"--nginx",
		"--non-interactive",
		"--agree-tos",
		"--email", "admin@example.com",
		"-d", "example.com",
		"-d", "www.example.com",
		"--staging",
		"--redirect",
	}

	if len(args) != len(want) {
		t.Fatalf("len(args) = %d, want %d: %#v", len(args), len(want), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q (full args: %#v)", i, args[i], want[i], args)
		}
	}

	for _, arg := range args {
		if arg == "certonly" {
			t.Fatalf("certbot add must not use certonly subcommand, got %#v", args)
		}
	}
}

func TestBuildCertbotAddArgsNoRedirect(t *testing.T) {
	args := buildCertbotAddArgs(AddOptions{
		Email:        "admin@example.com",
		Domains:      []string{"example.com"},
		RedirectHTTP: false,
	})

	if args[len(args)-1] != "--no-redirect" {
		t.Fatalf("expected --no-redirect, got %#v", args)
	}
}
