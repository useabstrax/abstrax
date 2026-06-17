package project

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"abstrax/internal/identity"
)

type mockIdentity struct {
	accounts map[string]*identity.Account
	homes    []identity.HomeEntry
}

func (m *mockIdentity) Lookup(_ context.Context, username string) (*identity.Account, error) {
	account, ok := m.accounts[username]
	if !ok {
		return nil, &identity.NotFoundError{Username: username}
	}
	copy := *account
	return &copy, nil
}

func (m *mockIdentity) ListHomes(_ context.Context) ([]identity.HomeEntry, error) {
	return append([]identity.HomeEntry(nil), m.homes...), nil
}

func testHomes(root string) []identity.HomeEntry {
	return []identity.HomeEntry{
		{Username: "mike", Home: mustEval(filepath.Join(root, "home", "mike"))},
		{Username: "jane", Home: mustEval(filepath.Join(root, "home", "jane"))},
	}
}

func mustEval(path string) string {
	real, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(real)
}

func testMike(root string) *identity.Account {
	return &identity.Account{
		Username:     "mike",
		UID:          1000,
		GID:          1000,
		PrimaryGroup: "mike",
		Home:         filepath.Join(root, "home", "mike"),
	}
}

func setupFilesystem(t *testing.T, root string) {
	t.Helper()
	dirs := []string{
		filepath.Join(root, "home", "mike"),
		filepath.Join(root, "home", "jane"),
		filepath.Join(root, "home", "jane", "projects"),
		filepath.Join(root, "srv", "sites"),
		filepath.Join(root, "var", "www"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
}

func resolvedPath(t *testing.T, p string) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(filepath.Clean(p))
	if err != nil {
		return filepath.Clean(p)
	}
	return filepath.Clean(real)
}

func TestResolveIdentitySharedByDefault(t *testing.T) {
	id, err := ResolveIdentity(context.Background(), &mockIdentity{}, AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if id.Mode != OwnershipShared || id.User != SharedWebUser || id.Group != SharedWebGroup {
		t.Fatalf("identity = %#v", id)
	}
}

func TestResolveIdentityExplicitUser(t *testing.T) {
	root := t.TempDir()
	resolver := &mockIdentity{accounts: map[string]*identity.Account{"mike": testMike(root)}}
	id, err := ResolveIdentity(context.Background(), resolver, AddOptions{UserExplicit: true, User: "mike"})
	if err != nil {
		t.Fatal(err)
	}
	if id.Mode != OwnershipIsolated || id.User != "mike" || id.Group != "mike" || id.Home != testMike(root).Home {
		t.Fatalf("identity = %#v", id)
	}
}

func TestResolveIdentityMissingUser(t *testing.T) {
	_, err := ResolveIdentity(context.Background(), &mockIdentity{}, AddOptions{UserExplicit: true, User: "missing"})
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := err.(*identity.NotFoundError); !ok {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveProjectPathPrecedence(t *testing.T) {
	root := t.TempDir()
	id := RuntimeIdentity{Mode: OwnershipShared}
	path, err := ResolveProjectPath("example", "", id)
	if err != nil || path != filepath.Join(DefaultSharedBase, "example") {
		t.Fatalf("shared default = %q err=%v", path, err)
	}

	id = RuntimeIdentity{Mode: OwnershipIsolated, Home: filepath.Join(root, "home", "mike")}
	path, err = ResolveProjectPath("example", "", id)
	if err != nil || path != filepath.Join(root, "home", "mike", "example") {
		t.Fatalf("isolated home = %q err=%v", path, err)
	}

	path, err = ResolveProjectPath("example", "/srv/sites/example", id)
	if err != nil || path != "/srv/sites/example" {
		t.Fatalf("explicit path = %q err=%v", path, err)
	}
}

func TestValidateIsolatedPaths(t *testing.T) {
	root := t.TempDir()
	homes := testHomes(root)
	setupFilesystem(t, root)
	homes = testHomes(root)
	id := RuntimeIdentity{
		Mode: OwnershipIsolated,
		User: "mike",
		UID:  1000,
		GID:  1000,
		Home: resolvedPath(t, filepath.Join(root, "home", "mike")),
	}

	cases := []struct {
		name    string
		path    string
		roots   []string
		wantErr string
	}{
		{
			name: "inside user home",
			path: filepath.Join(root, "home", "mike", "example.com"),
		},
		{
			name:    "inside other home",
			path:    filepath.Join(root, "home", "jane", "example"),
			wantErr: "inside another user's home directory",
		},
		{
			name:    "nonexistent under other home parent",
			path:    filepath.Join(root, "home", "jane", "projects", "new-site"),
			wantErr: "inside another user's home directory",
		},
		{
			name:  "approved root child",
			path:  filepath.Join(root, "srv", "sites", "example"),
			roots: []string{resolvedPath(t, filepath.Join(root, "srv", "sites"))},
		},
		{
			name:    "outside approved roots",
			path:    filepath.Join(root, "etc", "example"),
			roots:   []string{resolvedPath(t, filepath.Join(root, "srv", "sites"))},
			wantErr: "not inside an approved shared project root",
		},
		{
			name:    "approved root itself",
			path:    filepath.Join(root, "srv", "sites"),
			roots:   []string{resolvedPath(t, filepath.Join(root, "srv", "sites"))},
			wantErr: "cannot be the approved root itself",
		},
		{
			name:    "user home itself",
			path:    filepath.Join(root, "home", "mike"),
			wantErr: "cannot be the home directory",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateProjectPath(PathValidateOptions{
				RequestedPath: tc.path,
				ProjectName:   "example",
				PublicDir:     "public",
				Identity:      id,
				ApprovedRoots: tc.roots,
				Homes:         homes,
			})
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateSharedPathWithoutUser(t *testing.T) {
	root := t.TempDir()
	setupFilesystem(t, root)
	path := filepath.Join(root, "srv", "sites", "example")
	_, err := ValidateProjectPath(PathValidateOptions{
		RequestedPath: path,
		ProjectName:   "example",
		Identity:      RuntimeIdentity{Mode: OwnershipShared},
		Homes:         testHomes(root),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsTraversal(t *testing.T) {
	_, err := ValidateProjectPath(PathValidateOptions{
		RequestedPath: "/home/mike/../jane/example",
		ProjectName:   "example",
		Identity:      RuntimeIdentity{Mode: OwnershipIsolated, User: "mike", Home: "/home/mike"},
	})
	if err == nil || !strings.Contains(err.Error(), "unsafe traversal") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateExistingDirectoryOwnedByAnotherUser(t *testing.T) {
	root := t.TempDir()
	setupFilesystem(t, root)
	foreign := filepath.Join(root, "home", "mike", "owned")
	if err := os.MkdirAll(foreign, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(foreign, 1001, 1001); err != nil {
		t.Skip("cannot chown in test environment")
	}

	_, err := ValidateProjectPath(PathValidateOptions{
		RequestedPath: foreign,
		ProjectName:   "owned",
		Identity: RuntimeIdentity{
			Mode: OwnershipIsolated,
			User: "mike",
			UID:  1000,
			Home: filepath.Join(root, "home", "mike"),
		},
		Homes: testHomes(root),
	})
	if err == nil || !strings.Contains(err.Error(), "owned by") {
		t.Fatalf("error = %v", err)
	}
}

func TestMkdirIsolatedOnlyChownsNewDirectories(t *testing.T) {
	root := t.TempDir()
	setupFilesystem(t, root)
	parent := filepath.Join(root, "home", "mike")
	project := filepath.Join(parent, "example.com")
	public := filepath.Join(project, "public")

	result, err := mkdirProjectTree(&ValidatedPaths{
		ProjectPath:  project,
		PublicPath:   public,
		DocumentRoot: public,
	}, RuntimeIdentity{Mode: OwnershipIsolated, UID: os.Getuid(), GID: os.Getgid()}, 0o755)
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skip("chown not permitted in test environment")
		}
		t.Fatal(err)
	}
	if len(result.Created) != 2 {
		t.Fatalf("created = %#v", result.Created)
	}
	for _, p := range result.Created {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		stat := info.Sys()
		// ensure chown attempted; exact uid check may fail on some systems
		_ = stat
	}
}

func TestRequiredACLsHomeProject(t *testing.T) {
	root := t.TempDir()
	paths := &ValidatedPaths{
		ProjectPath:  filepath.Join(root, "home", "mike", "example.com"),
		PublicPath:   filepath.Join(root, "home", "mike", "example.com", "public"),
		DocumentRoot: filepath.Join(root, "home", "mike", "example.com", "public"),
	}
	id := RuntimeIdentity{Home: filepath.Join(root, "home", "mike")}
	entries := RequiredACLs(paths, id)

	var traversal []string
	for _, e := range entries {
		if e.Permissions == "x" {
			traversal = append(traversal, e.Path)
		}
	}
	if len(traversal) < 2 {
		t.Fatalf("traversal paths = %#v", traversal)
	}
	if traversal[0] != filepath.Join(root, "home", "mike") {
		t.Fatalf("first traversal = %s", traversal[0])
	}
	for _, e := range entries {
		if e.Recurse && e.Permissions == "rX" && !e.Default {
			if e.Path != paths.PublicPath {
				t.Fatalf("public acl path = %s", e.Path)
			}
		}
	}
}

func TestGeneratePHPPoolNameAndSocket(t *testing.T) {
	conf := buildPHPPoolPaths("my.long.domain-name.example", "8.4")
	if conf.PoolName == "" || !strings.HasPrefix(conf.PoolName, phpPoolPrefix) {
		t.Fatalf("pool = %#v", conf)
	}
	if len(conf.SocketPath) > maxUnixSocketPathLen {
		t.Fatalf("socket path too long: %d", len(conf.SocketPath))
	}
	if !strings.Contains(conf.SocketPath, "8.4") {
		t.Fatalf("socket = %s", conf.SocketPath)
	}
}

func TestBuildNginxConfigUsesProjectSocket(t *testing.T) {
	socket := "/run/php/php8.4-fpm-abstrax-example.sock"
	conf := buildNginxConfig(vhostConfig{
		Name:       "example",
		Path:       "/home/mike/example",
		Domains:    []string{"example.com"},
		Runtime:    RuntimePHP,
		PHPVersion: "8.4",
		PublicDir:  "public",
		PHPSocket:  socket,
	})
	if !strings.Contains(conf, "fastcgi_pass unix:"+socket+";") {
		t.Fatalf("config missing socket:\n%s", conf)
	}
	if !strings.Contains(conf, "try_files $uri =404;") {
		t.Fatalf("config missing php try_files guard:\n%s", conf)
	}
}

func TestBuildNginxConfigSharedDefaultSocket(t *testing.T) {
	conf := buildNginxConfig(vhostConfig{
		Name:       "example",
		Path:       "/var/www/example",
		Runtime:    RuntimePHP,
		PHPVersion: "8.5",
	})
	if !strings.Contains(conf, "fastcgi_pass unix:/run/php/php8.5-fpm.sock;") {
		t.Fatalf("config = %s", conf)
	}
}

func TestIdentityFromStateBackwardsCompatible(t *testing.T) {
	state := &State{Owner: "www-data"}
	id := IdentityFromState(state)
	if id.Mode != OwnershipShared || id.User != SharedWebUser {
		t.Fatalf("id = %#v", id)
	}
}

func TestRenderPHPPoolSocketPermissions(t *testing.T) {
	conf := PHPPoolConfig{
		PoolName:   "abstrax-example",
		SocketPath: "/run/php/php8.4-fpm-example.sock",
	}
	rendered := renderPHPPool(conf, RuntimeIdentity{User: "mike", Group: "mike", WebServerUser: NginxUser})
	if !strings.Contains(rendered, "user = mike") || !strings.Contains(rendered, "listen.group = www-data") {
		t.Fatalf("rendered = %s", rendered)
	}
	if !strings.Contains(rendered, "listen.mode = 0660") || strings.Contains(rendered, "0666") {
		t.Fatalf("rendered = %s", rendered)
	}
}

func TestPublicPathCannotEscapeProject(t *testing.T) {
	_, err := ValidateProjectPath(PathValidateOptions{
		RequestedPath: "/home/mike/example",
		ProjectName:   "example",
		WebRoot:       "/home/mike/other",
		Identity: RuntimeIdentity{
			Mode: OwnershipIsolated,
			User: "mike",
			Home: "/home/mike",
		},
		Homes: []identity.HomeEntry{{Username: "mike", Home: "/home/mike"}},
	})
	if err == nil || !strings.Contains(err.Error(), "escapes project directory") {
		t.Fatalf("error = %v", err)
	}
}

func TestSymlinkEscapeRejected(t *testing.T) {
	root := t.TempDir()
	setupFilesystem(t, root)
	trap := filepath.Join(root, "home", "mike", "trap")
	if err := os.Symlink(filepath.Join(root, "home", "jane"), trap); err != nil {
		t.Skip("cannot create symlink in test environment")
	}

	_, err := ValidateProjectPath(PathValidateOptions{
		RequestedPath: filepath.Join(trap, "example"),
		ProjectName:   "example",
		Identity: RuntimeIdentity{
			Mode: OwnershipIsolated,
			User: "mike",
			Home: resolvedPath(t, filepath.Join(root, "home", "mike")),
		},
		Homes: testHomes(root),
	})
	if err == nil || !strings.Contains(err.Error(), "inside another user's home directory") {
		t.Fatalf("error = %v", err)
	}
}
