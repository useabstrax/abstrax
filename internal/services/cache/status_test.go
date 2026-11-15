package cache

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestParseRedisVersion(t *testing.T) {
	v, ok := parseRedisVersion("Redis server v=7.2.7 sha=00000000:0 malloc=libc bits=64 build=7f24f11dd7e42c58")
	if !ok || v != "7.2.7" {
		t.Fatalf("got %q ok=%v", v, ok)
	}
}

func TestParseMemcachedVersion(t *testing.T) {
	v, ok := parseMemcachedVersion("memcached 1.6.18\n")
	if !ok || v != "1.6.18" {
		t.Fatalf("got %q ok=%v", v, ok)
	}
}

func TestStatusReportsInstalledAndVersion(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "redis.conf")
	if err := os.WriteFile(conf, []byte("port 6380\nbind 127.0.0.1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	origPkg := packageInstalledFn
	origLook := lookPathFn
	origRead := readFileFn
	origRun := runVersionFn
	t.Cleanup(func() {
		packageInstalledFn = origPkg
		lookPathFn = origLook
		readFileFn = origRead
		runVersionFn = origRun
	})

	packageInstalledFn = func(name string) bool { return name == "redis-server" || name == "redis" }
	lookPathFn = func(file string) (string, error) {
		if file == "redis-server" || file == "redis" {
			return "/usr/bin/redis-server", nil
		}
		return "", os.ErrNotExist
	}
	readFileFn = func(name string) ([]byte, error) {
		if filepath.Base(name) == "redis.conf" || name == conf {
			return os.ReadFile(conf)
		}
		return origRead(name)
	}
	runVersionFn = func(context.Context, string, ...string) (string, error) {
		return "Redis server v=7.2.7 sha=00000000:0", nil
	}

	info := &StatusInfo{Driver: DriverRedis}
	version, ok := parseRedisVersion("Redis server v=7.2.7 sha=00000000:0")
	if !ok {
		t.Fatal("expected redis version")
	}
	info.Installed = packageInstalledFn("redis-server")
	info.Version = &version
	port, bind := readRedisEndpoint(conf)
	info.Port = port
	info.Bind = bind

	if !info.Installed || info.Version == nil || *info.Version != "7.2.7" {
		t.Fatalf("status = %+v", info)
	}
	if info.Port != "6380" || info.Bind != "127.0.0.1" {
		t.Fatalf("endpoint = %q %q", info.Port, info.Bind)
	}
}

func TestReadMemcachedEndpointSysconfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memcached")
	body := "PORT=\"11212\"\nUSER=\"memcached\"\nOPTIONS=\"-l 10.0.0.5\"\n"
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	port, bind := readMemcachedEndpoint(path)
	if port != "11212" || bind != "10.0.0.5" {
		t.Fatalf("port=%q bind=%q", port, bind)
	}
}
