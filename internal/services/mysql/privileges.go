package mysql

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"abstrax/internal/validate"
)

var mysqlIdentRe = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

var allowedPrivileges = map[string]struct{}{
	"ALL":                     {},
	"ALL PRIVILEGES":          {},
	"USAGE":                   {},
	"SELECT":                  {},
	"INSERT":                  {},
	"UPDATE":                  {},
	"DELETE":                  {},
	"CREATE":                  {},
	"DROP":                    {},
	"REFERENCES":              {},
	"INDEX":                   {},
	"ALTER":                   {},
	"CREATE TEMPORARY TABLES": {},
	"LOCK TABLES":             {},
	"EXECUTE":                 {},
	"CREATE VIEW":             {},
	"SHOW VIEW":               {},
	"CREATE ROUTINE":          {},
	"ALTER ROUTINE":           {},
	"EVENT":                   {},
	"TRIGGER":                 {},
	"DELETE HISTORY":          {},
}

// ParsePrivileges splits a privilege list and allows only known names.
func ParsePrivileges(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	hasAll := false
	for _, part := range parts {
		priv := strings.Join(strings.Fields(strings.ToUpper(part)), " ")
		if priv == "" {
			return nil, fmt.Errorf("empty privilege in %q", raw)
		}
		if _, ok := allowedPrivileges[priv]; !ok {
			return nil, fmt.Errorf("unknown privilege %q; allowed: %s", priv, allowedPrivilegeList())
		}
		if priv == "ALL" || priv == "ALL PRIVILEGES" {
			hasAll = true
		}
		out = append(out, priv)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("privileges cannot be empty")
	}
	if hasAll && len(out) > 1 {
		return nil, fmt.Errorf("ALL cannot be combined with other privileges")
	}
	return out, nil
}

func allowedPrivilegeList() string {
	names := make([]string, 0, len(allowedPrivileges))
	for name := range allowedPrivileges {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func quoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

func sqlAccount(user, host string) (string, error) {
	if err := validate.MySQLUsername(user); err != nil {
		return "", err
	}
	if err := validate.MySQLHost(host); err != nil {
		return "", err
	}
	return fmt.Sprintf("'%s'@'%s'", escapeSQLString(user), escapeSQLString(host)), nil
}

func mysqlIdent(field, value string) error {
	return ValidateIdent(field, value)
}

// ValidateIdent checks a MySQL charset, collation or plugin name.
func ValidateIdent(field, value string) error {
	if !mysqlIdentRe.MatchString(value) {
		return fmt.Errorf("invalid %s %q", field, value)
	}
	return nil
}
