package cache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetConfigLine(t *testing.T) {
	lines := []string{
		"# comment",
		"port 6379",
		"bind 127.0.0.1",
	}
	got := setConfigLine(lines, "port", "6380")
	if !strings.Contains(strings.Join(got, "\n"), "port 6380") {
		t.Fatalf("expected updated port, got %#v", got)
	}
}

func TestSetConfigLineAppendsMissing(t *testing.T) {
	got := setConfigLine([]string{"# redis"}, "maxmemory", "256mb")
	if got[len(got)-1] != "maxmemory 256mb" {
		t.Fatalf("expected appended line, got %#v", got)
	}
}

func TestApplyRedisConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "redis.conf")
	if err := os.WriteFile(path, []byte("port 6379\nbind 127.0.0.1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	orig := redisConfigPath
	redisConfigPath = path
	t.Cleanup(func() { redisConfigPath = orig })

	if err := applyRedisConfig(InstallOptions{
		Driver: DriverRedis,
		Port:   6380,
		Bind:   "10.0.0.1",
		Memory: "128mb",
	}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{"port 6380", "bind 10.0.0.1", "maxmemory 128mb"} {
		if !strings.Contains(content, want) {
			t.Fatalf("missing %q in:\n%s", want, content)
		}
	}
}

func TestApplyMemcachedConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memcached.conf")
	if err := os.WriteFile(path, []byte("-m 64\n-p 11211\n-l 127.0.0.1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	orig := memcachedConfigPath
	memcachedConfigPath = path
	t.Cleanup(func() { memcachedConfigPath = orig })

	if err := applyMemcachedConfig(InstallOptions{
		Driver: DriverMemcached,
		Port:   11212,
		Bind:   "10.0.0.2",
		Memory: "128",
	}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{"-p 11212", "-l 10.0.0.2", "-m 128"} {
		if !strings.Contains(content, want) {
			t.Fatalf("missing %q in:\n%s", want, content)
		}
	}
}
