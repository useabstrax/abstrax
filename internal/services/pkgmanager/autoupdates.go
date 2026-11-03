package pkgmanager

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	executil "abstrax/internal/exec"
	"abstrax/internal/platform"
)

// Automatic update backends.
const (
	// AutoUpdatesBackendAPT is unattended-upgrades on Debian-family systems.
	AutoUpdatesBackendAPT = "unattended-upgrades"
	// AutoUpdatesBackendDNF is dnf-automatic on RHEL-family systems.
	AutoUpdatesBackendDNF = "dnf-automatic"
)

// Values of AutoUpdatesStatus.LastResult.
const (
	AutoUpdatesResultSuccess = "success"
	AutoUpdatesResultFailed  = "failed"
	AutoUpdatesResultNever   = "never_run"
	AutoUpdatesResultUnknown = "unknown"
)

const (
	aptAutoUpdatesPackage = "unattended-upgrades"
	dnfAutoUpdatesPackage = "dnf-automatic"
	dnfAutoUpdatesTimer   = "dnf-automatic-install.timer"
	dnfAutoUpdatesService = "dnf-automatic-install.service"

	// logTailBytes bounds how much of the unattended-upgrades log is read.
	logTailBytes = 64 << 10
)

// AutoUpdatesPaths are the files Abstrax writes and reads for automatic updates.
// Tests inject temporary paths; production uses DefaultAutoUpdatesPaths.
type AutoUpdatesPaths struct {
	// AptConfig is the Abstrax-owned apt drop-in.
	AptConfig string
	// AptStamp is touched by apt's daily job each time unattended-upgrades runs.
	AptStamp string
	// AptLog is the unattended-upgrades log.
	AptLog string

	// DnfConfig is the Abstrax-owned dnf-automatic configuration.
	DnfConfig string
	// DnfDropIn is the systemd drop-in that points the service at DnfConfig.
	DnfDropIn string
	// DnfStockConfig is the distro dnf-automatic configuration. It is only read.
	DnfStockConfig string
	// DnfTimerStamp is the file systemd touches when the persistent timer fires.
	DnfTimerStamp string
	// DnfBinary is the dnf-automatic executable the drop-in runs.
	DnfBinary string
}

// DefaultAutoUpdatesPaths returns the production paths.
func DefaultAutoUpdatesPaths() AutoUpdatesPaths {
	return AutoUpdatesPaths{
		AptConfig:      "/etc/apt/apt.conf.d/52abstrax-auto-updates",
		AptStamp:       "/var/lib/apt/periodic/unattended-upgrades-stamp",
		AptLog:         "/var/log/unattended-upgrades/unattended-upgrades.log",
		DnfConfig:      "/etc/abstrax/dnf-automatic.conf",
		DnfDropIn:      "/etc/systemd/system/dnf-automatic-install.service.d/50-abstrax.conf",
		DnfStockConfig: "/etc/dnf/automatic.conf",
		DnfTimerStamp:  "/var/lib/systemd/timers/stamp-dnf-automatic-install.timer",
		DnfBinary:      "/usr/bin/dnf-automatic",
	}
}

// AutoUpdatesStatus describes automatic security updates on this host.
type AutoUpdatesStatus struct {
	// Backend is "unattended-upgrades" or "dnf-automatic".
	Backend string `json:"backend"`
	// Installed reports whether the backend package is installed.
	Installed bool `json:"installed"`
	// Enabled reports whether updates are applied automatically right now.
	Enabled bool `json:"enabled"`
	// Managed reports whether Abstrax wrote the configuration in use.
	Managed bool `json:"managed"`
	// ConfigPath is the Abstrax-owned configuration file.
	ConfigPath string `json:"config_path"`
	// LastRun is the last run time as RFC3339 UTC, or null when unknown.
	LastRun *string `json:"last_run"`
	// LastResult is success, failed, unknown, or never_run.
	LastResult string `json:"last_result"`
}

// AutoUpdatesChange is the result of enabling or disabling automatic updates.
type AutoUpdatesChange struct {
	Backend    string `json:"backend"`
	Enabled    bool   `json:"enabled"`
	ConfigPath string `json:"config_path"`
	// Changed is false when the host was already in the requested state.
	Changed bool `json:"changed"`
}

// AutoUpdates manages automatic security updates.
// Debian-family hosts use unattended-upgrades and RHEL-family hosts use
// dnf-automatic. Neither backend ever reboots the server.
type AutoUpdates struct {
	family  string
	ubuntu  bool
	dryRun  bool
	runner  *executil.Runner
	paths   AutoUpdatesPaths
	logZone *time.Location
}

// NewAutoUpdates creates an AutoUpdates service for a platform profile.
func NewAutoUpdates(profile platform.Profile, dryRun, verbose bool) (*AutoUpdates, error) {
	switch profile.Family {
	case "debian", "rhel":
	default:
		return nil, fmt.Errorf("automatic updates are not supported on this platform (family %q)", profile.Family)
	}
	return &AutoUpdates{
		family:  profile.Family,
		ubuntu:  profile.DistroID == "ubuntu" || profile.UbuntuCodename != "",
		dryRun:  dryRun,
		runner:  executil.New(dryRun, verbose),
		paths:   DefaultAutoUpdatesPaths(),
		logZone: time.Local,
	}, nil
}

// NewAutoUpdatesFromPlatform detects the platform and returns the matching service.
func NewAutoUpdatesFromPlatform(dryRun, verbose bool) (*AutoUpdates, error) {
	info, _, err := platform.Detect()
	if err != nil {
		return nil, fmt.Errorf("platform detection failed: %w", err)
	}
	profile := info.Profile
	if profile.Family == "" {
		profile.Family = info.Family
	}
	return NewAutoUpdates(profile, dryRun, verbose)
}

// Backend returns the backend name for this host.
func (u *AutoUpdates) Backend() string {
	if u.family == "rhel" {
		return AutoUpdatesBackendDNF
	}
	return AutoUpdatesBackendAPT
}

// Status reports automatic update state. It changes nothing.
func (u *AutoUpdates) Status(ctx context.Context) (*AutoUpdatesStatus, error) {
	if u.family == "rhel" {
		return u.dnfStatus(ctx)
	}
	return u.aptStatus(ctx)
}

// Enable turns on automatic security updates, installing the backend if needed.
func (u *AutoUpdates) Enable(ctx context.Context) (*AutoUpdatesChange, error) {
	if u.family == "rhel" {
		return u.dnfEnable(ctx)
	}
	return u.aptEnable(ctx)
}

// Disable turns off automatic updates. Packages stay installed.
func (u *AutoUpdates) Disable(ctx context.Context) (*AutoUpdatesChange, error) {
	if u.family == "rhel" {
		return u.dnfDisable(ctx)
	}
	return u.aptDisable(ctx)
}

// ---------------------------------------------------------------------------
// Debian family: unattended-upgrades
// ---------------------------------------------------------------------------

// Security-origin patterns. Config lists in apt append, so the drop-in clears
// the distro lists first and then sets security origins only.
const aptHeader = `// Managed by Abstrax. Changes are overwritten by "abstrax package auto-updates".
`

const aptTail = `Unattended-Upgrade::Automatic-Reboot "false";
`

func aptEnabledConfig(ubuntu bool) string {
	patterns := []string{
		`"origin=Debian,codename=${distro_codename},label=Debian-Security";`,
		`"origin=Debian,codename=${distro_codename}-security,label=Debian-Security";`,
	}
	if ubuntu {
		patterns = []string{
			`"origin=Ubuntu,archive=${distro_codename}-security";`,
			`"origin=UbuntuESMApps,codename=${distro_codename}-apps-security";`,
			`"origin=UbuntuESM,codename=${distro_codename}-infra-security";`,
		}
	}
	var b strings.Builder
	b.WriteString(aptHeader)
	b.WriteString("// Security updates only. The server is never rebooted automatically.\n")
	b.WriteString("APT::Periodic::Update-Package-Lists \"1\";\n")
	b.WriteString("APT::Periodic::Unattended-Upgrade \"1\";\n")
	b.WriteString("#clear Unattended-Upgrade::Allowed-Origins;\n")
	b.WriteString("#clear Unattended-Upgrade::Origins-Pattern;\n")
	b.WriteString("Unattended-Upgrade::Origins-Pattern {\n")
	for _, p := range patterns {
		b.WriteString("\t" + p + "\n")
	}
	b.WriteString("};\n")
	b.WriteString(aptTail)
	return b.String()
}

func aptDisabledConfig() string {
	return aptHeader +
		"// Automatic updates are disabled.\n" +
		"APT::Periodic::Unattended-Upgrade \"0\";\n" +
		aptTail
}

func (u *AutoUpdates) aptInstalled(ctx context.Context) bool {
	res, err := u.runner.RunQuery(ctx, "dpkg-query", "-W", "-f=${Status}", aptAutoUpdatesPackage)
	return err == nil && strings.Contains(res.Stdout, "install ok installed")
}

func (u *AutoUpdates) aptEnable(ctx context.Context) (*AutoUpdatesChange, error) {
	changed := false
	if !u.aptInstalled(ctx) {
		if _, err := u.runner.Run(ctx, "env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y", aptAutoUpdatesPackage); err != nil {
			return nil, fmt.Errorf("installing %s: %w", aptAutoUpdatesPackage, err)
		}
		changed = true
	}
	wrote, err := u.writeIfChanged(u.paths.AptConfig, aptEnabledConfig(u.ubuntu))
	if err != nil {
		return nil, err
	}
	return &AutoUpdatesChange{
		Backend:    AutoUpdatesBackendAPT,
		Enabled:    true,
		ConfigPath: u.paths.AptConfig,
		Changed:    changed || wrote,
	}, nil
}

func (u *AutoUpdates) aptDisable(ctx context.Context) (*AutoUpdatesChange, error) {
	change := &AutoUpdatesChange{
		Backend:    AutoUpdatesBackendAPT,
		Enabled:    false,
		ConfigPath: u.paths.AptConfig,
	}
	_, statErr := os.Stat(u.paths.AptConfig)
	if statErr != nil && !os.IsNotExist(statErr) {
		return nil, statErr
	}
	// Nothing was ever installed or configured, so there is nothing to turn off.
	if os.IsNotExist(statErr) && !u.aptInstalled(ctx) {
		return change, nil
	}
	wrote, err := u.writeIfChanged(u.paths.AptConfig, aptDisabledConfig())
	if err != nil {
		return nil, err
	}
	change.Changed = wrote
	return change, nil
}

func (u *AutoUpdates) aptStatus(ctx context.Context) (*AutoUpdatesStatus, error) {
	st := &AutoUpdatesStatus{
		Backend:    AutoUpdatesBackendAPT,
		ConfigPath: u.paths.AptConfig,
		LastResult: AutoUpdatesResultNever,
	}
	st.Installed = u.aptInstalled(ctx)
	if _, err := os.Stat(u.paths.AptConfig); err == nil {
		st.Managed = true
	}
	if !st.Installed {
		return st, nil
	}
	st.Enabled = u.aptPeriodicEnabled(ctx)

	var lastRun time.Time
	if fi, err := os.Stat(u.paths.AptStamp); err == nil {
		lastRun = fi.ModTime()
	}
	result, resultTime := u.aptLogResult()
	if lastRun.IsZero() {
		lastRun = resultTime
	}
	st.LastRun = rfc3339UTC(lastRun)
	switch {
	case result != "":
		st.LastResult = result
	case st.LastRun != nil:
		st.LastResult = AutoUpdatesResultUnknown
	}
	return st, nil
}

// aptPeriodicEnabled reads the effective APT::Periodic::Unattended-Upgrade value.
// apt-config merges every conf.d file, so this reflects what apt will really do.
// If apt-config is unavailable the Abstrax drop-in is read instead.
func (u *AutoUpdates) aptPeriodicEnabled(ctx context.Context) bool {
	res, err := u.runner.RunQuery(ctx, "apt-config", "dump", "APT::Periodic::Unattended-Upgrade")
	if err == nil {
		if v, ok := parseAptConfigValue(res.Stdout, "APT::Periodic::Unattended-Upgrade"); ok {
			return aptPeriodicOn(v)
		}
		return false
	}
	data, rerr := os.ReadFile(u.paths.AptConfig)
	if rerr != nil {
		return false
	}
	v, ok := parseAptConfigValue(string(data), "APT::Periodic::Unattended-Upgrade")
	return ok && aptPeriodicOn(v)
}

func aptPeriodicOn(v string) bool {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	return err == nil && n > 0
}

// parseAptConfigValue finds `Key "value";` in apt-config dump or apt.conf output.
// The last assignment wins.
func parseAptConfigValue(content, key string) (string, bool) {
	var value string
	found := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "//") || !strings.HasPrefix(line, key) {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, key))
		if !strings.HasPrefix(rest, "\"") {
			continue
		}
		rest = rest[1:]
		end := strings.Index(rest, "\"")
		if end < 0 {
			continue
		}
		value = rest[:end]
		found = true
	}
	return value, found
}

// aptLogResult finds the last run outcome recorded in the unattended-upgrades log.
// It returns an empty result when the log is missing or has no outcome line.
func (u *AutoUpdates) aptLogResult() (string, time.Time) {
	data, err := readTail(u.paths.AptLog, logTailBytes)
	if err != nil {
		return "", time.Time{}
	}
	return parseUnattendedLog(string(data), u.logZone)
}

// parseUnattendedLog scans log lines such as
// "2026-11-03 06:25:11,123 INFO All upgrades installed" and returns the
// outcome of the last run plus when it was logged.
func parseUnattendedLog(content string, zone *time.Location) (string, time.Time) {
	if zone == nil {
		zone = time.Local
	}
	result := ""
	var at time.Time
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		level := fields[2]
		message := strings.Join(fields[3:], " ")
		var outcome string
		switch {
		case level == "ERROR", strings.HasPrefix(message, "Installing the upgrades failed"):
			outcome = AutoUpdatesResultFailed
		case strings.HasPrefix(message, "All upgrades installed"),
			strings.HasPrefix(message, "No packages found that can be upgraded unattended"):
			outcome = AutoUpdatesResultSuccess
		default:
			continue
		}
		ts, err := time.ParseInLocation("2006-01-02 15:04:05", fields[0]+" "+strings.SplitN(fields[1], ",", 2)[0], zone)
		if err != nil {
			continue
		}
		result = outcome
		at = ts
	}
	return result, at
}

// ---------------------------------------------------------------------------
// RHEL family: dnf-automatic
// ---------------------------------------------------------------------------

func dnfAutomaticConfig() string {
	return `# Managed by Abstrax. Changes are overwritten by "abstrax package auto-updates".
# Security updates only. The server is never rebooted automatically.
[commands]
upgrade_type = security
random_sleep = 0
download_updates = yes
apply_updates = yes
reboot = never

[emitters]
emit_via = stdio
`
}

func dnfDropIn(binary, configPath string) string {
	return fmt.Sprintf(`# Managed by Abstrax. Runs dnf-automatic with the Abstrax configuration.
[Service]
ExecStart=
ExecStart=%s %s --timer
`, binary, configPath)
}

func (u *AutoUpdates) dnfInstalled(ctx context.Context) bool {
	_, err := u.runner.RunQuery(ctx, "rpm", "-q", dnfAutoUpdatesPackage)
	return err == nil
}

func (u *AutoUpdates) dnfTimerEnabled(ctx context.Context) bool {
	res, err := u.runner.RunQuery(ctx, "systemctl", "is-enabled", dnfAutoUpdatesTimer)
	return err == nil && strings.TrimSpace(res.Stdout) == "enabled"
}

func (u *AutoUpdates) dnfEnable(ctx context.Context) (*AutoUpdatesChange, error) {
	changed := false
	if !u.dnfInstalled(ctx) {
		if _, err := u.runner.Run(ctx, "dnf", "install", "-y", dnfAutoUpdatesPackage); err != nil {
			return nil, fmt.Errorf("installing %s: %w", dnfAutoUpdatesPackage, err)
		}
		changed = true
	}
	wroteConf, err := u.writeIfChanged(u.paths.DnfConfig, dnfAutomaticConfig())
	if err != nil {
		return nil, err
	}
	wroteDrop, err := u.writeIfChanged(u.paths.DnfDropIn, dnfDropIn(u.paths.DnfBinary, u.paths.DnfConfig))
	if err != nil {
		return nil, err
	}
	if wroteConf || wroteDrop {
		changed = true
		if _, err := u.runner.Run(ctx, "systemctl", "daemon-reload"); err != nil {
			return nil, fmt.Errorf("systemctl daemon-reload: %w", err)
		}
	}
	if !u.dnfTimerEnabled(ctx) {
		changed = true
	}
	// Always run enable --now so a stopped timer is started again.
	if _, err := u.runner.Run(ctx, "systemctl", "enable", "--now", dnfAutoUpdatesTimer); err != nil {
		return nil, fmt.Errorf("enabling %s: %w", dnfAutoUpdatesTimer, err)
	}
	return &AutoUpdatesChange{
		Backend:    AutoUpdatesBackendDNF,
		Enabled:    true,
		ConfigPath: u.paths.DnfConfig,
		Changed:    changed,
	}, nil
}

func (u *AutoUpdates) dnfDisable(ctx context.Context) (*AutoUpdatesChange, error) {
	change := &AutoUpdatesChange{
		Backend:    AutoUpdatesBackendDNF,
		Enabled:    false,
		ConfigPath: u.paths.DnfConfig,
	}
	if u.dnfInstalled(ctx) && u.dnfTimerEnabled(ctx) {
		if _, err := u.runner.Run(ctx, "systemctl", "disable", "--now", dnfAutoUpdatesTimer); err != nil {
			return nil, fmt.Errorf("disabling %s: %w", dnfAutoUpdatesTimer, err)
		}
		change.Changed = true
	}
	removedDrop, err := u.removeFile(u.paths.DnfDropIn)
	if err != nil {
		return nil, err
	}
	removedConf, err := u.removeFile(u.paths.DnfConfig)
	if err != nil {
		return nil, err
	}
	if removedDrop || removedConf {
		change.Changed = true
		u.removeEmptyDir(filepath.Dir(u.paths.DnfDropIn))
		u.removeEmptyDir(filepath.Dir(u.paths.DnfConfig))
		if u.dnfInstalled(ctx) {
			if _, err := u.runner.Run(ctx, "systemctl", "daemon-reload"); err != nil {
				return nil, fmt.Errorf("systemctl daemon-reload: %w", err)
			}
		}
	}
	return change, nil
}

func (u *AutoUpdates) dnfStatus(ctx context.Context) (*AutoUpdatesStatus, error) {
	st := &AutoUpdatesStatus{
		Backend:    AutoUpdatesBackendDNF,
		ConfigPath: u.paths.DnfConfig,
		LastResult: AutoUpdatesResultNever,
	}
	st.Installed = u.dnfInstalled(ctx)
	_, confErr := os.Stat(u.paths.DnfConfig)
	_, dropErr := os.Stat(u.paths.DnfDropIn)
	st.Managed = confErr == nil && dropErr == nil
	if !st.Installed {
		return st, nil
	}

	// The timer must be enabled and the configuration it uses must apply updates.
	configPath := u.paths.DnfStockConfig
	if st.Managed {
		configPath = u.paths.DnfConfig
	}
	applies := false
	if data, err := os.ReadFile(configPath); err == nil {
		applies = parseINIBool(string(data), "commands", "apply_updates")
	}
	st.Enabled = u.dnfTimerEnabled(ctx) && applies

	var lastRun time.Time
	if fi, err := os.Stat(u.paths.DnfTimerStamp); err == nil {
		lastRun = fi.ModTime()
	}
	st.LastRun = rfc3339UTC(lastRun)

	// systemd only remembers the last service result since boot.
	// An empty exit timestamp means the service has not run since then.
	res, err := u.runner.RunQuery(ctx, "systemctl", "show", dnfAutoUpdatesService,
		"-p", "Result", "-p", "ExecMainExitTimestamp")
	exited := false
	failed := false
	if err == nil {
		props := parseKeyValues(res.Stdout)
		exited = strings.TrimSpace(props["ExecMainExitTimestamp"]) != ""
		failed = exited && strings.TrimSpace(props["Result"]) != "success"
	}
	switch {
	case failed:
		st.LastResult = AutoUpdatesResultFailed
	case exited:
		st.LastResult = AutoUpdatesResultSuccess
	case st.LastRun != nil:
		st.LastResult = AutoUpdatesResultUnknown
	}
	return st, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (u *AutoUpdates) writeIfChanged(path, content string) (bool, error) {
	existing, err := os.ReadFile(path)
	if err == nil && string(existing) == content {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if u.dryRun {
		u.runner.Note("write-file", path)
		return true, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func (u *AutoUpdates) removeFile(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if u.dryRun {
		u.runner.Note("remove-file", path)
		return true, nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	return true, nil
}

// removeEmptyDir removes dir when it is empty. Other errors are ignored.
func (u *AutoUpdates) removeEmptyDir(dir string) {
	if u.dryRun {
		return
	}
	_ = os.Remove(dir)
}

func rfc3339UTC(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

// readTail returns up to max bytes from the end of a file.
func readTail(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if fi.Size() > max {
		if _, err := f.Seek(fi.Size()-max, io.SeekStart); err != nil {
			return nil, err
		}
	}
	return io.ReadAll(f)
}

// parseINIBool reports whether section.key is a true value (yes, true, 1, on).
func parseINIBool(content, section, key string) bool {
	current := ""
	value := ""
	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		if current != section {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(k) != key {
			continue
		}
		value = strings.ToLower(strings.TrimSpace(v))
	}
	switch value {
	case "yes", "true", "1", "on":
		return true
	default:
		return false
	}
}

// parseKeyValues parses `Key=value` lines such as systemctl show output.
func parseKeyValues(s string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok {
			out[k] = v
		}
	}
	return out
}
