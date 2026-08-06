package platform_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"abstrax/internal/platform"
)

func debianProviderFor(t *testing.T, distroID, versionCodename, ubuntuCodename string) platform.Provider {
	t.Helper()
	info := &platform.Info{
		Family: "debian",
		Profile: platform.Profile{
			Family:          "debian",
			DistroID:        distroID,
			DistroName:      distroID,
			VersionCodename: versionCodename,
			UbuntuCodename:  ubuntuCodename,
			SupportLevel:    platform.SupportOfficial,
			WebUser:         "www-data",
			WebGroup:        "www-data",
		},
	}
	provider, err := platform.For(info)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func TestPHPAptSuiteDebian(t *testing.T) {
	provider := debianProviderFor(t, "debian", "trixie", "")
	suite, fallback, err := platform.PHPAptSuite(provider)
	if err != nil {
		t.Fatal(err)
	}
	if suite != "trixie" || fallback {
		t.Fatalf("suite=%q fallback=%v", suite, fallback)
	}
	if platform.UsesOndrejLaunchpadPPA(provider) {
		t.Fatal("debian should use packages.sury.org, not Launchpad")
	}
}

func TestPHPAptSuiteUbuntu(t *testing.T) {
	provider := debianProviderFor(t, "ubuntu", "noble", "noble")
	suite, fallback, err := platform.PHPAptSuite(provider)
	if err != nil {
		t.Fatal(err)
	}
	if suite != "noble" || fallback {
		t.Fatalf("suite=%q fallback=%v", suite, fallback)
	}
	if !platform.UsesOndrejLaunchpadPPA(provider) {
		t.Fatal("ubuntu should use Launchpad PPA")
	}
}

func TestPHPAptSuiteUbuntuFallback(t *testing.T) {
	provider := debianProviderFor(t, "ubuntu", "resolute", "resolute")
	suite, fallback, err := platform.PHPAptSuite(provider)
	if err != nil {
		t.Fatal(err)
	}
	if suite != "noble" || !fallback {
		t.Fatalf("suite=%q fallback=%v", suite, fallback)
	}
}

func TestPHPAptSuiteLinuxMintUsesUbuntuCodename(t *testing.T) {
	provider := debianProviderFor(t, "linuxmint", "virginia", "jammy")
	suite, fallback, err := platform.PHPAptSuite(provider)
	if err != nil {
		t.Fatal(err)
	}
	if suite != "jammy" || fallback {
		t.Fatalf("suite=%q fallback=%v", suite, fallback)
	}
}

func TestEnsureOndrejUbuntuWritesLaunchpadList(t *testing.T) {
	provider := debianProviderFor(t, "ubuntu", "noble", "noble")
	files := map[string][]byte{}
	err := platform.EnsureRepository(context.Background(), provider, platform.RepoOndrej, platform.RepoOptions{}, platform.RepoEnabler{
		Install: func(ctx context.Context, name string) error { return nil },
		Run: func(ctx context.Context, name string, args ...string) error {
			if name == "bash" {
				files["/usr/share/keyrings/abstrax-ondrej-php.gpg"] = []byte("key")
			}
			return nil
		},
		FileExists: func(path string) bool {
			_, ok := files[path]
			return ok
		},
		ReadFile: func(path string) ([]byte, error) {
			data, ok := files[path]
			if !ok {
				return nil, os.ErrNotExist
			}
			return data, nil
		},
		WriteFile: func(path string, data []byte, perm os.FileMode) error {
			files[path] = append([]byte(nil), data...)
			return nil
		},
		RemoveFile: func(path string) error {
			delete(files, path)
			return nil
		},
		Glob: func(pattern string) ([]string, error) { return nil, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	list := string(files["/etc/apt/sources.list.d/abstrax-ondrej-php.list"])
	if !strings.Contains(list, "ppa.launchpadcontent.net/ondrej/php/ubuntu") {
		t.Fatalf("list = %q", list)
	}
	if !strings.Contains(list, " noble ") {
		t.Fatalf("expected noble suite in %q", list)
	}
}

func TestEnsureOndrejIdempotent(t *testing.T) {
	provider := debianProviderFor(t, "debian", "trixie", "")
	want := "# Managed by Abstrax (packages.sury.org/php)\ndeb [signed-by=/usr/share/keyrings/debsuryorg-archive-keyring.gpg] https://packages.sury.org/php/ trixie main\n"
	files := map[string][]byte{
		"/etc/apt/sources.list.d/abstrax-ondrej-php.list":    []byte(want),
		"/usr/share/keyrings/debsuryorg-archive-keyring.gpg": []byte("key"),
	}
	writes := 0
	err := platform.EnsureRepository(context.Background(), provider, platform.RepoOndrej, platform.RepoOptions{}, platform.RepoEnabler{
		Install: func(ctx context.Context, name string) error {
			t.Fatal("should not install when already configured")
			return nil
		},
		Run: func(ctx context.Context, name string, args ...string) error {
			t.Fatal("should not run commands when already configured")
			return nil
		},
		FileExists: func(path string) bool {
			_, ok := files[path]
			return ok
		},
		ReadFile: func(path string) ([]byte, error) {
			data, ok := files[path]
			if !ok {
				return nil, os.ErrNotExist
			}
			return data, nil
		},
		WriteFile: func(path string, data []byte, perm os.FileMode) error {
			writes++
			files[path] = append([]byte(nil), data...)
			return nil
		},
		RemoveFile: func(path string) error {
			delete(files, path)
			return nil
		},
		Glob: func(pattern string) ([]string, error) { return nil, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if writes != 0 {
		t.Fatalf("unexpected writes: %d", writes)
	}
}
