package pkgmanager

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	executil "abstrax/internal/exec"
)

const aptNone = `Listing...
`

const aptMixed = `Listing...
openssl/jammy-security 3.0.2-0ubuntu1.18 amd64 [upgradable from: 3.0.2-0ubuntu1.15]
nginx/jammy-updates 1.18.0-6ubuntu14.4 amd64 [upgradable from: 1.18.0-6ubuntu14.3]
linux-image-generic/jammy-security,jammy-updates 5.15.0.91.89 amd64 [upgradable from: 5.15.0.88.86]
curl/jammy 7.81.0-1ubuntu1.16 amd64 [upgradable from: 7.81.0-1ubuntu1.15]
custom-tool/jammy 2.0-security amd64 [upgradable from: 1.0]
`

const dnfNone = `Last metadata expiration check: 0:01:00 ago on Sat 31 Oct 2026 11:00:00 PM UTC.
`

const dnfMixed = `Last metadata expiration check: 0:01:00 ago on Sat 31 Oct 2026 11:00:00 PM UTC.

openssl.x86_64                    1:3.0.7-25.el9_4          baseos
nginx.x86_64                      1:1.20.1-14.el9           appstream
kernel-core.x86_64                5.14.0-427.13.1.el9_4     baseos

Obsoleting Packages
oldpkg.x86_64                     2.0-1.el9                 appstream
`

const dnfSecurity = `Last metadata expiration check: 0:01:00 ago on Sat 31 Oct 2026 11:00:00 PM UTC.
RHSA-2024:1317 Important/Sec. openssl-1:3.0.7-25.el9_4.x86_64
RHSA-2024:2000 Important/Sec. kernel-core-5.14.0-427.13.1.el9_4.x86_64
`

const rpmInstalled = `openssl|1:3.0.7-18.el9_2
nginx|1:1.20.1-10.el9
kernel-core|5.14.0-362.el9
`

func TestParseAptUpgradableNone(t *testing.T) {
	got := parseAptUpgradable(aptNone)
	if len(got) != 0 {
		t.Fatalf("got %#v", got)
	}
}

func TestParseAptUpgradableMixed(t *testing.T) {
	got := parseAptUpgradable(aptMixed)
	want := []UpgradeInfo{
		{Name: "openssl", InstalledVersion: "3.0.2-0ubuntu1.15", NewVersion: "3.0.2-0ubuntu1.18", Security: true},
		{Name: "nginx", InstalledVersion: "1.18.0-6ubuntu14.3", NewVersion: "1.18.0-6ubuntu14.4", Security: false},
		{Name: "linux-image-generic", InstalledVersion: "5.15.0.88.86", NewVersion: "5.15.0.91.89", Security: true},
		{Name: "curl", InstalledVersion: "7.81.0-1ubuntu1.15", NewVersion: "7.81.0-1ubuntu1.16", Security: false},
		{Name: "custom-tool", InstalledVersion: "1.0", NewVersion: "2.0-security", Security: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
}

func TestParseDnfCheckUpdateNone(t *testing.T) {
	got := parseDnfCheckUpdate(dnfNone)
	if len(got) != 0 {
		t.Fatalf("got %#v", got)
	}
}

func TestParseDnfCheckUpdateMixed(t *testing.T) {
	got := parseDnfCheckUpdate(dnfMixed)
	want := []UpgradeInfo{
		{Name: "openssl", NewVersion: "1:3.0.7-25.el9_4"},
		{Name: "nginx", NewVersion: "1:1.20.1-14.el9"},
		{Name: "kernel-core", NewVersion: "5.14.0-427.13.1.el9_4"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
	names := parseDnfSecurityNames(dnfSecurity)
	if _, ok := names["openssl"]; !ok {
		t.Fatalf("openssl not security: %#v", names)
	}
	if _, ok := names["kernel-core"]; !ok {
		t.Fatalf("kernel-core not security: %#v", names)
	}
	if _, ok := names["nginx"]; ok {
		t.Fatal("nginx marked security")
	}
}

func TestAptUpgradesSecurityOnly(t *testing.T) {
	m := NewApt(false, false)
	m.runner.SetCommandFunc(aptListFunc(aptMixed))
	got, err := m.Upgrades(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %#v", got)
	}
	for _, up := range got {
		if !up.Security {
			t.Fatalf("non-security package included: %#v", up)
		}
	}
	cmds := m.runner.Commands()
	if len(cmds) != 1 || !reflect.DeepEqual(cmds[0], []string{"apt", "list", "--upgradable"}) {
		t.Fatalf("commands = %#v", cmds)
	}
}

func TestAptUpgradeSecurityOnlyInstallsListedPackages(t *testing.T) {
	m := NewApt(false, false)
	m.runner.SetCommandFunc(func(ctx context.Context, name string, args []string) (executil.Result, bool, error) {
		if name == "apt" {
			return executil.Result{Stdout: aptMixed}, true, nil
		}
		if name == "env" {
			return executil.Result{}, true, nil
		}
		return executil.Result{}, true, fmt.Errorf("unexpected %s %v", name, args)
	})
	if err := m.Upgrade(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"apt", "list", "--upgradable"},
		{"env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y", "openssl", "linux-image-generic", "custom-tool"},
	}
	if !reflect.DeepEqual(m.runner.Commands(), want) {
		t.Fatalf("commands = %#v", m.runner.Commands())
	}
}

func TestAptUpgradeSecurityOnlyEmptyDoesNotInstall(t *testing.T) {
	m := NewApt(false, false)
	m.runner.SetCommandFunc(aptListFunc(aptNone))
	if err := m.Upgrade(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	cmds := m.runner.Commands()
	if len(cmds) != 1 {
		t.Fatalf("commands = %#v", cmds)
	}
}

func TestAptUpgradeWithoutFlagUnchanged(t *testing.T) {
	m := NewApt(false, false)
	m.runner.SetCommandFunc(func(ctx context.Context, name string, args []string) (executil.Result, bool, error) {
		if name == "env" {
			return executil.Result{}, true, nil
		}
		return executil.Result{}, true, fmt.Errorf("unexpected %s %v", name, args)
	})
	if err := m.Upgrade(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	want := []string{"env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "upgrade", "-y"}
	cmds := m.runner.Commands()
	if len(cmds) != 1 || !reflect.DeepEqual(cmds[0], want) {
		t.Fatalf("commands = %#v", cmds)
	}
}

func TestDnfUpgradesNone(t *testing.T) {
	m := NewDnf(false, false)
	m.runner.SetCommandFunc(func(ctx context.Context, name string, args []string) (executil.Result, bool, error) {
		if name == "dnf" && len(args) > 0 && args[0] == "check-update" {
			return executil.Result{Stdout: dnfNone, ExitCode: 0}, true, nil
		}
		return executil.Result{}, true, fmt.Errorf("unexpected %s %v", name, args)
	})
	got, err := m.Upgrades(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %#v", got)
	}
	if len(m.runner.Commands()) != 1 {
		t.Fatalf("commands = %#v", m.runner.Commands())
	}
}

func TestDnfUpgradesMixed(t *testing.T) {
	m := NewDnf(false, false)
	m.runner.SetCommandFunc(dnfScript("dnf", []string{"updateinfo", "list", "--security"}))
	got, err := m.Upgrades(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	want := []UpgradeInfo{
		{Name: "openssl", InstalledVersion: "1:3.0.7-18.el9_2", NewVersion: "1:3.0.7-25.el9_4", Security: true},
		{Name: "nginx", InstalledVersion: "1:1.20.1-10.el9", NewVersion: "1:1.20.1-14.el9", Security: false},
		{Name: "kernel-core", InstalledVersion: "5.14.0-362.el9", NewVersion: "5.14.0-427.13.1.el9_4", Security: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
	wantCmds := [][]string{
		{"dnf", "check-update"},
		{"dnf", "updateinfo", "list", "--security"},
		{"rpm", "-q", "--qf", "%{NAME}|%{EVR}\n", "openssl", "nginx", "kernel-core"},
	}
	if !reflect.DeepEqual(m.runner.Commands(), wantCmds) {
		t.Fatalf("commands = %#v", m.runner.Commands())
	}
}

func TestDnfUpgradesSecurityOnly(t *testing.T) {
	m := NewDnf(false, false)
	m.runner.SetCommandFunc(dnfScript("dnf", []string{"updateinfo", "list", "--security"}))
	got, err := m.Upgrades(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "openssl" || got[1].Name != "kernel-core" {
		t.Fatalf("got %#v", got)
	}
}

func TestDnfUpgradeCommands(t *testing.T) {
	m := NewDnf(false, false)
	m.runner.SetCommandFunc(func(ctx context.Context, name string, args []string) (executil.Result, bool, error) {
		return executil.Result{}, true, nil
	})
	if err := m.Upgrade(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if err := m.Upgrade(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"dnf", "upgrade", "-y"},
		{"dnf", "upgrade", "-y", "--security"},
	}
	if !reflect.DeepEqual(m.runner.Commands(), want) {
		t.Fatalf("commands = %#v", m.runner.Commands())
	}
}

func TestYumUpgradeSecurityAndList(t *testing.T) {
	m := NewYum(false, false)
	m.runner.SetCommandFunc(func(ctx context.Context, name string, args []string) (executil.Result, bool, error) {
		if name == "yum" && len(args) > 0 && args[0] == "update" {
			return executil.Result{}, true, nil
		}
		return dnfScript("yum", []string{"updateinfo", "list", "security"})(ctx, name, args)
	})
	if err := m.Upgrade(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Upgrades(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	cmds := m.runner.Commands()
	if len(cmds) < 2 {
		t.Fatalf("commands = %#v", cmds)
	}
	if !reflect.DeepEqual(cmds[0], []string{"yum", "update", "-y", "--security"}) {
		t.Fatalf("upgrade argv = %#v", cmds[0])
	}
	if !reflect.DeepEqual(cmds[1], []string{"yum", "check-update"}) {
		t.Fatalf("check-update argv = %#v", cmds[1])
	}
	if !reflect.DeepEqual(cmds[2], []string{"yum", "updateinfo", "list", "security"}) {
		t.Fatalf("updateinfo argv = %#v", cmds[2])
	}
}

func aptListFunc(stdout string) executil.CommandFunc {
	return func(ctx context.Context, name string, args []string) (executil.Result, bool, error) {
		if name == "apt" {
			return executil.Result{Stdout: stdout}, true, nil
		}
		return executil.Result{}, true, fmt.Errorf("unexpected %s %v", name, args)
	}
}

func dnfScript(bin string, infoArgs []string) executil.CommandFunc {
	return func(ctx context.Context, name string, args []string) (executil.Result, bool, error) {
		switch name {
		case bin:
			if len(args) > 0 && args[0] == "check-update" {
				return executil.Result{Stdout: dnfMixed, ExitCode: 100}, true, fmt.Errorf("exit status 100")
			}
			if reflect.DeepEqual(args, infoArgs) {
				return executil.Result{Stdout: dnfSecurity}, true, nil
			}
		case "rpm":
			return executil.Result{Stdout: rpmInstalled}, true, nil
		}
		return executil.Result{}, true, fmt.Errorf("unexpected %s %v", name, args)
	}
}
