package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWebTraverseDirsHomeProject(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home", "mike")
	project := filepath.Join(home, "example.com")
	public := filepath.Join(project, "public")
	for _, dir := range []string{home, project, public} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}

	dirs := webTraverseDirs(&ValidatedPaths{
		ProjectPath:  project,
		PublicPath:   public,
		DocumentRoot: public,
	}, home)

	if len(dirs) < 2 {
		t.Fatalf("dirs = %#v", dirs)
	}
	if dirs[0] != home {
		t.Fatalf("first dir = %s", dirs[0])
	}
}

func TestAddOtherTraverse(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "site")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := addOtherTraverse(dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0001 == 0 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func TestIsDeployStylePublicPath(t *testing.T) {
	project := "/home/abstrax/useabstrax.com"
	cases := []struct {
		public string
		want   bool
	}{
		{filepath.Join(project, "current", "public"), true},
		{filepath.Join(project, "public"), false},
	}
	for _, tc := range cases {
		if got := isDeployStylePublicPath(project, tc.public); got != tc.want {
			t.Fatalf("isDeployStylePublicPath(%q) = %v, want %v", tc.public, got, tc.want)
		}
	}
}

func TestEnsureWebTraverseAccessDeployDirs(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home", "abstrax")
	project := filepath.Join(home, "useabstrax.com")
	for _, dir := range []string{home, project} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	releases := filepath.Join(project, "releases")
	shared := filepath.Join(project, "shared")
	for _, dir := range []string{releases, shared} {
		if err := os.Mkdir(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}

	public := filepath.Join(project, "current", "public")
	if err := ensureWebTraverseAccess(&ValidatedPaths{
		ProjectPath:  project,
		PublicPath:   public,
		DocumentRoot: public,
	}, RuntimeIdentity{Home: home}); err != nil {
		t.Fatal(err)
	}

	for _, dir := range []string{home, project, releases, shared} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0001 == 0 {
			t.Fatalf("%s mode = %o", dir, info.Mode().Perm())
		}
	}
}

func TestMkdirSkipsDeployStylePublicPath(t *testing.T) {
	root := t.TempDir()
	setupFilesystem(t, root)
	project := filepath.Join(root, "home", "abstrax", "useabstrax.com")
	public := filepath.Join(project, "current", "public")

	result, err := mkdirProjectTree(&ValidatedPaths{
		ProjectPath:  project,
		PublicPath:   public,
		DocumentRoot: public,
	}, RuntimeIdentity{Mode: OwnershipIsolated, UID: os.Getuid(), GID: os.Getgid()}, 0o755)
	if err != nil {
		t.Fatal(err)
	}

	var createdProject, createdReleases, createdShared bool
	for _, p := range result.Created {
		switch p {
		case project:
			createdProject = true
		case filepath.Join(project, "releases"):
			createdReleases = true
		case filepath.Join(project, "shared"):
			createdShared = true
		}
	}
	if !createdProject || !createdReleases || !createdShared {
		t.Fatalf("created = %#v", result.Created)
	}
	if _, err := os.Stat(public); !os.IsNotExist(err) {
		t.Fatalf("deploy-style public path should not be created: err=%v", err)
	}
}
