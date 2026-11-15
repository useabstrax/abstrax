package cache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadRedisConfigInfo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "redis.conf")
	body := "port 6379\nbind 127.0.0.1\nmaxmemory 256mb\nmaxmemory-policy allkeys-lru\nrequirepass secret\n"
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	info, err := readRedisConfigInfo(path, DriverRedis)
	if err != nil {
		t.Fatal(err)
	}
	if info.Port != "6379" || info.Bind != "127.0.0.1" || info.MaxMemory != "256mb" {
		t.Fatalf("info = %+v", info)
	}
	if info.MaxMemoryPolicy != "allkeys-lru" || !info.RequirePassSet {
		t.Fatalf("policy/pass = %+v", info)
	}
}

func TestApplyRedisConfigSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "redis.conf")
	if err := os.WriteFile(path, []byte("port 6379\nbind 127.0.0.1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	err := applyRedisConfigSettings(path, InstallOptions{
		Port:            6380,
		Bind:            "10.0.0.1",
		Memory:          "128mb",
		MaxMemoryPolicy: "volatile-lru",
		Password:        "s3cret",
	}, false)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, want := range []string{
		"port 6380",
		"bind 10.0.0.1",
		"maxmemory 128mb",
		"maxmemory-policy volatile-lru",
		"requirepass s3cret",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in:\n%s", want, body)
		}
	}

	if err := applyRedisConfigSettings(path, InstallOptions{}, true); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "requirepass") {
		t.Fatalf("requirepass still present:\n%s", data)
	}
}
