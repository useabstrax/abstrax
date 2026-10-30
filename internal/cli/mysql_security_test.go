package cli

import (
	"strings"
	"testing"
)

func TestMySQLRejectsUnsafeSQLInput(t *testing.T) {
	cases := [][]string{
		{"user", "add", "app", "--host", "localhost", "--grant-db", "shop`; DROP DATABASE x; --"},
		{"user", "add", "app", "--host", "localhost'; DROP DATABASE shop; --"},
		{"grant", "app", "shop", "--privileges", "ALL ON *.* TO 'app'@'localhost' WITH GRANT OPTION, SELECT"},
		{"grant", "app", "shop", "--host", "localhost; DROP DATABASE shop"},
		{"database", "add", "shop", "--charset", "utf8mb4; DROP DATABASE x"},
	}
	for _, args := range cases {
		cmd := NewMySQLCmd()
		cmd.SetArgs(args)
		err := cmd.Execute()
		if err == nil {
			t.Fatalf("expected error for %q", strings.Join(args, " "))
		}
	}
}
