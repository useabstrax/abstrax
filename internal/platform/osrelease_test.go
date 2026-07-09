package platform_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"abstrax/internal/platform"
)

func TestProfileFromOSReleaseFixtures(t *testing.T) {
	tests := []struct {
		fixture     string
		wantID      string
		wantFamily  string
		wantLevel   platform.SupportLevel
		wantWebUser string
		wantRoot    string
		wantNginx   platform.NginxLayout
		wantPHPFPM  string
	}{
		{"ubuntu-20.04.os-release", "ubuntu", "debian", platform.SupportOfficial, "www-data", "/var/www", platform.NginxSitesAvailableEnabled, "php{version}-fpm"},
		{"ubuntu-22.04.os-release", "ubuntu", "debian", platform.SupportOfficial, "www-data", "/var/www", platform.NginxSitesAvailableEnabled, "php{version}-fpm"},
		{"ubuntu-24.04.os-release", "ubuntu", "debian", platform.SupportOfficial, "www-data", "/var/www", platform.NginxSitesAvailableEnabled, "php{version}-fpm"},
		{"debian-11.os-release", "debian", "debian", platform.SupportOfficial, "www-data", "/var/www", platform.NginxSitesAvailableEnabled, "php{version}-fpm"},
		{"debian-12.os-release", "debian", "debian", platform.SupportOfficial, "www-data", "/var/www", platform.NginxSitesAvailableEnabled, "php{version}-fpm"},
		{"debian-13.os-release", "debian", "debian", platform.SupportOfficial, "www-data", "/var/www", platform.NginxSitesAvailableEnabled, "php{version}-fpm"},
		{"linuxmint.os-release", "linuxmint", "debian", platform.SupportOfficial, "www-data", "/var/www", platform.NginxSitesAvailableEnabled, "php{version}-fpm"},
		{"pop.os-release", "pop", "debian", platform.SupportOfficial, "www-data", "/var/www", platform.NginxSitesAvailableEnabled, "php{version}-fpm"},
		{"raspbian.os-release", "raspbian", "debian", platform.SupportOfficial, "www-data", "/var/www", platform.NginxSitesAvailableEnabled, "php{version}-fpm"},
		{"raspberrypi.os-release", "raspberrypi", "debian", platform.SupportOfficial, "www-data", "/var/www", platform.NginxSitesAvailableEnabled, "php{version}-fpm"},
		{"rocky-9.os-release", "rocky", "rhel", platform.SupportOfficial, "nginx", "/var/www", platform.NginxConfD, "remi-php{mm}-php-fpm"},
		{"rocky-10.os-release", "rocky", "rhel", platform.SupportOfficial, "nginx", "/var/www", platform.NginxConfD, "remi-php{mm}-php-fpm"},
		{"almalinux-9.os-release", "almalinux", "rhel", platform.SupportOfficial, "nginx", "/var/www", platform.NginxConfD, "remi-php{mm}-php-fpm"},
		{"almalinux-10.os-release", "almalinux", "rhel", platform.SupportOfficial, "nginx", "/var/www", platform.NginxConfD, "remi-php{mm}-php-fpm"},
		{"rhel-9.os-release", "rhel", "rhel", platform.SupportCompatible, "nginx", "/var/www", platform.NginxConfD, "remi-php{mm}-php-fpm"},
		{"rhel-10.os-release", "rhel", "rhel", platform.SupportCompatible, "nginx", "/var/www", platform.NginxConfD, "remi-php{mm}-php-fpm"},
		{"centos-stream-9.os-release", "centos", "rhel", platform.SupportCompatible, "nginx", "/var/www", platform.NginxConfD, "remi-php{mm}-php-fpm"},
		{"oracle-linux-9.os-release", "ol", "rhel", platform.SupportCompatible, "nginx", "/var/www", platform.NginxConfD, "remi-php{mm}-php-fpm"},
		{"rocky-8.os-release", "rocky", "rhel", platform.SupportUnsupported, "nginx", "/var/www", platform.NginxConfD, "remi-php{mm}-php-fpm"},
		{"fedora-40.os-release", "fedora", "unknown", platform.SupportUnsupported, "", "", platform.NginxLayoutUnknown, "unknown"},
		{"alpine-3.os-release", "alpine", "unknown", platform.SupportUnsupported, "", "", platform.NginxLayoutUnknown, "unknown"},
		{"malformed.os-release", "", "unknown", platform.SupportUnsupported, "", "", platform.NginxLayoutUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join("testdata", tt.fixture))
			if err != nil {
				t.Fatalf("reading fixture: %v", err)
			}

			profile := platform.ProfileFromOSRelease(string(content))

			if profile.DistroID != tt.wantID {
				t.Errorf("DistroID = %q, want %q", profile.DistroID, tt.wantID)
			}
			if profile.Family != tt.wantFamily {
				t.Errorf("Family = %q, want %q", profile.Family, tt.wantFamily)
			}
			if profile.SupportLevel != tt.wantLevel {
				t.Errorf("SupportLevel = %q, want %q", profile.SupportLevel, tt.wantLevel)
			}
			if profile.WebUser != tt.wantWebUser {
				t.Errorf("WebUser = %q, want %q", profile.WebUser, tt.wantWebUser)
			}
			if profile.DefaultProjectRoot != tt.wantRoot {
				t.Errorf("DefaultProjectRoot = %q, want %q", profile.DefaultProjectRoot, tt.wantRoot)
			}
			if profile.NginxLayout != tt.wantNginx {
				t.Errorf("NginxLayout = %q, want %q", profile.NginxLayout, tt.wantNginx)
			}
			if profile.PHPFPMStrategy != tt.wantPHPFPM {
				t.Errorf("PHPFPMStrategy = %q, want %q", profile.PHPFPMStrategy, tt.wantPHPFPM)
			}
			if tt.wantFamily == "rhel" {
				if profile.NginxConfigDir != "/etc/nginx/conf.d" {
					t.Errorf("NginxConfigDir = %q", profile.NginxConfigDir)
				}
				if profile.FirewallStrategy != "firewalld" {
					t.Errorf("FirewallStrategy = %q", profile.FirewallStrategy)
				}
				if profile.PackageManager != "dnf" {
					t.Errorf("PackageManager = %q", profile.PackageManager)
				}
			}
		})
	}
}

func TestCompatibleDebianFamilyDistros(t *testing.T) {
	profile := platform.ProfileFromOSRelease(`PRETTY_NAME="Ubuntu 18.04.6 LTS"
ID=ubuntu
ID_LIKE=debian
VERSION_ID="18.04"`)

	if profile.SupportLevel != platform.SupportCompatible {
		t.Fatalf("SupportLevel = %q, want compatible", profile.SupportLevel)
	}
	if !profile.Supported() {
		t.Fatal("compatible Ubuntu 18.04 should allow mutating commands")
	}
}

func TestRHELExperimentalAllowsMutatingCommands(t *testing.T) {
	profile := platform.ProfileFromOSRelease(`PRETTY_NAME="Red Hat Enterprise Linux 9.5 (Plow)"
ID=rhel
ID_LIKE=fedora
VERSION_ID="9.5"`)

	if profile.SupportLevel != platform.SupportCompatible {
		t.Fatalf("SupportLevel = %q, want compatible", profile.SupportLevel)
	}
	if !profile.Supported() {
		t.Fatal("experimental RHEL 9 should allow mutating commands")
	}
	if !strings.Contains(profile.SupportNote, "experimental") {
		t.Fatalf("SupportNote should mention experimental: %q", profile.SupportNote)
	}
}

func TestRequireSupportedBlocksUnsupported(t *testing.T) {
	info := &platform.Info{
		Supported: false,
		Profile: platform.Profile{
			DistroName:   "Fedora Linux 40",
			DistroID:     "fedora",
			Family:       "unknown",
			SupportLevel: platform.SupportUnsupported,
		},
	}

	err := platform.RequireSupported(info)
	if err == nil {
		t.Fatal("expected unsupported error")
	}
	if _, ok := err.(*platform.UnsupportedError); !ok {
		t.Fatalf("expected *UnsupportedError, got %T", err)
	}
}

func TestRequireSupportedBlocksOldRocky(t *testing.T) {
	profile := platform.ProfileFromOSRelease(`NAME="Rocky Linux"
ID="rocky"
VERSION_ID="8.10"
PRETTY_NAME="Rocky Linux 8.10 (Green Obsidian)"`)
	info := &platform.Info{
		Supported: profile.Supported(),
		Profile:   profile,
		Family:    profile.Family,
	}
	if err := platform.RequireSupported(info); err == nil {
		t.Fatal("expected Rocky 8 to be unsupported")
	}
}

func TestRequireSupportedAllowsCompatible(t *testing.T) {
	info := &platform.Info{
		Supported: true,
		Profile: platform.Profile{
			DistroID:     "ubuntu",
			SupportLevel: platform.SupportCompatible,
		},
	}

	if err := platform.RequireSupported(info); err != nil {
		t.Fatalf("compatible platform should be allowed: %v", err)
	}
}

func TestForReturnsDebianProvider(t *testing.T) {
	info := &platform.Info{
		Family: "debian",
		Profile: platform.Profile{
			Family:             "debian",
			WebUser:            "www-data",
			DefaultProjectRoot: "/var/www",
		},
	}

	provider, err := platform.For(info)
	if err != nil {
		t.Fatalf("For() error: %v", err)
	}
	if provider.WebUser() != "www-data" {
		t.Fatalf("WebUser = %q", provider.WebUser())
	}
	if provider.PHPFPMServiceName("8.5") != "php8.5-fpm" {
		t.Fatalf("PHPFPMServiceName = %q", provider.PHPFPMServiceName("8.5"))
	}
}

func TestForReturnsRHELProvider(t *testing.T) {
	info := &platform.Info{
		Family: "rhel",
		Profile: platform.Profile{
			Family:             "rhel",
			DistroID:           "rocky",
			VersionID:          "9.5",
			WebUser:            "nginx",
			WebGroup:           "nginx",
			DefaultProjectRoot: "/var/www",
			SupportLevel:       platform.SupportOfficial,
			NginxLayout:        platform.NginxConfD,
		},
	}

	provider, err := platform.For(info)
	if err != nil {
		t.Fatalf("For() error: %v", err)
	}
	if provider.WebUser() != "nginx" {
		t.Fatalf("WebUser = %q", provider.WebUser())
	}
	if provider.WebGroup() != "nginx" {
		t.Fatalf("WebGroup = %q", provider.WebGroup())
	}
	if provider.PHPFPMServiceName("8.2") != "php82-php-fpm" {
		t.Fatalf("PHPFPMServiceName = %q", provider.PHPFPMServiceName("8.2"))
	}
	if provider.PHPFPMPoolDir("8.2") != "/etc/opt/remi/php82/php-fpm.d" {
		t.Fatalf("PHPFPMPoolDir = %q", provider.PHPFPMPoolDir("8.2"))
	}
	if provider.PHPFPMDefaultPoolConfig("8.2") != "/etc/opt/remi/php82/php-fpm.d/www.conf" {
		t.Fatalf("PHPFPMDefaultPoolConfig = %q", provider.PHPFPMDefaultPoolConfig("8.2"))
	}
	if provider.NginxLayout() != platform.NginxConfD {
		t.Fatalf("NginxLayout = %q", provider.NginxLayout())
	}
	if provider.NginxSiteConfigPath("mysite") != "/etc/nginx/conf.d/mysite.conf" {
		t.Fatalf("NginxSiteConfigPath = %q", provider.NginxSiteConfigPath("mysite"))
	}
	if provider.SudoGroup() != "wheel" {
		t.Fatalf("SudoGroup = %q", provider.SudoGroup())
	}
}

func TestForRejectsUnsupportedFamily(t *testing.T) {
	info := &platform.Info{
		Family: "unknown",
		Profile: platform.Profile{
			DistroID:     "fedora",
			SupportLevel: platform.SupportUnsupported,
		},
	}

	_, err := platform.For(info)
	if err == nil {
		t.Fatal("expected unsupported error")
	}
}

func TestForRejectsUnsupportedRHELVersion(t *testing.T) {
	profile := platform.ProfileFromOSRelease(`NAME="Rocky Linux"
ID="rocky"
VERSION_ID="8.10"
PRETTY_NAME="Rocky Linux 8.10"`)
	info := &platform.Info{
		Family:  profile.Family,
		Profile: profile,
	}
	_, err := platform.For(info)
	if err == nil {
		t.Fatal("expected unsupported Rocky 8 provider error")
	}
}
