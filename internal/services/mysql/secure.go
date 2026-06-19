package mysql

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	executil "abstrax/internal/exec"
	"abstrax/internal/random"
)

const (
	defaultMySQLSocket   = "/var/run/mysqld/mysqld.sock"
	defaultMySQLPIDFile  = "/var/run/mysqld/mysqld.pid"
	defaultMySQLConfFile = "/etc/mysql/my.cnf"
	defaultMySQLDataDir  = "/var/lib/mysql"
)

// escapeSQLString escapes a string for use inside single-quoted SQL literals.
func escapeSQLString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// buildRootPasswordSQL returns SQL to enable password auth for root over both
// the localhost socket and TCP (127.0.0.1). Explicitly sets caching_sha2_password
// so auth_socket/unix_socket is replaced on Debian/Ubuntu default installs.
func buildRootPasswordSQL(password string) string {
	esc := escapeSQLString(password)
	return fmt.Sprintf(`ALTER USER 'root'@'localhost' IDENTIFIED WITH caching_sha2_password BY '%s';
CREATE USER IF NOT EXISTS 'root'@'127.0.0.1' IDENTIFIED WITH caching_sha2_password BY '%s';
GRANT ALL PRIVILEGES ON *.* TO 'root'@'127.0.0.1' WITH GRANT OPTION;`, esc, esc)
}

// buildSetRootPasswordSQL returns SQL to set root passwords for local connections.
func buildSetRootPasswordSQL(password string) string {
	return fmt.Sprintf("FLUSH PRIVILEGES;\n%s\nFLUSH PRIVILEGES;", buildRootPasswordSQL(password))
}

// buildSecureInstallSQL returns SQL to harden a fresh MySQL installation.
func buildSecureInstallSQL(password string) string {
	return fmt.Sprintf(`%s
DELETE FROM mysql.user WHERE User='';
DROP DATABASE IF EXISTS test;
DELETE FROM mysql.user WHERE User='root' AND Host NOT IN ('localhost', '127.0.0.1', '::1');
FLUSH PRIVILEGES;`, buildRootPasswordSQL(password))
}

func resolveOrGeneratePassword(provided string) (password string, generated bool, err error) {
	if provided != "" {
		return provided, false, nil
	}
	pw, err := random.Password()
	if err != nil {
		return "", false, err
	}
	return pw, true, nil
}

func (s *Service) defaultSocket() string {
	if s.cfg != nil && s.cfg.Socket != "" {
		return s.cfg.Socket
	}
	return discoverMySQLSocket()
}

func discoverMySQLSocket() string {
	for _, path := range []string{
		defaultMySQLSocket,
		"/run/mysqld/mysqld.sock",
	} {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return parseSocketFromMySQLConfig()
}

func parseSocketFromMySQLConfig() string {
	for _, path := range mysqlConfigPaths() {
		if sock := parseSocketInFile(path); sock != "" {
			return sock
		}
	}
	return ""
}

func mysqlConfigPaths() []string {
	var paths []string
	if _, err := os.Stat(defaultMySQLConfFile); err == nil {
		paths = append(paths, defaultMySQLConfFile)
	}
	for _, dir := range []string{
		"/etc/mysql/conf.d",
		"/etc/mysql/mysql.conf.d",
		"/etc/mysql/mariadb.conf.d",
	} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".cnf") {
				continue
			}
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}
	return paths
}

func parseSocketInFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	inClient := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
			inClient = strings.EqualFold(section, "client")
			continue
		}
		if !inClient {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(key) == "socket" {
			return strings.TrimSpace(val)
		}
	}
	return ""
}

type rootConnectStatus int

const (
	rootConnectReady rootConnectStatus = iota
	rootConnectAccessDenied
	rootConnectUnavailable
)

func classifyRootConnectError(detail string) rootConnectStatus {
	lower := strings.ToLower(detail)
	if strings.Contains(lower, "access denied") {
		return rootConnectAccessDenied
	}
	return rootConnectUnavailable
}

func mysqlDataDirExists() bool {
	info, err := os.Stat(filepath.Join(defaultMySQLDataDir, "mysql"))
	return err == nil && info.IsDir()
}

func errMySQLAlreadyConfigured() error {
	if mysqlDataDirExists() {
		return fmt.Errorf("mysql is already configured: database files in %s were kept by `package remove` (only the package was removed, not the data). Use `mysql reset-root-password`, or run `package remove mysql-server --purge` and remove %s for a fresh install", defaultMySQLDataDir, defaultMySQLDataDir)
	}
	return fmt.Errorf("mysql is already configured; use `mysql reset-root-password` if you have lost the root password")
}

func (s *Service) probeRootConnection(ctx context.Context) (rootConnectStatus, string) {
	args := s.rootSocketClientArgs("-e", "SELECT 1")
	res, err := s.runner.RunSilent(ctx, "mysql", args...)
	if err == nil && res.ExitCode == 0 {
		return rootConnectReady, ""
	}

	detail := res.Stderr
	if detail == "" && err != nil {
		detail = err.Error()
	}
	return classifyRootConnectError(detail), detail
}

func (s *Service) canConnectWithoutPassword(ctx context.Context) bool {
	status, _ := s.probeRootConnection(ctx)
	return status == rootConnectReady
}

func (s *Service) waitForReady(ctx context.Context, timeout time.Duration) error {
	var lastDetail string
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		status, detail := s.probeRootConnection(ctx)
		lastDetail = detail
		switch status {
		case rootConnectReady:
			return nil
		case rootConnectAccessDenied:
			return errMySQLAlreadyConfigured()
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}

	return s.readyTimeoutError(ctx, lastDetail)
}

func (s *Service) readyTimeoutError(ctx context.Context, lastDetail string) error {
	var msg strings.Builder
	msg.WriteString("timed out waiting for MySQL to become ready")
	if lastDetail != "" {
		msg.WriteString(": ")
		msg.WriteString(lastDetail)
	}

	if serviceName, err := s.detectServiceName(ctx); err == nil {
		res, _ := s.runner.RunSilent(ctx, "systemctl", "is-active", serviceName)
		state := strings.TrimSpace(res.Stdout)
		if state == "" {
			state = "unknown"
		}
		msg.WriteString(fmt.Sprintf(" (service %q is %s)", serviceName, state))
		if state != "active" {
			msg.WriteString(fmt.Sprintf("; check logs with: journalctl -u %s", serviceName))
		}
	}

	socket := s.defaultSocket()
	if socket != "" {
		msg.WriteString(fmt.Sprintf("; socket: %s", socket))
	}

	return fmt.Errorf(msg.String())
}

func (s *Service) rootSocketClientArgs(extra ...string) []string {
	args := []string{"-u", "root"}
	if sock := s.defaultSocket(); sock != "" {
		args = append(args, "--socket="+sock)
	}
	args = append(args, extra...)
	return args
}

func (s *Service) execAsRootSocket(ctx context.Context, sql string) error {
	args := s.rootSocketClientArgs("-e", sql)
	_, err := s.runner.Run(ctx, "mysql", args...)
	return err
}

func writeTempMySQLClientCnf(cfg Config) (string, error) {
	f, err := os.CreateTemp("", "abstrax-mysql-*.cnf")
	if err != nil {
		return "", fmt.Errorf("creating temp mysql config: %w", err)
	}
	path := f.Name()

	var content strings.Builder
	content.WriteString("[client]\n")
	if cfg.User != "" {
		content.WriteString("user=" + cfg.User + "\n")
	}
	if cfg.Password != "" {
		content.WriteString("password=" + cnfQuoteValue(cfg.Password) + "\n")
	}
	if cfg.Host != "" && cfg.Host != "localhost" {
		content.WriteString("host=" + cfg.Host + "\n")
	}
	if cfg.Port > 0 && cfg.Port != 3306 {
		content.WriteString(fmt.Sprintf("port=%d\n", cfg.Port))
	}
	if cfg.Socket != "" {
		content.WriteString("socket=" + cfg.Socket + "\n")
	}
	if cfg.Database != "" {
		content.WriteString("database=" + cfg.Database + "\n")
	}

	if _, err := f.WriteString(content.String()); err != nil {
		f.Close()
		os.Remove(path)
		return "", fmt.Errorf("writing temp mysql config: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("closing temp mysql config: %w", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("setting temp mysql config permissions: %w", err)
	}
	return path, nil
}

// cnfQuoteValue wraps a value for a MySQL option file, escaping special characters.
func cnfQuoteValue(s string) string {
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

func (s *Service) waitForRootLogin(ctx context.Context, password string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		if err := s.verifyRootLogin(ctx, password); err != nil {
			lastErr = err
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(500 * time.Millisecond):
				continue
			}
		}
		return nil
	}

	if lastErr != nil {
		return fmt.Errorf("timed out waiting for MySQL to accept the new root password: %w", lastErr)
	}
	return fmt.Errorf("timed out waiting for MySQL to accept the new root password")
}

func (s *Service) execWithPassword(ctx context.Context, password, sql string) error {
	cfg := Config{User: "root", Password: password}
	if sock := s.defaultSocket(); sock != "" {
		cfg.Socket = sock
	}
	return s.runMySQL(ctx, cfg, sql, false)
}

func (s *Service) verifyRootLogin(ctx context.Context, password string) error {
	// Verify TCP first: GUI apps and SSH tunnels connect this way. Socket-only
	// checks are not enough because auth_socket can authenticate root without
	// a password when abstrax runs as the root OS user.
	if err := s.execWithPasswordHost(ctx, password, "127.0.0.1", "SELECT 1"); err != nil {
		return fmt.Errorf("verifying root login via 127.0.0.1: %w", err)
	}
	if err := s.execWithPassword(ctx, password, "SELECT 1"); err != nil {
		return fmt.Errorf("verifying root login via socket: %w", err)
	}
	return nil
}

func (s *Service) execWithPasswordHost(ctx context.Context, password, host, sql string) error {
	cfg := Config{User: "root", Password: password, Host: host}
	if s.cfg != nil && s.cfg.Port > 0 {
		cfg.Port = s.cfg.Port
	}
	return s.runMySQL(ctx, cfg, sql, false)
}

// detectServiceName returns the systemd service name for MySQL or MariaDB.
func (s *Service) detectServiceName(ctx context.Context) (string, error) {
	for _, name := range []string{"mysql", "mariadb"} {
		if !executil.SystemctlWorks() {
			break
		}
		res, err := s.runner.RunSilent(ctx, "systemctl", "is-active", name)
		if err == nil && res.ExitCode == 0 {
			return name, nil
		}
		res, err = s.runner.RunSilent(ctx, "systemctl", "list-unit-files", name+".service")
		if err == nil && strings.Contains(res.Stdout, name+".service") {
			return name, nil
		}
	}

	if executil.Exists("mysql") || executil.Exists("mysqld") {
		return "mysql", nil
	}
	if executil.Exists("mariadb") || executil.Exists("mariadbd") {
		return "mariadb", nil
	}
	return "", fmt.Errorf("could not detect mysql or mariadb service")
}

func findDaemonBinary() (string, error) {
	for _, name := range []string{"mysqld", "mariadbd"} {
		if path := executil.Which(name); path != "" {
			return path, nil
		}
	}
	return "", fmt.Errorf("mysqld or mariadbd not found in PATH")
}

func (s *Service) startRecoveryMysqld(ctx context.Context) error {
	daemon, err := findDaemonBinary()
	if err != nil {
		return err
	}

	args := []string{"--skip-grant-tables", "--skip-networking", "--daemonize"}
	if _, err := os.Stat(defaultMySQLConfFile); err == nil {
		args = append([]string{"--defaults-file=" + defaultMySQLConfFile}, args...)
	}
	if sock := s.defaultSocket(); sock != "" {
		args = append(args, "--socket="+sock)
	}

	_, err = s.runner.Run(ctx, daemon, args...)
	if err != nil {
		return fmt.Errorf("starting recovery mysqld: %w", err)
	}
	return s.waitForReady(ctx, 30*time.Second)
}

func (s *Service) stopRecoveryMysqld(ctx context.Context) error {
	args := s.rootSocketClientArgs("-e", "SHUTDOWN")
	if _, err := s.runner.Run(ctx, "mysql", args...); err == nil {
		return nil
	}

	data, err := os.ReadFile(defaultMySQLPIDFile)
	if err != nil {
		return fmt.Errorf("stopping recovery mysqld: %w", err)
	}
	pid := strings.TrimSpace(string(data))
	if pid == "" {
		return fmt.Errorf("stopping recovery mysqld: empty pid file")
	}
	_, err = s.runner.Run(ctx, "kill", pid)
	if err != nil {
		return fmt.Errorf("stopping recovery mysqld: %w", err)
	}
	return nil
}

func dryRunPasswordResult() *RootPasswordResult {
	return &RootPasswordResult{
		RootPassword: "[dry-run]",
		Generated:    true,
	}
}
