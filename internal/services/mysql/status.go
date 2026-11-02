package mysql

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	executil "abstrax/internal/exec"
	"abstrax/internal/platform"
	"abstrax/internal/services/pkgmanager"
	"abstrax/internal/services/svcmanager"
)

// serverVersionPattern matches the numeric version at the start of MySQL and
// MariaDB version strings, including distro suffixes such as
// 8.0.39-0ubuntu0.24.04.1 and 10.11.6-MariaDB-0+deb12u1.
var serverVersionPattern = regexp.MustCompile(`\d+\.\d+\.\d+`)

// Status reports whether a MySQL-compatible server is installed and, when it
// is, which engine is present and how the service is running.
// Version is null when the version cannot be determined. Fields that describe
// a running or installed server are omitted when nothing is installed.
type Status struct {
	Installed bool    `json:"installed"`
	Engine    string  `json:"engine"`
	Version   *string `json:"version"`
	Service   string  `json:"service,omitempty"`
	Active    *bool   `json:"active,omitempty"`
	Enabled   *bool   `json:"enabled,omitempty"`
	Port      *int    `json:"port,omitempty"`
	Socket    string  `json:"socket,omitempty"`
}

// ParseServerVersion extracts the engine and a clean version from SELECT VERSION()
// output or from `mysqld --version` / `mariadbd --version`.
// MariaDB strings such as 10.11.6-MariaDB-0+deb12u1 become engine mariadb and
// version 10.11.6. MySQL strings such as 8.4.3 and 8.0.39-0ubuntu0.24.04.1
// stay engine mysql with versions 8.4.3 and 8.0.39.
func ParseServerVersion(raw string) (engine, version string, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", false
	}
	version = serverVersionPattern.FindString(raw)
	if version == "" {
		return "", "", false
	}
	engine = string(platform.DatabaseMySQL)
	if strings.Contains(strings.ToLower(raw), "mariadb") {
		engine = string(platform.DatabaseMariaDB)
	}
	return engine, version, true
}

// Status reports installation, engine, version, and service state.
// A missing or stopped server is a normal result and does not return an error.
// Connection failures leave the running fields less detailed.
func (s *Service) Status(ctx context.Context) (*Status, error) {
	provider := platform.Resolve()
	engine := string(provider.DatabaseEngine())
	if !s.databaseInstalled() {
		return &Status{
			Installed: false,
			Engine:    engine,
		}, nil
	}

	st := &Status{
		Installed: true,
		Engine:    engine,
		Service:   provider.DatabaseServiceName(),
	}

	active := false
	svc, err := s.databaseServiceStatus(ctx, st.Service)
	if err != nil || svc == nil {
		st.Active = boolPtr(false)
	} else {
		active = svc.Active == "active"
		st.Active = boolPtr(active)
		if svc.Enabled != "" {
			st.Enabled = boolPtr(unitEnabled(svc.Enabled))
		}
	}

	if active {
		s.readRunningServer(ctx, st)
	}
	if st.Version == nil {
		if raw, err := s.readDaemonVersion(ctx); err == nil {
			applyServerVersion(st, raw)
		}
	}
	s.applyConfigEndpoint(st)
	return st, nil
}

func (s *Service) databaseInstalled() bool {
	seen := make(map[string]struct{})
	for _, name := range []string{
		platform.Resolve().DatabasePackage(""),
		"mysql-server",
		"mariadb-server",
	} {
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		if s.packageInstalled(name) {
			return true
		}
	}
	_, err := s.daemonBinary()
	return err == nil
}

func (s *Service) packageInstalled(name string) bool {
	if s.packageInstalledFn != nil {
		return s.packageInstalledFn(name)
	}
	return pkgmanager.PackageInstalled(name)
}

func (s *Service) daemonBinary() (string, error) {
	if s.daemonBinaryFn != nil {
		return s.daemonBinaryFn()
	}
	return findDaemonBinary()
}

func (s *Service) databaseServiceStatus(ctx context.Context, name string) (*svcmanager.ServiceStatus, error) {
	if s.serviceStatusFn != nil {
		return s.serviceStatusFn(ctx, name)
	}
	return svcmanager.New(false, false).Status(ctx, name)
}

func (s *Service) readRunningServer(ctx context.Context, st *Status) {
	if raw, err := s.query(ctx, "SELECT VERSION()"); err == nil {
		applyServerVersion(st, raw)
	}
	raw, err := s.query(ctx, "SELECT @@port, @@socket")
	if err != nil {
		return
	}
	port, socket, ok := parsePortAndSocket(raw)
	if !ok {
		return
	}
	if port > 0 {
		st.Port = intPtr(port)
	}
	if socket != "" && !strings.EqualFold(socket, "NULL") {
		st.Socket = socket
	}
}

func (s *Service) readDaemonVersion(ctx context.Context) (string, error) {
	binary, err := s.daemonBinary()
	if err != nil {
		return "", err
	}
	if s.daemonVersionFn != nil {
		return s.daemonVersionFn(ctx, binary)
	}
	if s.runner == nil {
		s.runner = executil.New(false, false)
	}
	res, err := s.runner.RunSilent(ctx, binary, "--version")
	text := strings.TrimSpace(res.Stdout)
	if stderr := strings.TrimSpace(res.Stderr); stderr != "" {
		if text != "" {
			text += "\n"
		}
		text += stderr
	}
	if text == "" {
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("empty version output from %s", binary)
	}
	return text, nil
}

func (s *Service) applyConfigEndpoint(st *Status) {
	cfg := s.connectionConfig()
	if st.Port == nil && cfg.Port > 0 {
		st.Port = intPtr(cfg.Port)
	}
	if st.Socket == "" && cfg.Socket != "" {
		st.Socket = cfg.Socket
	}
}

func applyServerVersion(st *Status, raw string) bool {
	engine, version, ok := ParseServerVersion(raw)
	if !ok {
		return false
	}
	st.Engine = engine
	st.Version = stringPtr(version)
	return true
}

func parsePortAndSocket(raw string) (port int, socket string, ok bool) {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "+") {
			continue
		}
		if strings.HasPrefix(line, "|") {
			cells := splitMySQLTableRow(line)
			if len(cells) < 2 {
				continue
			}
			p, err := strconv.Atoi(cells[0])
			if err != nil {
				continue
			}
			return p, cells[1], true
		}

		var parts []string
		if strings.Contains(line, "\t") {
			parts = strings.Split(line, "\t")
		} else {
			parts = strings.Fields(line)
		}
		if len(parts) < 2 {
			continue
		}
		p, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			continue
		}
		return p, strings.TrimSpace(parts[1]), true
	}
	return 0, "", false
}

func splitMySQLTableRow(line string) []string {
	line = strings.Trim(line, "|")
	chunks := strings.Split(line, "|")
	out := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		chunk = strings.TrimSpace(chunk)
		if chunk != "" {
			out = append(out, chunk)
		}
	}
	return out
}

func unitEnabled(state string) bool {
	return state == "enabled" || state == "enabled-runtime"
}

func boolPtr(v bool) *bool { return &v }

func intPtr(v int) *int { return &v }

func stringPtr(v string) *string { return &v }
