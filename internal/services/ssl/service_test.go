package ssl

import (
	"reflect"
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
		Email:    "admin@example.com",
		Domains:  []string{"example.com", " www.example.com "},
		CertName: "abstrax-example",
		Staging:  true,
	})

	want := []string{
		"certonly",
		"--nginx",
		"--non-interactive",
		"--agree-tos",
		"--email", "admin@example.com",
		"--cert-name", "abstrax-example",
		"--expand",
		"--deploy-hook", "nginx -s reload",
		"-d", "example.com",
		"-d", "www.example.com",
		"--staging",
	}

	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v\nwant   %#v", args, want)
	}
}

func TestBuildCertbotAddArgsNeverInstalls(t *testing.T) {
	args := buildCertbotAddArgs(AddOptions{
		Email:        "admin@example.com",
		Domains:      []string{"example.com"},
		CertName:     "abstrax-example",
		RedirectHTTP: true,
	})
	for _, arg := range args {
		if arg == "--redirect" || arg == "--no-redirect" || arg == "--installer" {
			t.Fatalf("certbot must not edit nginx; got %#v", args)
		}
	}
	if args[0] != "certonly" {
		t.Fatalf("args = %#v", args)
	}
}
