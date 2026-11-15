package cache

import (
	"testing"

	"abstrax/internal/globals"
	"abstrax/internal/platform"
)

func TestDriverNamesValkey(t *testing.T) {
	provider := platform.RHELDefaults()
	pkg, svc, err := driverNames(provider, DriverValkey)
	if err != nil {
		t.Fatal(err)
	}
	if pkg != "valkey" || svc != "valkey" {
		t.Fatalf("got %s/%s", pkg, svc)
	}
}

func TestMaybeFallbackRedisToValkey(t *testing.T) {
	prev := globals.Flags.EnableRequiredRepos
	t.Cleanup(func() { globals.Flags.EnableRequiredRepos = prev })

	rocky10 := mustRHEL(t, "10.1")
	globals.Flags.EnableRequiredRepos = false
	if got := maybeFallbackRedisToValkey(rocky10, DriverRedis); got != DriverValkey {
		t.Fatalf("expected valkey fallback, got %q", got)
	}
	if got := maybeFallbackRedisToValkey(rocky10, DriverMemcached); got != DriverMemcached {
		t.Fatalf("memcached should be unchanged, got %q", got)
	}

	globals.Flags.EnableRequiredRepos = true
	if got := maybeFallbackRedisToValkey(rocky10, DriverRedis); got != DriverRedis {
		t.Fatalf("with --enable-required-repos expected redis, got %q", got)
	}

	rocky9 := mustRHEL(t, "9.5")
	globals.Flags.EnableRequiredRepos = false
	if got := maybeFallbackRedisToValkey(rocky9, DriverRedis); got != DriverRedis {
		t.Fatalf("rocky 9 should keep redis, got %q", got)
	}
}

func mustRHEL(t *testing.T, versionID string) platform.Provider {
	t.Helper()
	info := &platform.Info{
		Family: "rhel",
		Profile: platform.Profile{
			Family:             "rhel",
			DistroID:           "rocky",
			DistroName:         "Rocky Linux " + versionID,
			VersionID:          versionID,
			NginxLayout:        platform.NginxConfD,
			NginxConfigDir:     "/etc/nginx/conf.d",
			WebUser:            "nginx",
			WebGroup:           "nginx",
			DefaultProjectRoot: "/var/www",
			PackageManager:     "dnf",
			ServiceManager:     "systemd",
			FirewallStrategy:   "firewalld",
			SupportLevel:       platform.SupportOfficial,
		},
	}
	p, err := platform.For(info)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
