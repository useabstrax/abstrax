// Package mysql manages MySQL/MariaDB databases and users.
package mysql

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	executil "abstrax/internal/exec"
	"abstrax/internal/platform/debian"
	"abstrax/internal/services/pkgmanager"
	"abstrax/internal/services/svcmanager"
)

// Service manages MySQL operations.
type Service struct {
	runner           *executil.Runner
	cfg              *Config
	configPath       string
	legacyConfigPath string
}

// New creates a Service.
func New(dryRun, verbose bool) *Service {
	svc := &Service{
		runner:           executil.New(dryRun, verbose),
		cfg:              &Config{Host: "127.0.0.1", Port: 3306, User: "root"},
		configPath:       debian.MySQLConfig,
		legacyConfigPath: debian.MySQLConfigLegacy,
	}
	_ = svc.loadConfig()
	return svc
}

// SetConfig saves MySQL connection config.
func (s *Service) SetConfig(_ context.Context, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(s.configPath), 0750); err != nil {
		return err
	}

	data, err := json.MarshalIndent(configToFile(cfg), "", "  ")
	if err != nil {
		return fmt.Errorf("encoding mysql config: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(s.configPath, data, 0600); err != nil {
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
	res, err := s.runMySQLQuery(ctx, s.connectionConfig(), "SELECT 1")
	if err != nil {
		if res.Stderr != "" {
			return fmt.Errorf("mysql connection failed: %s", res.Stderr)
		}
		return fmt.Errorf("mysql connection failed: %w", err)
	}
	return nil
}

// Install installs MySQL/MariaDB, applies secure defaults, and sets the root password.
func (s *Service) Install(ctx context.Context, opts InstallOptions) (*RootPasswordResult, error) {
	if opts.DryRun {
		return dryRunPasswordResult(), nil
	}

	mgr := pkgmanager.NewApt(false, false)
	svcMgr := svcmanager.New(false, false)

	pkg := "mysql-server"
	if opts.Version != "" {
		pkg = fmt.Sprintf("mysql-server-%s", opts.Version)
	}

	if err := mgr.Update(ctx); err != nil {
		return nil, err
	}
	if err := mgr.Install(ctx, pkgmanager.InstallOptions{Name: pkg}); err != nil {
		return nil, fmt.Errorf("installing mysql: %w", err)
	}

	serviceName, err := s.detectServiceName(ctx)
	if err != nil {
		return nil, err
	}
	if err := svcMgr.Enable(ctx, serviceName); err != nil {
		return nil, err
	}
	if err := svcMgr.Start(ctx, serviceName); err != nil {
		return nil, err
	}

	if err := s.waitForReady(ctx, 120*time.Second); err != nil {
		return nil, err
	}

	password, generated, err := resolveOrGeneratePassword(opts.RootPassword)
	if err != nil {
		return nil, err
	}

	if err := s.execAsRootSocket(ctx, buildSecureInstallSQL(password)); err != nil {
		return nil, fmt.Errorf("securing mysql installation: %w", err)
	}

	if err := s.verifyRootLogin(ctx, password); err != nil {
		return nil, err
	}

	if err := s.saveRootCredentials(ctx, password); err != nil {
		return nil, fmt.Errorf("saving mysql config: %w", err)
	}

	return &RootPasswordResult{
		RootPassword: password,
		Generated:    generated,
	}, nil
}

// ResetRootPassword resets the MySQL root password without knowing the current one.
func (s *Service) ResetRootPassword(ctx context.Context, opts ResetRootPasswordOptions) (*RootPasswordResult, error) {
	if opts.DryRun {
		return dryRunPasswordResult(), nil
	}

	serviceName, err := s.detectServiceName(ctx)
	if err != nil {
		return nil, err
	}

	svcMgr := svcmanager.New(false, false)
	if err := svcMgr.Stop(ctx, serviceName); err != nil {
		return nil, err
	}

	if err := s.startRecoveryMysqld(ctx); err != nil {
		return nil, err
	}

	password, generated, err := resolveOrGeneratePassword(opts.RootPassword)
	if err != nil {
		_ = s.stopRecoveryMysqld(ctx)
		return nil, err
	}

	if err := s.execAsRootSocket(ctx, buildSetRootPasswordSQL(password)); err != nil {
		_ = s.stopRecoveryMysqld(ctx)
		return nil, fmt.Errorf("setting root password: %w", err)
	}

	if err := s.stopRecoveryMysqld(ctx); err != nil {
		return nil, err
	}

	if err := svcMgr.Start(ctx, serviceName); err != nil {
		return nil, err
	}

	if err := s.waitForRootLogin(ctx, password, 60*time.Second); err != nil {
		return nil, err
	}

	if err := s.saveRootCredentials(ctx, password); err != nil {
		return nil, fmt.Errorf("saving mysql config: %w", err)
	}

	return &RootPasswordResult{
		RootPassword: password,
		Generated:    generated,
	}, nil
}

// saveRootCredentials stores the root password in Abstrax MySQL config so
// subsequent mysql commands can connect without running config set manually.
func (s *Service) saveRootCredentials(ctx context.Context, password string) error {
	cfg := Config{Host: "127.0.0.1", Port: 3306, User: "root", Password: password}
	if s.cfg != nil {
		if s.cfg.Host != "" {
			cfg.Host = s.cfg.Host
		}
		if s.cfg.Port != 0 {
			cfg.Port = s.cfg.Port
		}
		if s.cfg.User != "" {
			cfg.User = s.cfg.User
		}
		cfg.Socket = s.cfg.Socket
		cfg.Database = s.cfg.Database
	}
	return s.SetConfig(ctx, cfg)
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

func localAppHosts() []string {
	return []string{"localhost", "127.0.0.1"}
}

func usesLocalAppHosts(host string) bool {
	return host == "" || host == "localhost"
}

// UserAdd creates a MySQL user.
func (s *Service) UserAdd(ctx context.Context, opts UserAddOptions) error {
	host := opts.Host
	if host == "" {
		host = "localhost"
	}

	hosts := []string{host}
	if usesLocalAppHosts(host) {
		hosts = localAppHosts()
	}

	for _, h := range hosts {
		createSQL := fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%s' IDENTIFIED BY '%s'",
			opts.Name, h, escapeSQLString(opts.Password))
		if err := s.exec(ctx, createSQL); err != nil {
			return err
		}
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

		for _, h := range hosts {
			grantSQL := fmt.Sprintf("GRANT %s ON `%s`.* TO '%s'@'%s'",
				privs, opts.GrantDB, opts.Name, h)
			if err := s.exec(ctx, grantSQL); err != nil {
				return err
			}
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
	// Try common host values: localhost first (default for most setups),
	// then the wildcard '%'.
	var res string
	var err error
	var matchedHost string
	for _, host := range []string{"localhost", "127.0.0.1", "%"} {
		res, err = s.query(ctx,
			fmt.Sprintf("SHOW GRANTS FOR '%s'@'%s'", name, host))
		if err == nil {
			matchedHost = host
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("mysql user %q not found: %w", name, err)
	}

	info := &UserInfo{Name: name, Host: matchedHost}
	for _, line := range strings.Split(res, "\n") {
		if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "Grants") {
			info.Grants = append(info.Grants, strings.TrimSpace(line))
		}
	}
	return info, nil
}

// Grant grants privileges on a database to a user.
func (s *Service) Grant(ctx context.Context, user, database, host, privileges string) error {
	if privileges == "" {
		privileges = PresetPrivileges[PresetApp]
	}

	hosts, err := s.grantTargetHosts(ctx, user, host)
	if err != nil {
		return err
	}

	for _, h := range hosts {
		sql := fmt.Sprintf("GRANT %s ON `%s`.* TO '%s'@'%s'",
			privileges, database, user, h)
		if err := s.exec(ctx, sql); err != nil {
			return err
		}
	}
	return s.exec(ctx, "FLUSH PRIVILEGES")
}

// Revoke revokes all privileges from a user on a database.
func (s *Service) Revoke(ctx context.Context, user, database, host string) error {
	hosts, err := s.grantTargetHosts(ctx, user, host)
	if err != nil {
		return err
	}

	for _, h := range hosts {
		sql := fmt.Sprintf("REVOKE ALL PRIVILEGES ON `%s`.* FROM '%s'@'%s'",
			database, user, h)
		if err := s.exec(ctx, sql); err != nil {
			return err
		}
	}
	return s.exec(ctx, "FLUSH PRIVILEGES")
}

func (s *Service) grantTargetHosts(ctx context.Context, user, host string) ([]string, error) {
	if host != "" {
		return []string{host}, nil
	}

	if err := s.ensureLocalAppUserHosts(ctx, user); err != nil {
		return nil, err
	}

	existing, err := s.userHosts(ctx, user)
	if err != nil {
		return nil, err
	}

	have := make(map[string]bool, len(existing))
	for _, h := range existing {
		have[h] = true
	}

	var targets []string
	for _, h := range localAppHosts() {
		if have[h] {
			targets = append(targets, h)
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("mysql user %q has no localhost or 127.0.0.1 account; use --host to grant a specific host", user)
	}
	return targets, nil
}

func (s *Service) ensureLocalAppUserHosts(ctx context.Context, user string) error {
	existing, err := s.userHosts(ctx, user)
	if err != nil {
		return err
	}

	have := make(map[string]bool, len(existing))
	for _, h := range existing {
		have[h] = true
	}

	switch {
	case !have["127.0.0.1"] && have["localhost"]:
		sql := fmt.Sprintf(
			"CREATE USER IF NOT EXISTS '%s'@'127.0.0.1' IDENTIFIED WITH caching_sha2_password AS '%s'@'localhost'",
			user, user,
		)
		if err := s.exec(ctx, sql); err != nil {
			return fmt.Errorf("creating %q@127.0.0.1 from localhost account: %w", user, err)
		}
	case !have["localhost"] && have["127.0.0.1"]:
		sql := fmt.Sprintf(
			"CREATE USER IF NOT EXISTS '%s'@'localhost' IDENTIFIED WITH caching_sha2_password AS '%s'@'127.0.0.1'",
			user, user,
		)
		if err := s.exec(ctx, sql); err != nil {
			return fmt.Errorf("creating %q@localhost from 127.0.0.1 account: %w", user, err)
		}
	}

	return nil
}

func (s *Service) userHosts(ctx context.Context, user string) ([]string, error) {
	res, err := s.query(ctx,
		fmt.Sprintf("SELECT Host FROM mysql.user WHERE User = '%s' ORDER BY Host",
			escapeSQLString(user)))
	if err != nil {
		return nil, err
	}

	var hosts []string
	scanner := bufio.NewScanner(strings.NewReader(res))
	first := true
	for scanner.Scan() {
		if first {
			first = false
			continue
		}
		h := strings.TrimSpace(scanner.Text())
		if h != "" {
			hosts = append(hosts, h)
		}
	}
	if len(hosts) == 0 {
		return nil, fmt.Errorf("mysql user %q not found", user)
	}
	return hosts, nil
}

func (s *Service) connectionConfig() Config {
	if s.cfg == nil {
		return Config{Host: "127.0.0.1", Port: 3306, User: "root"}
	}
	return *s.cfg
}

func (s *Service) clientArgsWithoutPassword(cfg Config) []string {
	args := []string{"-u", cfg.User}
	if cfg.Host != "" && cfg.Host != "localhost" {
		args = append(args, "-h", cfg.Host)
	}
	if cfg.Port > 0 && cfg.Port != 3306 {
		args = append(args, fmt.Sprintf("--port=%d", cfg.Port))
	}
	if cfg.Socket != "" {
		args = append(args, fmt.Sprintf("--socket=%s", cfg.Socket))
	}
	return args
}

func (s *Service) configuredClientArgs(cfg Config) (args []string, cleanup func(), err error) {
	if cfg.Password != "" {
		cnf, err := writeTempMySQLClientCnf(cfg)
		if err != nil {
			return nil, nil, err
		}
		return []string{"--defaults-extra-file=" + cnf}, func() { os.Remove(cnf) }, nil
	}
	return s.clientArgsWithoutPassword(cfg), func() {}, nil
}

func (s *Service) runMySQL(ctx context.Context, cfg Config, sql string, silent bool) error {
	res, err := s.runMySQLWithConfig(ctx, cfg, sql, silent)
	if err != nil && res.Stderr != "" {
		return fmt.Errorf("%w: %s", err, res.Stderr)
	}
	return err
}

func (s *Service) runMySQLQuery(ctx context.Context, cfg Config, sql string) (executil.Result, error) {
	return s.runMySQLWithConfig(ctx, cfg, sql, true)
}

func (s *Service) runMySQLWithConfig(ctx context.Context, cfg Config, sql string, silent bool) (executil.Result, error) {
	args, cleanup, err := s.configuredClientArgs(cfg)
	if err != nil {
		return executil.Result{}, err
	}
	defer cleanup()

	args = append(args, "-e", sql)
	if silent {
		return s.runner.RunSilent(ctx, "mysql", args...)
	}
	return s.runner.Run(ctx, "mysql", args...)
}

func (s *Service) exec(ctx context.Context, sql string) error {
	return s.runMySQL(ctx, s.connectionConfig(), sql, false)
}

func (s *Service) query(ctx context.Context, sql string) (string, error) {
	res, err := s.runMySQLQuery(ctx, s.connectionConfig(), sql)
	if err != nil {
		if res.Stderr != "" {
			return "", fmt.Errorf("%w: %s", err, res.Stderr)
		}
		return "", err
	}
	return res.Stdout, nil
}

func (s *Service) loadConfig() error {
	cfg, err := readConfigFile(s.configPath)
	if err == nil {
		s.cfg = cfg
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}

	legacy, err := readLegacyTOMLConfig(s.legacyConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no config file yet; use defaults
		}
		return err
	}

	data, err := json.MarshalIndent(configToFile(*legacy), "", "  ")
	if err != nil {
		return fmt.Errorf("migrating mysql config: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(s.configPath), 0750); err != nil {
		return err
	}
	if err := os.WriteFile(s.configPath, data, 0600); err != nil {
		return fmt.Errorf("migrating mysql config: %w", err)
	}
	_ = os.Remove(s.legacyConfigPath)

	s.cfg = legacy
	return nil
}

func readConfigFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var stored fileConfig
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("parsing mysql config: %w", err)
	}
	return configFromFile(stored), nil
}

type fileConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password,omitempty"`
	Socket   string `json:"socket,omitempty"`
	Database string `json:"database,omitempty"`
}

func configToFile(cfg Config) fileConfig {
	return fileConfig{
		Host:     cfg.Host,
		Port:     cfg.Port,
		User:     cfg.User,
		Password: cfg.Password,
		Socket:   cfg.Socket,
		Database: cfg.Database,
	}
}

func configFromFile(stored fileConfig) *Config {
	cfg := &Config{
		Host:     stored.Host,
		Port:     stored.Port,
		User:     stored.User,
		Password: stored.Password,
		Socket:   stored.Socket,
		Database: stored.Database,
	}
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.User == "" {
		cfg.User = "root"
	}
	if cfg.Port == 0 {
		cfg.Port = 3306
	}
	return cfg
}

func readLegacyTOMLConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &Config{Host: "127.0.0.1", Port: 3306, User: "root"}
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
			cfg.Host = val
		case "port":
			if port, err := strconv.Atoi(val); err == nil {
				cfg.Port = port
			}
		case "user":
			cfg.User = val
		case "password":
			cfg.Password = val
		case "socket":
			cfg.Socket = val
		case "database":
			cfg.Database = val
		}
	}
	return cfg, nil
}
