package pkgmanager

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	executil "abstrax/internal/exec"
	"abstrax/internal/platform"
)

// fakeCmds answers commands by their full command line. Unlisted commands fail
// so a test notices any command it did not expect.
type fakeCmds map[string]fakeResp

type fakeResp struct {
	stdout string
	fail   bool
}

func (f fakeCmds) handle(_ context.Context, name string, args []string) (executil.Result, bool, error) {
	line := strings.Join(append([]string{name}, args...), " ")
	resp, ok := f[line]
	if !ok {
		return executil.Result{ExitCode: 127}, true, errors.New("unexpected command: " + line)
	}
	if resp.fail {
		return executil.Result{Stdout: resp.stdout, ExitCode: 1}, true, errors.New("exit status 1")
	}
	return executil.Result{Stdout: resp.stdout}, true, nil
}

func newTestAutoUpdates(t *testing.T, family string, ubuntu, dryRun bool, cmds fakeCmds) *AutoUpdates {
	t.Helper()
	dir := t.TempDir()
	runner := executil.New(dryRun, false)
	runner.SetCommandFunc(cmds.handle)
	return &AutoUpdates{
		family: family,
		ubuntu: ubuntu,
		dryRun: dryRun,
		runner: runner,
		paths: AutoUpdatesPaths{
			AptConfig:      filepath.Join(dir, "apt.conf.d", "52abstrax-auto-updates"),
			AptStamp:       filepath.Join(dir, "periodic", "unattended-upgrades-stamp"),
			AptLog:         filepath.Join(dir, "log", "unattended-upgrades.log"),
			DnfConfig:      filepath.Join(dir, "abstrax", "dnf-automatic.conf"),
			DnfDropIn:      filepath.Join(dir, "systemd", "dnf-automatic-install.service.d", "50-abstrax.conf"),
			DnfStockConfig: filepath.Join(dir, "dnf", "automatic.conf"),
			DnfTimerStamp:  filepath.Join(dir, "timers", "stamp-dnf-automatic-install.timer"),
			DnfBinary:      "/usr/bin/dnf-automatic",
		},
		logZone: time.UTC,
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func ranCommand(u *AutoUpdates, line string) bool {
	for _, c := range u.runner.Commands() {
		if strings.Join(c, " ") == line {
			return true
		}
	}
	return false
}

func setMtime(t *testing.T, path string, at time.Time) {
	t.Helper()
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

const (
	dpkgQuery   = "dpkg-query -W -f=${Status} unattended-upgrades"
	aptInstall  = "env DEBIAN_FRONTEND=noninteractive apt-get install -y unattended-upgrades"
	aptCfgQuery = "apt-config dump APT::Periodic::Unattended-Upgrade"
	rpmQuery    = "rpm -q dnf-automatic"
	timerQuery  = "systemctl is-enabled dnf-automatic-install.timer"
	showQuery   = "systemctl show dnf-automatic-install.service -p Result -p ExecMainExitTimestamp"
)

var installedOK = fakeResp{stdout: "install ok installed"}

// ---------------------------------------------------------------------------
// Debian family
// ---------------------------------------------------------------------------

func TestAptEnableInstallsAndWritesSecurityOnlyConfig(t *testing.T) {
	u := newTestAutoUpdates(t, "debian", true, false, fakeCmds{
		dpkgQuery:  {fail: true},
		aptInstall: {},
	})
	change, err := u.Enable(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !change.Changed || !change.Enabled || change.Backend != AutoUpdatesBackendAPT {
		t.Fatalf("change = %#v", change)
	}
	if change.ConfigPath != u.paths.AptConfig {
		t.Fatalf("config path = %q", change.ConfigPath)
	}
	if !ranCommand(u, aptInstall) {
		t.Fatalf("expected install, got %v", u.runner.Commands())
	}

	conf := readTestFile(t, u.paths.AptConfig)
	for _, want := range []string{
		`APT::Periodic::Unattended-Upgrade "1";`,
		`Unattended-Upgrade::Automatic-Reboot "false";`,
		`#clear Unattended-Upgrade::Allowed-Origins;`,
		`#clear Unattended-Upgrade::Origins-Pattern;`,
		`origin=Ubuntu,archive=${distro_codename}-security`,
	} {
		if !strings.Contains(conf, want) {
			t.Errorf("config missing %q:\n%s", want, conf)
		}
	}
	if strings.Contains(conf, "}-updates") || strings.Contains(conf, "Debian-Security") {
		t.Errorf("config has non-security or wrong-distro origin:\n%s", conf)
	}
}

func TestAptEnableDebianOrigins(t *testing.T) {
	u := newTestAutoUpdates(t, "debian", false, false, fakeCmds{dpkgQuery: installedOK})
	if _, err := u.Enable(context.Background()); err != nil {
		t.Fatal(err)
	}
	conf := readTestFile(t, u.paths.AptConfig)
	if !strings.Contains(conf, "label=Debian-Security") || strings.Contains(conf, "origin=Ubuntu") {
		t.Fatalf("config:\n%s", conf)
	}
}

func TestAptEnableIsIdempotent(t *testing.T) {
	u := newTestAutoUpdates(t, "debian", true, false, fakeCmds{dpkgQuery: installedOK})
	writeTestFile(t, u.paths.AptConfig, aptEnabledConfig(true))

	change, err := u.Enable(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if change.Changed {
		t.Fatalf("expected no change, got %#v", change)
	}
	if ranCommand(u, aptInstall) {
		t.Fatal("must not reinstall an installed package")
	}
}

func TestAptDisableWritesZero(t *testing.T) {
	u := newTestAutoUpdates(t, "debian", true, false, fakeCmds{dpkgQuery: installedOK})
	writeTestFile(t, u.paths.AptConfig, aptEnabledConfig(true))

	change, err := u.Disable(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !change.Changed || change.Enabled {
		t.Fatalf("change = %#v", change)
	}
	conf := readTestFile(t, u.paths.AptConfig)
	if !strings.Contains(conf, `APT::Periodic::Unattended-Upgrade "0";`) {
		t.Fatalf("config:\n%s", conf)
	}
	if strings.Contains(conf, "Origins-Pattern") {
		t.Fatalf("disabled config should not carry origins:\n%s", conf)
	}
	if !strings.Contains(conf, `Automatic-Reboot "false"`) {
		t.Fatalf("config:\n%s", conf)
	}
}

func TestAptDisableWhenNeverConfiguredDoesNothing(t *testing.T) {
	u := newTestAutoUpdates(t, "debian", true, false, fakeCmds{dpkgQuery: {fail: true}})
	change, err := u.Disable(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if change.Changed {
		t.Fatalf("change = %#v", change)
	}
	if fileExists(u.paths.AptConfig) {
		t.Fatal("disable must not create a config when nothing was installed")
	}
}

func TestAptDisableWritesZeroWhenInstalledButUnmanaged(t *testing.T) {
	u := newTestAutoUpdates(t, "debian", true, false, fakeCmds{dpkgQuery: installedOK})
	change, err := u.Disable(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !change.Changed {
		t.Fatalf("change = %#v", change)
	}
	if !strings.Contains(readTestFile(t, u.paths.AptConfig), `"0"`) {
		t.Fatal("expected disabled config")
	}
}

func TestAptDryRunWritesNothing(t *testing.T) {
	u := newTestAutoUpdates(t, "debian", true, true, fakeCmds{
		dpkgQuery:  {fail: true},
		aptInstall: {},
	})
	change, err := u.Enable(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !change.Changed {
		t.Fatalf("change = %#v", change)
	}
	if fileExists(u.paths.AptConfig) {
		t.Fatal("dry run wrote a file")
	}
}

func TestAptStatusEnabled(t *testing.T) {
	u := newTestAutoUpdates(t, "debian", true, false, fakeCmds{
		dpkgQuery:   installedOK,
		aptCfgQuery: {stdout: `APT::Periodic::Unattended-Upgrade "1";`},
	})
	writeTestFile(t, u.paths.AptConfig, aptEnabledConfig(true))
	writeTestFile(t, u.paths.AptStamp, "")
	stamp := time.Date(2026, 11, 3, 6, 25, 11, 0, time.UTC)
	setMtime(t, u.paths.AptStamp, stamp)
	writeTestFile(t, u.paths.AptLog, strings.Join([]string{
		"2026-11-02 06:25:10,001 INFO Starting unattended upgrades script",
		"2026-11-03 06:25:11,123 INFO All upgrades installed",
		"",
	}, "\n"))

	st, err := u.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Installed || !st.Enabled || !st.Managed {
		t.Fatalf("status = %#v", st)
	}
	if st.LastRun == nil || *st.LastRun != "2026-11-03T06:25:11Z" {
		t.Fatalf("last run = %v", st.LastRun)
	}
	if st.LastResult != AutoUpdatesResultSuccess {
		t.Fatalf("last result = %q", st.LastResult)
	}
	if st.Backend != AutoUpdatesBackendAPT || st.ConfigPath != u.paths.AptConfig {
		t.Fatalf("status = %#v", st)
	}
}

func TestAptStatusDisabled(t *testing.T) {
	u := newTestAutoUpdates(t, "debian", true, false, fakeCmds{
		dpkgQuery:   installedOK,
		aptCfgQuery: {stdout: `APT::Periodic::Unattended-Upgrade "0";`},
	})
	writeTestFile(t, u.paths.AptConfig, aptDisabledConfig())

	st, err := u.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Installed || st.Enabled || !st.Managed {
		t.Fatalf("status = %#v", st)
	}
}

func TestAptStatusNeverRun(t *testing.T) {
	u := newTestAutoUpdates(t, "debian", true, false, fakeCmds{
		dpkgQuery:   installedOK,
		aptCfgQuery: {stdout: `APT::Periodic::Unattended-Upgrade "1";`},
	})
	st, err := u.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Enabled {
		t.Fatalf("status = %#v", st)
	}
	if st.LastRun != nil || st.LastResult != AutoUpdatesResultNever {
		t.Fatalf("status = %#v", st)
	}
}

func TestAptStatusNotInstalled(t *testing.T) {
	u := newTestAutoUpdates(t, "debian", true, false, fakeCmds{dpkgQuery: {fail: true}})
	st, err := u.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Installed || st.Enabled || st.LastRun != nil || st.LastResult != AutoUpdatesResultNever {
		t.Fatalf("status = %#v", st)
	}
}

func TestAptStatusFailedRunFromLogOnly(t *testing.T) {
	u := newTestAutoUpdates(t, "debian", true, false, fakeCmds{
		dpkgQuery:   installedOK,
		aptCfgQuery: {stdout: `APT::Periodic::Unattended-Upgrade "1";`},
	})
	writeTestFile(t, u.paths.AptLog,
		"2026-11-03 06:25:11,123 ERROR Installing the upgrades failed!\n")

	st, err := u.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.LastResult != AutoUpdatesResultFailed {
		t.Fatalf("status = %#v", st)
	}
	if st.LastRun == nil || *st.LastRun != "2026-11-03T06:25:11Z" {
		t.Fatalf("last run = %v", st.LastRun)
	}
}

func TestAptStatusStampWithoutLogIsUnknown(t *testing.T) {
	u := newTestAutoUpdates(t, "debian", true, false, fakeCmds{
		dpkgQuery:   installedOK,
		aptCfgQuery: {stdout: `APT::Periodic::Unattended-Upgrade "1";`},
	})
	writeTestFile(t, u.paths.AptStamp, "")
	st, err := u.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.LastRun == nil || st.LastResult != AutoUpdatesResultUnknown {
		t.Fatalf("status = %#v", st)
	}
}

func TestAptStatusFallsBackToDropInWithoutAptConfig(t *testing.T) {
	u := newTestAutoUpdates(t, "debian", true, false, fakeCmds{
		dpkgQuery:   installedOK,
		aptCfgQuery: {fail: true},
	})
	writeTestFile(t, u.paths.AptConfig, aptEnabledConfig(true))
	st, err := u.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Enabled {
		t.Fatalf("status = %#v", st)
	}
}

func TestStatusIsReadOnly(t *testing.T) {
	for _, tc := range []struct {
		name   string
		family string
		cmds   fakeCmds
	}{
		{"apt", "debian", fakeCmds{
			dpkgQuery:   installedOK,
			aptCfgQuery: {stdout: `APT::Periodic::Unattended-Upgrade "1";`},
		}},
		{"dnf", "rhel", fakeCmds{
			rpmQuery:   {},
			timerQuery: {stdout: "enabled"},
			showQuery:  {stdout: "Result=success\nExecMainExitTimestamp="},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := newTestAutoUpdates(t, tc.family, true, false, tc.cmds)
			if _, err := u.Status(context.Background()); err != nil {
				t.Fatal(err)
			}
			for _, c := range u.runner.Commands() {
				switch c[0] {
				case "dpkg-query", "apt-config", "rpm":
				case "systemctl":
					if c[1] != "is-enabled" && c[1] != "show" {
						t.Errorf("status ran a mutating command: %v", c)
					}
				default:
					t.Errorf("status ran unexpected command: %v", c)
				}
			}
			for _, path := range []string{u.paths.AptConfig, u.paths.DnfConfig, u.paths.DnfDropIn} {
				if fileExists(path) {
					t.Errorf("status created %s", path)
				}
			}
		})
	}
}

func TestParseUnattendedLogLastOutcomeWins(t *testing.T) {
	log := strings.Join([]string{
		"2026-11-01 06:00:00,000 INFO All upgrades installed",
		"2026-11-02 06:00:00,000 ERROR Installing the upgrades failed!",
		"2026-11-03 06:00:00,000 INFO No packages found that can be upgraded unattended and no pending auto-removals",
		"2026-11-03 06:00:01,000 INFO Some unrelated line",
	}, "\n")
	result, at := parseUnattendedLog(log, time.UTC)
	if result != AutoUpdatesResultSuccess {
		t.Fatalf("result = %q", result)
	}
	if !at.Equal(time.Date(2026, 11, 3, 6, 0, 0, 0, time.UTC)) {
		t.Fatalf("at = %v", at)
	}
}

func TestParseUnattendedLogIgnoresErrorWordInInfoLine(t *testing.T) {
	result, _ := parseUnattendedLog("2026-11-03 06:00:00,000 INFO Found no ERROR here\n", time.UTC)
	if result != "" {
		t.Fatalf("result = %q", result)
	}
}

func TestParseAptConfigValue(t *testing.T) {
	v, ok := parseAptConfigValue("APT::Periodic::Unattended-Upgrade \"1\";\nAPT::Periodic::Unattended-Upgrade \"0\";\n", "APT::Periodic::Unattended-Upgrade")
	if !ok || v != "0" {
		t.Fatalf("v=%q ok=%v", v, ok)
	}
	if _, ok := parseAptConfigValue("// APT::Periodic::Unattended-Upgrade \"1\";\n", "APT::Periodic::Unattended-Upgrade"); ok {
		t.Fatal("comment must not match")
	}
}

// ---------------------------------------------------------------------------
// RHEL family
// ---------------------------------------------------------------------------

func TestDnfEnableWritesConfigAndStartsTimer(t *testing.T) {
	u := newTestAutoUpdates(t, "rhel", false, false, fakeCmds{
		rpmQuery:                       {fail: true},
		"dnf install -y dnf-automatic": {},
		"systemctl daemon-reload":      {},
		timerQuery:                     {stdout: "disabled", fail: true},
		"systemctl enable --now dnf-automatic-install.timer": {},
	})
	change, err := u.Enable(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !change.Changed || !change.Enabled || change.Backend != AutoUpdatesBackendDNF {
		t.Fatalf("change = %#v", change)
	}
	if change.ConfigPath != u.paths.DnfConfig {
		t.Fatalf("config path = %q", change.ConfigPath)
	}

	conf := readTestFile(t, u.paths.DnfConfig)
	for _, want := range []string{"upgrade_type = security", "apply_updates = yes", "reboot = never"} {
		if !strings.Contains(conf, want) {
			t.Errorf("config missing %q:\n%s", want, conf)
		}
	}
	drop := readTestFile(t, u.paths.DnfDropIn)
	for _, want := range []string{
		"[Service]\n",
		"ExecStart=\n",
		"ExecStart=/usr/bin/dnf-automatic " + u.paths.DnfConfig + " --timer\n",
	} {
		if !strings.Contains(drop, want) {
			t.Errorf("drop-in missing %q:\n%s", want, drop)
		}
	}
	for _, line := range []string{
		"dnf install -y dnf-automatic",
		"systemctl daemon-reload",
		"systemctl enable --now dnf-automatic-install.timer",
	} {
		if !ranCommand(u, line) {
			t.Errorf("missing command %q in %v", line, u.runner.Commands())
		}
	}
	if fileExists(u.paths.DnfStockConfig) {
		t.Fatal("must not touch the distro config")
	}
}

func TestDnfEnableUnchangedWhenAlreadyEnabled(t *testing.T) {
	u := newTestAutoUpdates(t, "rhel", false, false, fakeCmds{
		rpmQuery:   {},
		timerQuery: {stdout: "enabled"},
		"systemctl enable --now dnf-automatic-install.timer": {},
	})
	writeTestFile(t, u.paths.DnfConfig, dnfAutomaticConfig())
	writeTestFile(t, u.paths.DnfDropIn, dnfDropIn(u.paths.DnfBinary, u.paths.DnfConfig))

	change, err := u.Enable(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if change.Changed {
		t.Fatalf("change = %#v", change)
	}
	if ranCommand(u, "systemctl daemon-reload") {
		t.Fatal("no reload needed when files are unchanged")
	}
}

func TestDnfDisableRemovesManagedFiles(t *testing.T) {
	u := newTestAutoUpdates(t, "rhel", false, false, fakeCmds{
		rpmQuery:   {},
		timerQuery: {stdout: "enabled"},
		"systemctl disable --now dnf-automatic-install.timer": {},
		"systemctl daemon-reload":                             {},
	})
	writeTestFile(t, u.paths.DnfConfig, dnfAutomaticConfig())
	writeTestFile(t, u.paths.DnfDropIn, dnfDropIn(u.paths.DnfBinary, u.paths.DnfConfig))

	change, err := u.Disable(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !change.Changed || change.Enabled {
		t.Fatalf("change = %#v", change)
	}
	if fileExists(u.paths.DnfConfig) || fileExists(u.paths.DnfDropIn) {
		t.Fatal("managed files should be removed")
	}
	if fileExists(filepath.Dir(u.paths.DnfDropIn)) || fileExists(filepath.Dir(u.paths.DnfConfig)) {
		t.Fatal("empty managed directories should be removed")
	}
	for _, line := range []string{
		"systemctl disable --now dnf-automatic-install.timer",
		"systemctl daemon-reload",
	} {
		if !ranCommand(u, line) {
			t.Errorf("missing command %q in %v", line, u.runner.Commands())
		}
	}
}

func TestDnfDisableWhenNothingConfiguredDoesNothing(t *testing.T) {
	u := newTestAutoUpdates(t, "rhel", false, false, fakeCmds{rpmQuery: {fail: true}})
	change, err := u.Disable(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if change.Changed {
		t.Fatalf("change = %#v", change)
	}
}

func TestDnfDryRunWritesNothing(t *testing.T) {
	u := newTestAutoUpdates(t, "rhel", false, true, fakeCmds{
		rpmQuery:                  {},
		timerQuery:                {fail: true},
		"systemctl daemon-reload": {},
		"systemctl enable --now dnf-automatic-install.timer": {},
	})
	if _, err := u.Enable(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fileExists(u.paths.DnfConfig) || fileExists(u.paths.DnfDropIn) {
		t.Fatal("dry run wrote files")
	}
}

func TestDnfStatusEnabled(t *testing.T) {
	u := newTestAutoUpdates(t, "rhel", false, false, fakeCmds{
		rpmQuery:   {},
		timerQuery: {stdout: "enabled"},
		showQuery:  {stdout: "Result=success\nExecMainExitTimestamp=Tue 2026-11-03 06:25:11 UTC"},
	})
	writeTestFile(t, u.paths.DnfConfig, dnfAutomaticConfig())
	writeTestFile(t, u.paths.DnfDropIn, dnfDropIn(u.paths.DnfBinary, u.paths.DnfConfig))
	writeTestFile(t, u.paths.DnfTimerStamp, "")
	setMtime(t, u.paths.DnfTimerStamp, time.Date(2026, 11, 3, 6, 25, 0, 0, time.UTC))

	st, err := u.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Installed || !st.Enabled || !st.Managed {
		t.Fatalf("status = %#v", st)
	}
	if st.LastRun == nil || *st.LastRun != "2026-11-03T06:25:00Z" {
		t.Fatalf("last run = %v", st.LastRun)
	}
	if st.LastResult != AutoUpdatesResultSuccess {
		t.Fatalf("last result = %q", st.LastResult)
	}
}

func TestDnfStatusFailedRun(t *testing.T) {
	u := newTestAutoUpdates(t, "rhel", false, false, fakeCmds{
		rpmQuery:   {},
		timerQuery: {stdout: "enabled"},
		showQuery:  {stdout: "Result=exit-code\nExecMainExitTimestamp=Tue 2026-11-03 06:25:11 UTC"},
	})
	writeTestFile(t, u.paths.DnfConfig, dnfAutomaticConfig())
	writeTestFile(t, u.paths.DnfDropIn, dnfDropIn(u.paths.DnfBinary, u.paths.DnfConfig))
	st, err := u.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.LastResult != AutoUpdatesResultFailed {
		t.Fatalf("status = %#v", st)
	}
}

func TestDnfStatusDisabled(t *testing.T) {
	u := newTestAutoUpdates(t, "rhel", false, false, fakeCmds{
		rpmQuery:   {},
		timerQuery: {stdout: "disabled", fail: true},
		showQuery:  {stdout: "Result=success\nExecMainExitTimestamp="},
	})
	st, err := u.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Installed || st.Enabled || st.Managed {
		t.Fatalf("status = %#v", st)
	}
}

func TestDnfStatusNeverRun(t *testing.T) {
	u := newTestAutoUpdates(t, "rhel", false, false, fakeCmds{
		rpmQuery:   {},
		timerQuery: {stdout: "enabled"},
		showQuery:  {stdout: "Result=success\nExecMainExitTimestamp="},
	})
	writeTestFile(t, u.paths.DnfConfig, dnfAutomaticConfig())
	writeTestFile(t, u.paths.DnfDropIn, dnfDropIn(u.paths.DnfBinary, u.paths.DnfConfig))
	st, err := u.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Enabled || st.LastRun != nil || st.LastResult != AutoUpdatesResultNever {
		t.Fatalf("status = %#v", st)
	}
}

func TestDnfStatusRunBeforeBootIsUnknown(t *testing.T) {
	u := newTestAutoUpdates(t, "rhel", false, false, fakeCmds{
		rpmQuery:   {},
		timerQuery: {stdout: "enabled"},
		showQuery:  {stdout: "Result=success\nExecMainExitTimestamp="},
	})
	writeTestFile(t, u.paths.DnfConfig, dnfAutomaticConfig())
	writeTestFile(t, u.paths.DnfDropIn, dnfDropIn(u.paths.DnfBinary, u.paths.DnfConfig))
	writeTestFile(t, u.paths.DnfTimerStamp, "")
	st, err := u.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.LastRun == nil || st.LastResult != AutoUpdatesResultUnknown {
		t.Fatalf("status = %#v", st)
	}
}

func TestDnfStatusUnmanagedUsesStockConfig(t *testing.T) {
	cmds := fakeCmds{
		rpmQuery:   {},
		timerQuery: {stdout: "enabled"},
		showQuery:  {stdout: "Result=success\nExecMainExitTimestamp="},
	}
	u := newTestAutoUpdates(t, "rhel", false, false, cmds)
	writeTestFile(t, u.paths.DnfStockConfig, "[commands]\napply_updates = no\n")
	st, err := u.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Enabled || st.Managed {
		t.Fatalf("timer on but apply_updates=no must not count as enabled: %#v", st)
	}

	u = newTestAutoUpdates(t, "rhel", false, false, cmds)
	writeTestFile(t, u.paths.DnfStockConfig, "[commands]\napply_updates = yes\n")
	st, err = u.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Enabled || st.Managed {
		t.Fatalf("status = %#v", st)
	}
}

func TestDnfStatusNotInstalled(t *testing.T) {
	u := newTestAutoUpdates(t, "rhel", false, false, fakeCmds{rpmQuery: {fail: true}})
	st, err := u.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Installed || st.Enabled || st.LastResult != AutoUpdatesResultNever {
		t.Fatalf("status = %#v", st)
	}
}

func TestParseINIBool(t *testing.T) {
	conf := "[other]\napply_updates = yes\n[commands]\n# apply_updates = no\napply_updates = Yes\n"
	if !parseINIBool(conf, "commands", "apply_updates") {
		t.Fatal("expected true")
	}
	if parseINIBool("[commands]\napply_updates = no\n", "commands", "apply_updates") {
		t.Fatal("expected false")
	}
	if parseINIBool("[other]\napply_updates = yes\n", "commands", "apply_updates") {
		t.Fatal("wrong section must not match")
	}
}

// ---------------------------------------------------------------------------
// JSON and construction
// ---------------------------------------------------------------------------

func TestStatusJSONShape(t *testing.T) {
	run := "2026-11-03T06:25:11Z"
	got, err := json.Marshal(AutoUpdatesStatus{
		Backend:    AutoUpdatesBackendAPT,
		Installed:  true,
		Enabled:    true,
		Managed:    true,
		ConfigPath: "/etc/apt/apt.conf.d/52abstrax-auto-updates",
		LastRun:    &run,
		LastResult: AutoUpdatesResultSuccess,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"backend":"unattended-upgrades","installed":true,"enabled":true,"managed":true,"config_path":"/etc/apt/apt.conf.d/52abstrax-auto-updates","last_run":"2026-11-03T06:25:11Z","last_result":"success"}`
	if string(got) != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}

	got, err = json.Marshal(AutoUpdatesStatus{Backend: AutoUpdatesBackendDNF, LastResult: AutoUpdatesResultNever})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"last_run":null`) {
		t.Fatalf("last_run must be null: %s", got)
	}
}

func TestNewAutoUpdatesRejectsUnknownFamily(t *testing.T) {
	if _, err := NewAutoUpdates(platformProfile("unknown", ""), false, false); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewAutoUpdatesBackendByFamily(t *testing.T) {
	deb, err := NewAutoUpdates(platformProfile("debian", "ubuntu"), false, false)
	if err != nil || deb.Backend() != AutoUpdatesBackendAPT || !deb.ubuntu {
		t.Fatalf("debian: %v %#v", err, deb)
	}
	rhel, err := NewAutoUpdates(platformProfile("rhel", "rocky"), false, false)
	if err != nil || rhel.Backend() != AutoUpdatesBackendDNF {
		t.Fatalf("rhel: %v %#v", err, rhel)
	}
}

func platformProfile(family, distro string) platform.Profile {
	return platform.Profile{Family: family, DistroID: distro}
}
