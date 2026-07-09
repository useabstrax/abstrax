package mysql_test

import (
	"strings"
	"testing"

	"abstrax/internal/platform"
)

// Re-export helpers via a thin test in the mysql package would be ideal;
// these tests validate provider auth plugins used by secure SQL builders.
func TestDatabaseAuthPlugins(t *testing.T) {
	if platform.DebianDefaults().DatabaseAuthPlugin() != platform.DatabaseAuthCachingSHA2 {
		t.Fatal("debian should use caching_sha2_password")
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
