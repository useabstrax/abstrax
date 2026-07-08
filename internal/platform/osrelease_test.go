package platform_test

import (
	"os"
	"path/filepath"
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
		{"fedora-40.os-release", "fedora", "unknown", platform.SupportUnsupported, "", "", platform.NginxLayoutUnknown, "unknown"},
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
		t.Fatal("expected error for unsupported family")
	}
}
