package pkgmanager

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"abstrax/internal/validate"
)

// apt list --upgradable lines look like:
// openssl/jammy-security 3.0.2-0ubuntu1.18 amd64 [upgradable from: 3.0.2-0ubuntu1.15]
var aptUpgradablePattern = regexp.MustCompile(`^(\S+)/(\S+)\s+(\S+)\s+\S+\s+\[upgradable from:\s*(\S+)\]`)

// Upgrades lists pending package upgrades.
// securityOnly limits the result to security updates.
func (a *AptManager) Upgrades(ctx context.Context, securityOnly bool) ([]UpgradeInfo, error) {
	res, err := a.runner.RunQuery(ctx, "apt", "list", "--upgradable")
	if err != nil {
		if res.Stderr != "" {
			return nil, fmt.Errorf("apt list --upgradable: %s", strings.TrimSpace(res.Stderr))
		}
		return nil, fmt.Errorf("apt list --upgradable: %w", err)
	}
	ups := parseAptUpgradable(res.Stdout)
	if securityOnly {
		ups = filterSecurity(ups)
	}
	return ups, nil
}

func (a *AptManager) upgradeSecurity(ctx context.Context, env []string) error {
	ups, err := a.Upgrades(ctx, true)
	if err != nil {
		return err
	}
	if len(ups) == 0 {
		return nil
	}

	args := append(env, "apt-get", "install", "-y")
	for _, up := range ups {
		if err := validate.PackageName(up.Name); err != nil {
			return fmt.Errorf("refusing to install %q: %w", up.Name, err)
		}
		args = append(args, up.Name)
	}
	_, err = a.runner.Run(ctx, "env", args...)
	if err != nil {
		return fmt.Errorf("apt install security updates: %w", err)
	}
	return nil
}

func parseAptUpgradable(output string) []UpgradeInfo {
	var ups []UpgradeInfo
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		m := aptUpgradablePattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		origin := m[2]
		newVersion := m[3]
		ups = append(ups, UpgradeInfo{
			Name:             m[1],
			InstalledVersion: strings.TrimRight(m[4], "]"),
			NewVersion:       newVersion,
			Security:         aptSecurity(origin, newVersion),
		})
	}
	if ups == nil {
		ups = []UpgradeInfo{}
	}
	return ups
}

// aptSecurity reports whether a candidate comes from the security pocket.
// The candidate version or the origin (suite) contains "-security".
func aptSecurity(origin, candidate string) bool {
	return strings.Contains(origin, "-security") || strings.Contains(candidate, "-security")
}

func filterSecurity(ups []UpgradeInfo) []UpgradeInfo {
	out := make([]UpgradeInfo, 0)
	for _, up := range ups {
		if up.Security {
			out = append(out, up)
		}
	}
	return out
}

// Upgrades lists pending package upgrades from dnf/yum.
func (d *DnfManager) Upgrades(ctx context.Context, securityOnly bool) ([]UpgradeInfo, error) {
	bin := d.pm()
	res, err := d.runner.RunQuery(ctx, bin, "check-update")
	// check-update exits 100 when updates are available.
	if err != nil && res.ExitCode != 100 {
		if res.Stderr != "" {
			return nil, fmt.Errorf("%s check-update: %s", bin, strings.TrimSpace(res.Stderr))
		}
		return nil, fmt.Errorf("%s check-update: %w", bin, err)
	}

	ups := parseDnfCheckUpdate(res.Stdout)
	if len(ups) == 0 {
		return []UpgradeInfo{}, nil
	}

	infoArgs := []string{"updateinfo", "list", "--security"}
	if bin == "yum" {
		infoArgs = []string{"updateinfo", "list", "security"}
	}
	info, err := d.runner.RunQuery(ctx, bin, infoArgs...)
	if err != nil {
		if info.Stderr != "" {
			return nil, fmt.Errorf("%s updateinfo: %s", bin, strings.TrimSpace(info.Stderr))
		}
		return nil, fmt.Errorf("%s updateinfo: %w", bin, err)
	}
	security := parseDnfSecurityNames(info.Stdout)

	names := make([]string, 0, len(ups))
	seen := map[string]struct{}{}
	for i := range ups {
		if _, ok := security[ups[i].Name]; ok {
			ups[i].Security = true
		}
		if _, ok := seen[ups[i].Name]; ok {
			continue
		}
		seen[ups[i].Name] = struct{}{}
		names = append(names, ups[i].Name)
	}

	installed, err := d.installedVersions(ctx, names)
	if err != nil {
		return nil, err
	}
	for i := range ups {
		if evr, ok := installed[ups[i].Name]; ok {
			ups[i].InstalledVersion = evr
		}
	}
	if securityOnly {
		ups = filterSecurity(ups)
	}
	return ups, nil
}

func (d *DnfManager) installedVersions(ctx context.Context, names []string) (map[string]string, error) {
	args := append([]string{"-q", "--qf", "%{NAME}|%{EVR}\n"}, names...)
	res, err := d.runner.RunQuery(ctx, "rpm", args...)
	if err != nil && strings.TrimSpace(res.Stdout) == "" {
		if res.Stderr != "" {
			return nil, fmt.Errorf("rpm -q: %s", strings.TrimSpace(res.Stderr))
		}
		return nil, fmt.Errorf("rpm -q: %w", err)
	}
	return parseRPMVersions(res.Stdout), nil
}

func parseDnfCheckUpdate(output string) []UpgradeInfo {
	var ups []UpgradeInfo
	obsoleting := false
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.EqualFold(line, "Obsoleting Packages") {
			obsoleting = true
			continue
		}
		if obsoleting || skipCheckUpdateLine(line) {
			continue
		}
		fields := strings.Fields(line)
		var name, version string
		switch {
		case len(fields) >= 4 && isArch(fields[1]):
			name = fields[0]
			version = fields[2]
		case len(fields) >= 3:
			name, _ = splitNameArch(fields[0])
			version = fields[1]
		default:
			continue
		}
		if name == "" || version == "" || !isArchToken(fields[0], fields) {
			continue
		}
		ups = append(ups, UpgradeInfo{
			Name:       name,
			NewVersion: version,
		})
	}
	if ups == nil {
		ups = []UpgradeInfo{}
	}
	return ups
}

func isArchToken(first string, fields []string) bool {
	if len(fields) >= 4 && isArch(fields[1]) {
		return true
	}
	_, arch := splitNameArch(first)
	return arch != ""
}

func skipCheckUpdateLine(line string) bool {
	switch {
	case strings.HasPrefix(line, "Last metadata"):
		return true
	case strings.HasPrefix(line, "Loaded plugins"):
		return true
	case strings.HasPrefix(line, "Loading "):
		return true
	case strings.HasPrefix(line, "Determining "):
		return true
	case strings.HasPrefix(strings.ToLower(line), "package ") && strings.Contains(strings.ToLower(line), "arch"):
		return true
	default:
		return false
	}
}

func splitNameArch(token string) (string, string) {
	i := strings.LastIndex(token, ".")
	if i <= 0 {
		return token, ""
	}
	arch := token[i+1:]
	if isArch(arch) {
		return token[:i], arch
	}
	return token, ""
}

func parseDnfSecurityNames(output string) map[string]struct{} {
	names := map[string]struct{}{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Last metadata") || strings.HasPrefix(line, "Loaded plugins") {
			continue
		}
		for _, field := range strings.Fields(line) {
			name, ok := nevraPackageName(strings.Trim(field, ",;"))
			if ok {
				names[name] = struct{}{}
			}
		}
	}
	return names
}

func nevraPackageName(token string) (string, bool) {
	i := strings.LastIndex(token, ".")
	if i <= 0 {
		return "", false
	}
	if !isArch(token[i+1:]) {
		return "", false
	}
	rest := token[:i]
	if colon := strings.Index(rest, ":"); colon > 0 {
		head := rest[:colon]
		dash := strings.LastIndex(head, "-")
		if dash <= 0 || !isDigits(head[dash+1:]) {
			return "", false
		}
		return head[:dash], true
	}
	parts := strings.Split(rest, "-")
	for n := 1; n < len(parts); n++ {
		if len(parts[n]) > 0 && parts[n][0] >= '0' && parts[n][0] <= '9' {
			return strings.Join(parts[:n], "-"), true
		}
	}
	return "", false
}

func parseRPMVersions(output string) map[string]string {
	versions := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name, evr, ok := strings.Cut(line, "|")
		if !ok || name == "" {
			continue
		}
		if _, exists := versions[name]; exists {
			continue
		}
		versions[name] = evr
	}
	return versions
}

func isArch(arch string) bool {
	switch arch {
	case "x86_64", "aarch64", "noarch", "i686", "i386", "ppc64le", "s390x", "armv7hl", "src":
		return true
	default:
		return false
	}
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
