package mysql_test

import (
	"strings"
	"testing"

	"abstrax/internal/platform"
)

// Re-export helpers via a thin test in the mysql package would be ideal;
// these tests validate provider auth plugins used by secure SQL builders.
func TestDatabaseAuthPlugins(t *testing.T) {
	if platform.DebianDefaults().DatabaseAuthPlugin() != platform.DatabaseAuthNativePassword {
		t.Fatal("debian should use mysql_native_password for MariaDB")
	}
	if platform.DebianDefaults().DatabasePackage("") != "mariadb-server" {
		t.Fatal("debian database package should be mariadb-server")
	}
	if !strings.Contains(platform.DebianDefaults().DatabaseDisplayName(), "MariaDB") {
		t.Fatal("debian display name should mention MariaDB")
	}
	if platform.RHELDefaults().DatabaseAuthPlugin() != platform.DatabaseAuthNativePassword {
		t.Fatal("rhel should use mysql_native_password for MariaDB")
	}
	if platform.RHELDefaults().DatabasePackage("") != "mariadb-server" {
		t.Fatal("rhel database package should be mariadb-server")
	}
	if !strings.Contains(platform.RHELDefaults().DatabaseDisplayName(), "MariaDB") {
		t.Fatal("rhel display name should mention MariaDB")
	}
}
