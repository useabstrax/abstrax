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

func TestEnsureWebTraverseAccessExistingDirs(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home", "abstrax")
	project := filepath.Join(home, "useabstrax.com")
	for _, dir := range []string{home, project} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}

	if err := ensureWebTraverseAccess(&ValidatedPaths{
		ProjectPath:  project,
		PublicPath:   filepath.Join(project, "current", "public"),
		DocumentRoot: filepath.Join(project, "current", "public"),
	}, RuntimeIdentity{Home: home}); err != nil {
		t.Fatal(err)
	}

	for _, dir := range []string{home, project} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0001 == 0 {
			t.Fatalf("%s mode = %o", dir, info.Mode().Perm())
		}
	}
}

func TestMkdirCreatesOnlyProjectRoot(t *testing.T) {
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

	if len(result.Created) == 0 || result.Created[len(result.Created)-1] != project {
		t.Fatalf("created = %#v", result.Created)
	}
	if _, err := os.Stat(public); !os.IsNotExist(err) {
		t.Fatalf("public path should not be created: err=%v", err)
	}
}
