// Package mysql manages MySQL/MariaDB databases and users.
package mysql

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	executil "abstrax/internal/exec"
	"abstrax/internal/platform/debian"
	"abstrax/internal/services/pkgmanager"
)

// Service manages MySQL operations.
type Service struct {
	runner *executil.Runner
	cfg    *Config
}

// New creates a Service.
func New(dryRun, verbose bool) *Service {
	svc := &Service{
		runner: executil.New(dryRun, verbose),
		cfg:    &Config{Host: "127.0.0.1", Port: 3306, User: "root"},
	}
	_ = svc.loadConfig()
	return svc
}

// SetConfig saves MySQL connection config.
func (s *Service) SetConfig(_ context.Context, cfg Config) error {
	if err := os.MkdirAll(debian.AbstraxConfigDir, 0750); err != nil {
		return err
	}

	var sb strings.Builder
	sb.WriteString("# Abstrax MySQL config – root readable only\n")
	sb.WriteString(fmt.Sprintf("host = %q\n", cfg.Host))
	sb.WriteString(fmt.Sprintf("port = %d\n", cfg.Port))
	sb.WriteString(fmt.Sprintf("user = %q\n", cfg.User))
	if cfg.Password != "" {
		sb.WriteString(fmt.Sprintf("password = %q\n", cfg.Password))
	}
	if cfg.Socket != "" {
		sb.WriteString(fmt.Sprintf("socket = %q\n", cfg.Socket))
	}
	if cfg.Database != "" {
		sb.WriteString(fmt.Sprintf("database = %q\n", cfg.Database))
	}

	if err := os.WriteFile(debian.MySQLConfig, []byte(sb.String()), 0600); err != nil {
		return fmt.Errorf("writing mysql config: %w", err)
	}
	s.cfg = &cfg
	return nil
}

// ShowConfig returns the current saved config.
func (s *Service) ShowConfig(_ context.Context) (*Config, error) {
	return s.cfg, nil
}

// Test tests the MySQL connection.
func (s *Service) Test(ctx context.Context) error {
	args := s.clientArgs()
	args = append(args, "-e", "SELECT 1")
	_, err := s.runner.Run(ctx, "mysql", args...)
	if err != nil {
		return fmt.Errorf("mysql connection failed: %w", err)
	}
	return nil
}

// Install installs MySQL/MariaDB.
func (s *Service) Install(ctx context.Context, opts InstallOptions) error {
	mgr := pkgmanager.NewApt(false, false)

	pkg := "mysql-server"
	if opts.Version != "" {
		pkg = fmt.Sprintf("mysql-server-%s", opts.Version)
	}

	if err := mgr.Update(ctx); err != nil {
		return err
	}
	if err := mgr.Install(ctx, pkgmanager.InstallOptions{Name: pkg}); err != nil {
		return fmt.Errorf("installing mysql: %w", err)
	}

	if opts.Secure {
		// Run mysql_secure_installation non-interactively is complex –
		// TODO: implement secure setup with predefined options.
		fmt.Println("NOTE: Run 'mysql_secure_installation' manually to harden the installation.")
	}

	return nil
}

// DBAdd creates a database.
func (s *Service) DBAdd(ctx context.Context, opts DBAddOptions) error {
	charset := opts.Charset
	if charset == "" {
		charset = "utf8mb4"
	}
	collation := opts.Collation
	if collation == "" {
		collation = "utf8mb4_unicode_ci"
	}

	ifNotExists := ""
	if opts.IfNotExists {
		ifNotExists = "IF NOT EXISTS "
	}

	sql := fmt.Sprintf("CREATE DATABASE %s`%s` CHARACTER SET %s COLLATE %s",
		ifNotExists, opts.Name, charset, collation)

	return s.exec(ctx, sql)
}

// DBRemove drops a database.
func (s *Service) DBRemove(ctx context.Context, name string) error {
	return s.exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", name))
}

// DBList lists databases.
func (s *Service) DBList(ctx context.Context) ([]Database, error) {
	res, err := s.query(ctx, "SHOW DATABASES")
	if err != nil {
		return nil, err
	}

	var dbs []Database
	for _, line := range strings.Split(res, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "Database" {
			continue
		}
		// Skip system databases.
		if line == "information_schema" || line == "performance_schema" ||
			line == "mysql" || line == "sys" {
			continue
		}
		dbs = append(dbs, Database{Name: line})
	}
	return dbs, nil
}

// UserAdd creates a MySQL user.
func (s *Service) UserAdd(ctx context.Context, opts UserAddOptions) error {
	host := opts.Host
	if host == "" {
		host = "localhost"
	}

	createSQL := fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%s' IDENTIFIED BY '%s'",
		opts.Name, host, opts.Password)
	if err := s.exec(ctx, createSQL); err != nil {
		return err
	}

	if opts.GrantDB != "" {
		privs := opts.Privileges
		if privs == "" && opts.Preset != "" {
			var ok bool
			privs, ok = PresetPrivileges[opts.Preset]
			if !ok {
				return fmt.Errorf("unknown privilege preset %q", opts.Preset)
			}
		}
		if privs == "" {
			privs = PresetPrivileges[PresetApp]
		}

		grantSQL := fmt.Sprintf("GRANT %s ON `%s`.* TO '%s'@'%s'",
			privs, opts.GrantDB, opts.Name, host)
		if err := s.exec(ctx, grantSQL); err != nil {
			return err
		}
		return s.exec(ctx, "FLUSH PRIVILEGES")
	}

	return nil
}

// UserRemove drops a MySQL user.
func (s *Service) UserRemove(ctx context.Context, name, host string) error {
	if host == "" {
		host = "localhost"
	}
	return s.exec(ctx, fmt.Sprintf("DROP USER IF EXISTS '%s'@'%s'", name, host))
}

// UserList lists MySQL users.
func (s *Service) UserList(ctx context.Context) ([]UserInfo, error) {
	res, err := s.query(ctx, "SELECT User, Host FROM mysql.user ORDER BY User")
	if err != nil {
		return nil, err
	}

	var users []UserInfo
	scanner := bufio.NewScanner(strings.NewReader(res))
	first := true
	for scanner.Scan() {
		if first {
			first = false
			continue // skip header
		}
		parts := strings.Split(scanner.Text(), "\t")
		if len(parts) < 2 {
			continue
		}
		users = append(users, UserInfo{Name: parts[0], Host: parts[1]})
	}
	return users, nil
}

// UserInfo returns info about a MySQL user.
func (s *Service) UserInfo(ctx context.Context, name string) (*UserInfo, error) {
	res, err := s.query(ctx,
		fmt.Sprintf("SHOW GRANTS FOR '%s'@'%%'", name))
	if err != nil {
		return nil, fmt.Errorf("mysql user %q not found: %w", name, err)
	}

	info := &UserInfo{Name: name}
	for _, line := range strings.Split(res, "\n") {
		if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "Grants") {
			info.Grants = append(info.Grants, strings.TrimSpace(line))
		}
	}
	return info, nil
}

// Grant grants privileges on a database to a user.
func (s *Service) Grant(ctx context.Context, user, database, privileges string) error {
	if privileges == "" {
		privileges = PresetPrivileges[PresetApp]
	}
	sql := fmt.Sprintf("GRANT %s ON `%s`.* TO '%s'@'localhost'; FLUSH PRIVILEGES",
		privileges, database, user)
	return s.exec(ctx, sql)
}

// Revoke revokes all privileges from a user on a database.
func (s *Service) Revoke(ctx context.Context, user, database string) error {
	sql := fmt.Sprintf("REVOKE ALL PRIVILEGES ON `%s`.* FROM '%s'@'localhost'; FLUSH PRIVILEGES",
		database, user)
	return s.exec(ctx, sql)
}

func (s *Service) clientArgs() []string {
	args := []string{
		"-u", s.cfg.User,
	}
	if s.cfg.Password != "" {
		args = append(args, fmt.Sprintf("-p%s", s.cfg.Password))
	}
	if s.cfg.Host != "" && s.cfg.Host != "localhost" {
		args = append(args, "-h", s.cfg.Host)
	}
	if s.cfg.Port > 0 && s.cfg.Port != 3306 {
		args = append(args, fmt.Sprintf("--port=%d", s.cfg.Port))
	}
	if s.cfg.Socket != "" {
		args = append(args, fmt.Sprintf("--socket=%s", s.cfg.Socket))
	}
	return args
}

func (s *Service) exec(ctx context.Context, sql string) error {
	args := s.clientArgs()
	args = append(args, "-e", sql)
	_, err := s.runner.Run(ctx, "mysql", args...)
	return err
}

func (s *Service) query(ctx context.Context, sql string) (string, error) {
	args := s.clientArgs()
	args = append(args, "-e", sql)
	res, err := s.runner.RunSilent(ctx, "mysql", args...)
	if err != nil {
		return "", err
	}
	return res.Stdout, nil
}

func (s *Service) loadConfig() error {
	data, err := os.ReadFile(debian.MySQLConfig)
	if err != nil {
		return nil // no config file yet; use defaults
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		kv := strings.SplitN(line, " = ", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.Trim(strings.TrimSpace(kv[1]), `"`)
		switch key {
		case "host":
			s.cfg.Host = val
		case "user":
			s.cfg.User = val
		case "password":
			s.cfg.Password = val
		case "socket":
			s.cfg.Socket = val
		case "database":
			s.cfg.Database = val
		}
	}
	return nil
}
