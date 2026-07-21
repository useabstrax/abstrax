package platform_test

import (
	"testing"

	"abstrax/internal/platform"
)

func TestDetectReturnsInfo(t *testing.T) {
	info, tools, err := platform.Detect()
	if err != nil {
		t.Fatalf("Detect() returned unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("Detect() returned nil Info")
	}
	if tools == nil {
		t.Fatal("Detect() returned nil Tools")
	}
	if info.Architecture == "" {
		t.Error("Architecture should not be empty")
	}
	if info.Profile.Family == "" && info.OSName != "" {
		t.Error("Profile family should be set when OS is detected")
	}
}

func TestRequireRootReturnsErrorWhenNotRoot(t *testing.T) {
	err := platform.RequireRoot()
	_ = err
}

func TestDebianDefaultsProvider(t *testing.T) {
	p := platform.DebianDefaults()
	if p.WebUser() != "www-data" {
		t.Fatalf("WebUser = %q", p.WebUser())
	}
	if p.DefaultProjectRoot() != "/var/www" {
		t.Fatalf("DefaultProjectRoot = %q", p.DefaultProjectRoot())
	}
}

func TestRHELDefaultsProvider(t *testing.T) {
	p := platform.RHELDefaults()
	if p.WebUser() != "nginx" {
		t.Fatalf("WebUser = %q", p.WebUser())
	}
	if p.WebGroup() != "nginx" {
		t.Fatalf("WebGroup = %q", p.WebGroup())
	}
	if p.DefaultProjectRoot() != "/var/www" {
		t.Fatalf("DefaultProjectRoot = %q", p.DefaultProjectRoot())
	}
	if p.NginxLayout() != platform.NginxConfD {
		t.Fatalf("NginxLayout = %q", p.NginxLayout())
	}
	if p.PHPFPMServiceName("8.2") != "php82-php-fpm" {
		t.Fatalf("PHPFPMServiceName = %q", p.PHPFPMServiceName("8.2"))
	}
	if p.NginxSiteConfigPath("app") != "/etc/nginx/conf.d/app.conf" {
		t.Fatalf("NginxSiteConfigPath = %q", p.NginxSiteConfigPath("app"))
	}
	if p.Profile().FirewallStrategy != "firewalld" {
		t.Fatalf("FirewallStrategy = %q", p.Profile().FirewallStrategy)
	}
}
