package debian_test

import (
	"testing"

	"abstrax/internal/platform/debian"
)

func TestProviderPathsAndNaming(t *testing.T) {
	p := debian.NewProvider(debian.ProviderOptions{})

	if p.WebUser() != "www-data" || p.WebGroup() != "www-data" {
		t.Fatalf("unexpected web user/group: %s/%s", p.WebUser(), p.WebGroup())
	}
	if p.DefaultProjectRoot() != "/var/www" {
		t.Fatalf("DefaultProjectRoot = %q", p.DefaultProjectRoot())
	}
	if p.PHPFPMServiceName("8.5") != "php8.5-fpm" {
		t.Fatalf("PHPFPMServiceName = %q", p.PHPFPMServiceName("8.5"))
	}
	if p.PHPFPMBinary("8.5") != "php-fpm8.5" {
		t.Fatalf("PHPFPMBinary = %q", p.PHPFPMBinary("8.5"))
	}
	if p.PHPFPMPoolDir("8.5") != "/etc/php/8.5/fpm/pool.d" {
		t.Fatalf("PHPFPMPoolDir = %q", p.PHPFPMPoolDir("8.5"))
	}
	if p.PHPFPMDefaultSocket("8.5") != "/run/php/php8.5-fpm.sock" {
		t.Fatalf("PHPFPMDefaultSocket = %q", p.PHPFPMDefaultSocket("8.5"))
	}
	if p.PHPFPMProjectSocket("8.5", "example") != "/run/php/php8.5-fpm-example.sock" {
		t.Fatalf("PHPFPMProjectSocket = %q", p.PHPFPMProjectSocket("8.5", "example"))
	}
	if p.NginxSitesAvailable() != "/etc/nginx/sites-available" {
		t.Fatalf("NginxSitesAvailable = %q", p.NginxSitesAvailable())
	}
	if p.NginxSitesEnabled() != "/etc/nginx/sites-enabled" {
		t.Fatalf("NginxSitesEnabled = %q", p.NginxSitesEnabled())
	}
	if p.NginxConfPath() != "/etc/nginx/nginx.conf" {
		t.Fatalf("NginxConfPath = %q", p.NginxConfPath())
	}
	if p.NginxSitesEnabledInclude() != "include /etc/nginx/sites-enabled/*;" {
		t.Fatalf("NginxSitesEnabledInclude = %q", p.NginxSitesEnabledInclude())
	}
}

func TestPHPPackageNames(t *testing.T) {
	p := debian.NewProvider(debian.ProviderOptions{})
	pkgs := p.PHPPackageNames("8.5", []string{"mysql", "pcntl", "redis"})
	want := []string{"php8.5-fpm", "php8.5-cli", "php8.5-mysql", "php8.5-redis"}
	if len(pkgs) != len(want) {
		t.Fatalf("got %v, want %v", pkgs, want)
	}
	for i := range want {
		if pkgs[i] != want[i] {
			t.Fatalf("package %d = %q, want %q", i, pkgs[i], want[i])
		}
	}
}
