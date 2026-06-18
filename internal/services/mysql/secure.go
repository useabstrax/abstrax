package mysql

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	executil "abstrax/internal/exec"
	"abstrax/internal/random"
)

const (
	defaultMySQLSocket   = "/var/run/mysqld/mysqld.sock"
	defaultMySQLPIDFile  = "/var/run/mysqld/mysqld.pid"
	defaultMySQLConfFile = "/etc/mysql/my.cnf"
)

// escapeSQLString escapes a string for use inside single-quoted SQL literals.
func escapeSQLString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// buildSetRootPasswordSQL returns SQL to set the root@localhost password.
func buildSetRootPasswordSQL(password string) string {
	esc := escapeSQLString(password)
	return fmt.Sprintf("FLUSH PRIVILEGES; ALTER USER 'root'@'localhost' IDENTIFIED BY '%s';", esc)
}

// buildSecureInstallSQL returns SQL to harden a fresh MySQL installation.
func buildSecureInstallSQL(password string) string {
	esc := escapeSQLString(password)
	return fmt.Sprintf(`ALTER USER 'root'@'localhost' IDENTIFIED BY '%s';
DELETE FROM mysql.user WHERE User='';
DROP DATABASE IF EXISTS test;
DELETE FROM mysql.user WHERE User='root' AND Host NOT IN ('localhost', '127.0.0.1', '::1');
FLUSH PRIVILEGES;`, esc)
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
	if _, err := os.Stat(defaultMySQLSocket); err == nil {
		return defaultMySQLSocket
	}
	return ""
}

func (s *Service) mysqlClientArgs(extra ...string) []string {
	args := []string{"-u", "root"}
	if sock := s.defaultSocket(); sock != "" {
		args = append(args, "--socket="+sock)
	}
	args = append(args, extra...)
	return args
}

func (s *Service) canConnectWithoutPassword(ctx context.Context) bool {
	args := s.mysqlClientArgs("-e", "SELECT 1")
	res, err := s.runner.RunSilent(ctx, "mysql", args...)
	return err == nil && res.ExitCode == 0
}

func (s *Service) waitForReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if s.canConnectWithoutPassword(ctx) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("timed out waiting for MySQL to become ready")
}

func (s *Service) execAsRootSocket(ctx context.Context, sql string) error {
	args := s.mysqlClientArgs("-e", sql)
	_, err := s.runner.Run(ctx, "mysql", args...)
	return err
}

func writeTempMySQLCnf(password string) (string, error) {
	f, err := os.CreateTemp("", "abstrax-mysql-*.cnf")
	if err != nil {
		return "", fmt.Errorf("creating temp mysql config: %w", err)
	}
	path := f.Name()

	content := fmt.Sprintf("[client]\nuser=root\npassword=%s\n", password)
	if _, err := f.WriteString(content); err != nil {
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

func (s *Service) execWithPassword(ctx context.Context, password, sql string) error {
	cnf, err := writeTempMySQLCnf(password)
	if err != nil {
		return err
	}
	defer os.Remove(cnf)

	args := []string{"--defaults-extra-file=" + cnf}
	if sock := s.defaultSocket(); sock != "" {
		args = append(args, "--socket="+sock)
	}
	args = append(args, "-e", sql)

	_, err = s.runner.Run(ctx, "mysql", args...)
	return err
}

func (s *Service) verifyRootLogin(ctx context.Context, password string) error {
	if err := s.execWithPassword(ctx, password, "SELECT 1"); err != nil {
		return fmt.Errorf("verifying root login: %w", err)
	}
	return nil
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
	args := s.mysqlClientArgs("-e", "SHUTDOWN")
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

// HasSavedPassword reports whether Abstrax has a saved MySQL password in config.
func (s *Service) HasSavedPassword() bool {
	cfg, err := readConfigFile(s.configPath)
	if err != nil {
		return false
	}
	return cfg.Password != ""
}

func dryRunPasswordResult() *RootPasswordResult {
	return &RootPasswordResult{
		RootPassword: "[dry-run]",
		Generated:    true,
	}
}
