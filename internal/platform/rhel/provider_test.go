package rhel_test

import (
	"strings"
	"testing"

	"abstrax/internal/platform/rhel"
)

func TestProviderPathsAndNaming(t *testing.T) {
	p := rhel.NewProvider(rhel.ProviderOptions{})

	if p.WebUser() != "nginx" || p.WebGroup() != "nginx" {
		t.Fatalf("unexpected web user/group: %s/%s", p.WebUser(), p.WebGroup())
	}
	if p.DefaultProjectRoot() != "/var/www" {
		t.Fatalf("DefaultProjectRoot = %q", p.DefaultProjectRoot())
	}
	if !p.SupportsMultiplePHPVersions() {
		t.Fatal("RHEL provider should support multiple PHP versions via Remi")
	}
	if !p.RequiresExternalRepoForPHP("8.2") {
		t.Fatal("Remi PHP should require external repo")
	}
	if err := p.ValidatePHPVersion("8.2"); err != nil {
		t.Fatal(err)
	}
	if err := p.ValidatePHPVersion("7.4"); err == nil {
		t.Fatal("expected unsupported PHP version error")
	}
	if p.PHPFPMServiceName("8.2") != "php82-php-fpm" {
		t.Fatalf("PHPFPMServiceName = %q", p.PHPFPMServiceName("8.2"))
	}
	if p.PHPFPMBinary("8.2") != "/opt/remi/php82/root/usr/sbin/php-fpm" {
		t.Fatalf("PHPFPMBinary = %q", p.PHPFPMBinary("8.2"))
	}
	if p.PHPCLIBinary("8.2") != "/opt/remi/php82/root/usr/bin/php" {
		t.Fatalf("PHPCLIBinary = %q", p.PHPCLIBinary("8.2"))
	}
	if p.PHPFPMPoolDir("8.2") != "/etc/opt/remi/php82/php-fpm.d" {
		t.Fatalf("PHPFPMPoolDir = %q", p.PHPFPMPoolDir("8.2"))
	}
	if p.PHPFPMDefaultPoolConfig("8.2") != "/etc/opt/remi/php82/php-fpm.d/www.conf" {
		t.Fatalf("PHPFPMDefaultPoolConfig = %q", p.PHPFPMDefaultPoolConfig("8.2"))
	}
	if p.PHPFPMDefaultSocket("8.2") != "/var/opt/remi/php82/run/php-fpm/www.sock" {
		t.Fatalf("PHPFPMDefaultSocket = %q", p.PHPFPMDefaultSocket("8.2"))
	}
	if p.PHPFPMProjectSocket("8.2", "example") != "/var/opt/remi/php82/run/php-fpm/example.sock" {
		t.Fatalf("PHPFPMProjectSocket = %q", p.PHPFPMProjectSocket("8.2", "example"))
	}
	if p.NginxConfigDir() != "/etc/nginx/conf.d" {
		t.Fatalf("NginxConfigDir = %q", p.NginxConfigDir())
	}
	if p.NginxSiteConfigPath("mysite") != "/etc/nginx/conf.d/mysite.conf" {
		t.Fatalf("NginxSiteConfigPath = %q", p.NginxSiteConfigPath("mysite"))
	}
	if p.NginxConfPath() != "/etc/nginx/nginx.conf" {
		t.Fatalf("NginxConfPath = %q", p.NginxConfPath())
	}
	if p.NginxConfDInclude() != "include /etc/nginx/conf.d/*.conf;" {
		t.Fatalf("NginxConfDInclude = %q", p.NginxConfDInclude())
	}
	if p.SudoGroup() != "wheel" {
		t.Fatalf("SudoGroup = %q", p.SudoGroup())
	}
}

func TestPHPPackageNames(t *testing.T) {
	p := rhel.NewProvider(rhel.ProviderOptions{})
	pkgs := p.PHPPackageNames("8.2", []string{"mysql", "pcntl", "redis"})
	want := []string{"php82-php-fpm", "php82-php-cli", "php82-php-mysqlnd", "php82-php-redis"}
	if len(pkgs) != len(want) {
		t.Fatalf("got %v, want %v", pkgs, want)
	}
	for i := range want {
		if pkgs[i] != want[i] {
			t.Fatalf("package %d = %q, want %q", i, pkgs[i], want[i])
		}
	}
}

func TestPHPPackageNamesAllSupportedVersions(t *testing.T) {
	p := rhel.NewProvider(rhel.ProviderOptions{})
	for _, v := range rhel.SupportedRemiPHPVersions {
		pkgs := p.PHPPackageNames(v, nil)
		if len(pkgs) < 2 {
			t.Fatalf("version %s: expected fpm+cli packages", v)
		}
		if !strings.Contains(pkgs[0], "-php-fpm") {
			t.Fatalf("version %s: unexpected fpm package %q", v, pkgs[0])
		}
		svc := p.PHPFPMServiceName(v)
		if svc != pkgs[0] {
			t.Fatalf("version %s: service %q should match package %q", v, svc, pkgs[0])
		}
	}
}
