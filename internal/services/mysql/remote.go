package mysql

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"abstrax/internal/platform"
	"abstrax/internal/services/svcmanager"
	"abstrax/internal/validate"
)

// remoteDropInName sorts last among included config files so this bind-address wins.
const remoteDropInName = "zz-abstrax-remote.cnf"

const defaultRemoteBind = "0.0.0.0"

// ValidateBindAddress checks that bind is a single IP address.
func ValidateBindAddress(bind string) error {
	if bind == "" {
		return fmt.Errorf("bind address cannot be empty")
	}
	if strings.ContainsAny(bind, " \t\r\n\"'") {
		return fmt.Errorf("bind address cannot contain spaces, quotes, or newlines")
	}
	return validate.IPAddress(bind)
}

// EnableRemote writes the Abstrax bind-address drop-in and restarts the database.
// If the service does not come back, the drop-in is removed and the service is restarted again.
func (s *Service) EnableRemote(ctx context.Context, opts RemoteEnableOptions) (*RemoteStatus, error) {
	bind := opts.Bind
	if bind == "" {
		bind = defaultRemoteBind
	}
	if err := ValidateBindAddress(bind); err != nil {
		return nil, err
	}

	path, err := s.remoteDropInPath()
	if err != nil {
		return nil, err
	}
	status := &RemoteStatus{
		Enabled:     remoteAccessEnabled(bind),
		BindAddress: bind,
		ManagedFile: path,
		Port:        s.mysqlPort(),
	}
	if opts.DryRun {
		return status, nil
	}

	body := remoteDropInContents(bind, databaseProvider().DatabaseEngine())
	if err := writeRemoteDropIn(path, body); err != nil {
		return nil, err
	}
	if err := s.restartAndWait(ctx); err != nil {
		return nil, s.rollbackEnable(ctx, path, err)
	}
	return status, nil
}

// DisableRemote removes the Abstrax bind-address drop-in and restarts the database.
// If the running bind address was not set by that file, nothing is changed.
// If the service does not come back, the drop-in is restored and the service is restarted again.
func (s *Service) DisableRemote(ctx context.Context, opts RemoteDisableOptions) (*RemoteStatus, error) {
	path, err := s.remoteDropInPath()
	if err != nil {
		return nil, err
	}
	if opts.DryRun {
		return &RemoteStatus{
			ManagedFile: path,
			Port:        s.mysqlPort(),
		}, nil
	}

	previous, existed, err := readDropIn(path)
	if err != nil {
		return nil, err
	}

	up, err := s.databaseAccepting(ctx)
	if err != nil {
		return nil, err
	}
	if up {
		live, err := s.queryBindAddress(ctx)
		if err != nil {
			return nil, err
		}
		if !existed {
			if remoteAccessEnabled(live) {
				return nil, fmt.Errorf("bind-address is %s and does not come from Abstrax's file %s; remote access was not turned off", live, path)
			}
			return &RemoteStatus{
				Enabled:     false,
				BindAddress: live,
				ManagedFile: path,
				Port:        s.mysqlPort(),
				Note:        fmt.Sprintf("MySQL is already listening on %s. Abstrax has no remote drop-in to remove.", live),
			}, nil
		}
		fileBind, ok := bindAddressInDropIn(string(previous))
		if !ok || !sameBind(fileBind, live) {
			return nil, fmt.Errorf("bind-address is %s and does not come from Abstrax's file %s; remote access was not turned off", live, path)
		}
	} else if !existed {
		return nil, fmt.Errorf("Abstrax has no remote drop-in at %s, and the live bind address is unknown because the database is not accepting connections; remote access was not turned off", path)
	}

	if err := os.Remove(path); err != nil {
		return nil, fmt.Errorf("removing %s: %w", path, err)
	}
	if err := s.restartAndWait(ctx); err != nil {
		return nil, s.rollbackDisable(ctx, path, previous, err)
	}

	live, qerr := s.queryBindAddress(ctx)
	if qerr != nil {
		return nil, fmt.Errorf("removed %s and restarted the database, but the live bind address is unknown (%v); remote access was not confirmed off", path, qerr)
	}
	if remoteAccessEnabled(live) {
		return nil, fmt.Errorf("removed %s and restarted the database, but bind-address is still %s and does not come from Abstrax's file; remote access was not turned off", path, live)
	}
	return &RemoteStatus{
		Enabled:     false,
		BindAddress: live,
		ManagedFile: path,
		Port:        s.mysqlPort(),
	}, nil
}

// RemoteStatus reports the bind address the running server is using.
// When the server is not accepting connections, the live value is unknown and the drop-in is read instead.
func (s *Service) RemoteStatus(ctx context.Context) (*RemoteStatus, error) {
	path, err := s.remoteDropInPath()
	if err != nil {
		return nil, err
	}
	st := &RemoteStatus{
		ManagedFile: path,
		Port:        s.mysqlPort(),
	}
	up, err := s.databaseAccepting(ctx)
	if err != nil {
		return nil, err
	}
	if !up {
		st.BindAddress = "unknown"
		st.Note = "The database is not accepting connections, so the live bind address is unknown."
		data, readErr := os.ReadFile(path)
		if readErr == nil {
			if bind, ok := bindAddressInDropIn(string(data)); ok {
				st.Note += fmt.Sprintf(" The drop-in sets bind-address = %s.", bind)
			}
		}
		return st, nil
	}
	live, err := s.queryBindAddress(ctx)
	if err != nil {
		return nil, err
	}
	st.BindAddress = live
	st.Enabled = remoteAccessEnabled(live)
	return st, nil
}

func (s *Service) rollbackEnable(ctx context.Context, path string, cause error) error {
	removeErr := os.Remove(path)
	if removeErr != nil && os.IsNotExist(removeErr) {
		removeErr = nil
	}
	restartErr := s.restartAndWait(ctx)
	switch {
	case removeErr != nil && restartErr != nil:
		return fmt.Errorf("database did not come back after enabling remote access (%v); failed to remove %s (%v) and restarting again failed: %w", cause, path, removeErr, restartErr)
	case removeErr != nil:
		return fmt.Errorf("database did not come back after enabling remote access (%v); failed to remove %s: %w", cause, path, removeErr)
	case restartErr != nil:
		return fmt.Errorf("database did not come back after enabling remote access (%v); the drop-in was removed but the service still did not accept connections: %w", cause, restartErr)
	default:
		return fmt.Errorf("database did not come back after enabling remote access; the drop-in was removed and the service was restarted: %w", cause)
	}
}

func (s *Service) rollbackDisable(ctx context.Context, path string, previous []byte, cause error) error {
	writeErr := writeRemoteDropIn(path, string(previous))
	restartErr := s.restartAndWait(ctx)
	switch {
	case writeErr != nil && restartErr != nil:
		return fmt.Errorf("database did not come back after disabling remote access (%v); failed to restore %s (%v) and restarting again failed: %w", cause, path, writeErr, restartErr)
	case writeErr != nil:
		return fmt.Errorf("database did not come back after disabling remote access (%v); failed to restore %s: %w", cause, path, writeErr)
	case restartErr != nil:
		return fmt.Errorf("database did not come back after disabling remote access (%v); the drop-in was restored but the service still did not accept connections: %w", cause, restartErr)
	default:
		return fmt.Errorf("database did not come back after disabling remote access; the drop-in was restored and the service was restarted: %w", cause)
	}
}

func (s *Service) restartAndWait(ctx context.Context) error {
	name := databaseProvider().DatabaseServiceName()
	if s.restartFn != nil {
		if err := s.restartFn(ctx, name); err != nil {
			return err
		}
	} else if err := svcmanager.New(false, false).Restart(ctx, name); err != nil {
		return err
	}
	return s.waitForDatabase(ctx)
}

// waitForDatabase reuses waitForReady. A password challenge means the server
// is accepting connections; waitForReady reports that as an already-configured install.
func (s *Service) waitForDatabase(ctx context.Context) error {
	if s.readyFn != nil {
		return s.readyFn(ctx)
	}
	err := s.waitForReady(ctx, 60*time.Second)
	if err == nil || isMySQLAlreadyConfigured(err) {
		return nil
	}
	return err
}

func (s *Service) databaseAccepting(ctx context.Context) (bool, error) {
	if s.serverUpFn != nil {
		return s.serverUpFn(ctx)
	}
	if s.runner == nil {
		return false, nil
	}
	status, _ := s.probeRootConnection(ctx)
	switch status {
	case rootConnectReady, rootConnectAccessDenied:
		return true, nil
	default:
		return false, nil
	}
}

func (s *Service) queryBindAddress(ctx context.Context) (string, error) {
	out, err := s.query(ctx, "SHOW VARIABLES LIKE 'bind_address'")
	if err != nil {
		return "", err
	}
	return parseBindAddress(out)
}

func (s *Service) remoteDropInPath() (string, error) {
	if s.remoteDirFn != nil {
		dir, err := s.remoteDirFn()
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, remoteDropInName), nil
	}
	dir, err := remoteConfigDir(databaseProvider())
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, remoteDropInName), nil
}

func (s *Service) mysqlPort() int {
	port := s.connectionConfig().Port
	if port == 0 {
		return 3306
	}
	return port
}

func remoteConfigDir(p platform.Provider) (string, error) {
	return chooseRemoteConfigDir(p.DatabaseConfigPaths(), preferRemoteConfigDir(p), pathIsDir)
}

// chooseRemoteConfigDir picks a directory from the provider's config paths.
// A preferred directory that exists wins. Otherwise the first existing directory
// is used. If none exist, the conventional conf.d path is returned.
func chooseRemoteConfigDir(paths []string, prefer func(string) bool, isDir func(string) bool) (string, error) {
	if isDir == nil {
		isDir = pathIsDir
	}
	var conventional string
	var firstExisting string
	for _, path := range paths {
		if path == "" {
			continue
		}
		if prefer != nil && prefer(path) && conventional == "" {
			conventional = path
		}
		if !isDir(path) {
			continue
		}
		if prefer != nil && prefer(path) {
			return path, nil
		}
		if firstExisting == "" {
			firstExisting = path
		}
	}
	if firstExisting != "" {
		return firstExisting, nil
	}
	if conventional != "" {
		return conventional, nil
	}
	return "", fmt.Errorf("no database configuration directory available")
}

func preferRemoteConfigDir(p platform.Provider) func(string) bool {
	switch {
	case p.DatabaseEngine() == platform.DatabaseMySQL:
		return func(path string) bool {
			return strings.HasSuffix(filepath.Clean(path), "mysql.conf.d")
		}
	case p.Profile().Family == "rhel":
		return func(path string) bool {
			return filepath.Clean(path) == "/etc/my.cnf.d"
		}
	default:
		return func(path string) bool {
			return strings.HasSuffix(filepath.Clean(path), "mariadb.conf.d")
		}
	}
}

func pathIsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func remoteDropInContents(bind string, engine platform.DatabaseEngine) string {
	body := "[mysqld]\nbind-address = " + bind + "\n"
	if engine == platform.DatabaseMySQL {
		body += "mysqlx-bind-address = " + bind + "\n"
	}
	return body
}

func writeRemoteDropIn(path, contents string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating database config directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

func readDropIn(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("reading %s: %w", path, err)
	}
	return data, true, nil
}

func bindAddressInDropIn(data string) (string, bool) {
	section := ""
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			continue
		}
		if section != "mysqld" {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(key), "bind-address") {
			return strings.TrimSpace(val), true
		}
	}
	return "", false
}

func parseBindAddress(out string) (string, error) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "+") {
			continue
		}
		fields := strings.Fields(line)
		var vals []string
		for _, field := range fields {
			if field == "|" {
				continue
			}
			vals = append(vals, field)
		}
		if len(vals) < 2 || !strings.EqualFold(vals[0], "bind_address") {
			continue
		}
		return vals[1], nil
	}
	return "", fmt.Errorf("bind_address not found in server response")
}

func remoteAccessEnabled(bind string) bool {
	bind = strings.TrimSpace(bind)
	if bind == "" || bind == "unknown" {
		return false
	}
	for _, part := range strings.Split(bind, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !isLoopbackAddress(part) {
			return true
		}
	}
	return false
}

func isLoopbackAddress(bind string) bool {
	if bind == "127.0.0.1" || bind == "::1" {
		return true
	}
	ip := net.ParseIP(bind)
	return ip != nil && ip.IsLoopback()
}

func sameBind(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if strings.EqualFold(a, b) {
		return true
	}
	ipa := net.ParseIP(a)
	ipb := net.ParseIP(b)
	return ipa != nil && ipb != nil && ipa.Equal(ipb)
}
