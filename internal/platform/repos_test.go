package platform_test

import (
	"context"
	"strings"
	"testing"

	"abstrax/internal/platform"
)

func rhelProviderFor(t *testing.T, distroID, distroName string, level platform.SupportLevel) platform.Provider {
	t.Helper()
	info := &platform.Info{
		Family: "rhel",
		Profile: platform.Profile{
			Family:       "rhel",
			DistroID:     distroID,
			DistroName:   distroName,
			SupportLevel: level,
			WebUser:      "nginx",
			WebGroup:     "nginx",
		},
	}
	provider, err := platform.For(info)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func TestSupportsAutomaticRepoEnable(t *testing.T) {
	rocky := rhelProviderFor(t, "rocky", "Rocky Linux 9.5", platform.SupportOfficial)
	if !platform.SupportsAutomaticRepoEnable(rocky, platform.RepoEPEL) {
		t.Fatal("rocky should auto-enable EPEL")
	}
	if platform.SupportsAutomaticRepoEnable(rocky, platform.RepoRemi) {
		t.Fatal("remi should never auto-enable without flag")
	}

	rhel := rhelProviderFor(t, "rhel", "Red Hat Enterprise Linux 9.5", platform.SupportCompatible)
	if platform.SupportsAutomaticRepoEnable(rhel, platform.RepoEPEL) {
		t.Fatal("rhel should not auto-enable EPEL")
	}
}

func TestEnsureRepositoryEPELRocky(t *testing.T) {
	provider := rhelProviderFor(t, "rocky", "Rocky Linux 9.5", platform.SupportOfficial)
	var installed []string
	err := platform.EnsureRepository(context.Background(), provider, platform.RepoEPEL, platform.RepoOptions{}, platform.RepoEnabler{
		Install: func(ctx context.Context, name string) error {
			installed = append(installed, name)
			return nil
		},
		Run: func(ctx context.Context, name string, args ...string) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(installed) != 1 || installed[0] != "epel-release" {
		t.Fatalf("installed = %#v", installed)
	}
}

func TestEnsureRepositoryEPELRHELRequiresFlag(t *testing.T) {
	provider := rhelProviderFor(t, "rhel", "Red Hat Enterprise Linux 9.5", platform.SupportCompatible)
	err := platform.EnsureRepository(context.Background(), provider, platform.RepoEPEL, platform.RepoOptions{}, platform.RepoEnabler{
		Install: func(ctx context.Context, name string) error { return nil },
		Run:     func(ctx context.Context, name string, args ...string) error { return nil },
	})
	if err == nil {
		t.Fatal("expected error without --enable-required-repos")
	}
	if !strings.Contains(err.Error(), "repo enable epel") && !strings.Contains(err.Error(), "enable-required-repos") {
		t.Fatalf("error = %v", err)
	}

	var installed []string
	err = platform.EnsureRepository(context.Background(), provider, platform.RepoEPEL, platform.RepoOptions{EnableRequiredRepos: true}, platform.RepoEnabler{
		Install: func(ctx context.Context, name string) error {
			installed = append(installed, name)
			return nil
		},
		Run: func(ctx context.Context, name string, args ...string) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(installed) != 1 || installed[0] != "epel-release" {
		t.Fatalf("installed = %#v", installed)
	}
}

func TestEnsureRepositoryEPELOracleRequiresFlag(t *testing.T) {
	provider := rhelProviderFor(t, "ol", "Oracle Linux 9.5", platform.SupportCompatible)
	err := platform.EnsureRepository(context.Background(), provider, platform.RepoEPEL, platform.RepoOptions{}, platform.RepoEnabler{
		Install: func(ctx context.Context, name string) error { return nil },
		Run:     func(ctx context.Context, name string, args ...string) error { return nil },
	})
	if err == nil {
		t.Fatal("expected error for Oracle without flag")
	}
}

func TestEnsureRemiRequiresFlagOnRocky(t *testing.T) {
	provider := rhelProviderFor(t, "rocky", "Rocky Linux 9.5", platform.SupportOfficial)
	err := platform.EnsureRepository(context.Background(), provider, platform.RepoRemi, platform.RepoOptions{}, platform.RepoEnabler{
		Install: func(ctx context.Context, name string) error { return nil },
		Run:     func(ctx context.Context, name string, args ...string) error { return nil },
	})
	if err == nil {
		t.Fatal("expected Remi to require --enable-required-repos")
	}
	if !strings.Contains(err.Error(), "Remi") {
		t.Fatalf("error = %v", err)
	}

	var installed []string
	err = platform.EnsureRepository(context.Background(), provider, platform.RepoRemi, platform.RepoOptions{EnableRequiredRepos: true}, platform.RepoEnabler{
		Install: func(ctx context.Context, name string) error {
			installed = append(installed, name)
			return nil
		},
		Run: func(ctx context.Context, name string, args ...string) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(installed) < 2 {
		t.Fatalf("expected epel-release then remi; got %#v", installed)
	}
	if installed[0] != "epel-release" {
		t.Fatalf("first install should be epel-release, got %#v", installed)
	}
}

func TestEnsurePHPRepositoryDebianNoop(t *testing.T) {
	err := platform.EnsurePHPRepository(context.Background(), platform.DebianDefaults(), "8.3", platform.RepoOptions{}, platform.RepoEnabler{
		Install: func(ctx context.Context, name string) error {
			t.Fatal("should not install repos on debian")
			return nil
		},
		Run: func(ctx context.Context, name string, args ...string) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPHPProviderParity(t *testing.T) {
	debian := platform.DebianDefaults()
	if !debian.SupportsMultiplePHPVersions() {
		t.Fatal("debian should support multiple PHP versions")
	}
	if debian.RequiresExternalRepoForPHP("8.3") {
		t.Fatal("debian should not require Remi")
	}
	if debian.PHPFPMServiceName("8.3") != "php8.3-fpm" {
		t.Fatalf("debian service = %q", debian.PHPFPMServiceName("8.3"))
	}
	if debian.PHPCLIBinary("8.3") != "php8.3" {
		t.Fatalf("debian cli = %q", debian.PHPCLIBinary("8.3"))
	}

	rhel := platform.RHELDefaults()
	if !rhel.SupportsMultiplePHPVersions() {
		t.Fatal("rhel should support multiple PHP versions via Remi")
	}
	if !rhel.RequiresExternalRepoForPHP("8.3") {
		t.Fatal("rhel should require Remi for PHP")
	}
	if err := rhel.ValidatePHPVersion("8.3"); err != nil {
		t.Fatal(err)
	}
	if err := rhel.ValidatePHPVersion("5.6"); err == nil {
		t.Fatal("expected unsupported version")
	}
	if rhel.PHPFPMServiceName("8.3") != "php83-php-fpm" {
		t.Fatalf("rhel service = %q", rhel.PHPFPMServiceName("8.3"))
	}
	if rhel.PHPCLIBinary("8.3") != "/opt/remi/php83/root/usr/bin/php" {
		t.Fatalf("rhel cli = %q", rhel.PHPCLIBinary("8.3"))
	}
	pkgs := rhel.PHPPackageNames("8.3", []string{"mysql"})
	if len(pkgs) < 3 || pkgs[0] != "php83-php-fpm" || pkgs[2] != "php83-php-mysqlnd" {
		t.Fatalf("rhel packages = %#v", pkgs)
	}
}

func TestNormalizePHPVersion(t *testing.T) {
	v, err := platform.NormalizePHPVersion("php8.4")
	if err != nil || v != "8.4" {
		t.Fatalf("got %q %v", v, err)
	}
	if _, err := platform.NormalizePHPVersion("latest"); err == nil {
		t.Fatal("expected invalid version")
	}
}
