package mysql

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigJSONRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mysql.json")

	svc := &Service{
		cfg:        &Config{Host: "127.0.0.1", Port: 3306, User: "root"},
		configPath: path,
	}

	cfg := Config{
		Host:     "db.example.com",
		Port:     3307,
		User:     "app",
		Password: "secret",
		Socket:   "/run/mysqld/mysqld.sock",
	}
	if err := svc.SetConfig(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" {
		t.Fatal("expected config file to be written")
	}

	loaded := &Service{
		cfg:        &Config{Host: "127.0.0.1", Port: 3306, User: "root"},
		configPath: path,
	}
	if err := loaded.loadConfig(); err != nil {
		t.Fatal(err)
	}
	if loaded.cfg.Host != cfg.Host {
		t.Fatalf("host: got %q want %q", loaded.cfg.Host, cfg.Host)
	}
	if loaded.cfg.Port != cfg.Port {
		t.Fatalf("port: got %d want %d", loaded.cfg.Port, cfg.Port)
	}
	if loaded.cfg.User != cfg.User {
		t.Fatalf("user: got %q want %q", loaded.cfg.User, cfg.User)
	}
	if loaded.cfg.Password != cfg.Password {
		t.Fatalf("password: got %q want %q", loaded.cfg.Password, cfg.Password)
	}
	if loaded.cfg.Socket != cfg.Socket {
		t.Fatalf("socket: got %q want %q", loaded.cfg.Socket, cfg.Socket)
	}
}

func TestSaveRootCredentials(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mysql.json")

	svc := &Service{
		cfg:        &Config{Host: "127.0.0.1", Port: 3306, User: "root"},
		configPath: path,
	}

	if err := svc.saveRootCredentials(context.Background(), "install-pass"); err != nil {
		t.Fatal(err)
	}

	loaded := &Service{
		cfg:        &Config{Host: "127.0.0.1", Port: 3306, User: "root"},
		configPath: path,
	}
	if err := loaded.loadConfig(); err != nil {
		t.Fatal(err)
	}
	if loaded.cfg.Password != "install-pass" {
		t.Fatalf("password: got %q want %q", loaded.cfg.Password, "install-pass")
	}
	if loaded.cfg.Host != "127.0.0.1" {
		t.Fatalf("host: got %q want %q", loaded.cfg.Host, "127.0.0.1")
	}
	if loaded.cfg.User != "root" {
		t.Fatalf("user: got %q want %q", loaded.cfg.User, "root")
	}
}

func TestLegacyTOMLMigration(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "mysql.json")
	tomlPath := filepath.Join(dir, "mysql.toml")

	toml := `# Abstrax MySQL config
host = "10.0.0.5"
port = 3308
user = "admin"
password = "legacy"
`
	if err := os.WriteFile(tomlPath, []byte(toml), 0600); err != nil {
		t.Fatal(err)
	}

	svc := &Service{
		cfg:              &Config{Host: "127.0.0.1", Port: 3306, User: "root"},
		configPath:       jsonPath,
		legacyConfigPath: tomlPath,
	}
	if err := svc.loadConfig(); err != nil {
		t.Fatal(err)
	}
	if svc.cfg.Host != "10.0.0.5" {
		t.Fatalf("host: got %q want %q", svc.cfg.Host, "10.0.0.5")
	}
	if svc.cfg.Port != 3308 {
		t.Fatalf("port: got %d want %d", svc.cfg.Port, 3308)
	}
	if svc.cfg.Password != "legacy" {
		t.Fatalf("password: got %q want %q", svc.cfg.Password, "legacy")
	}
	if _, err := os.Stat(jsonPath); err != nil {
		t.Fatalf("expected migrated json config: %v", err)
	}
	if _, err := os.Stat(tomlPath); !os.IsNotExist(err) {
		t.Fatal("expected legacy toml to be removed after migration")
	}
}
