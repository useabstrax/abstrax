package mysql

import (
	"regexp"
	"sort"
	"strings"
)

var grantLineRe = regexp.MustCompile(`(?i)^GRANT\s+(.+?)\s+ON\s+(\S+)\s+TO\s+`)

var systemMySQLUsers = map[string]struct{}{
	"mysql.sys":        {},
	"mysql.session":    {},
	"mysql.infoschema": {},
	"debian-sys-maint": {},
	"mariadb.sys":      {},
	"root":             {},
}

func isSystemMySQLUser(name string) bool {
	_, ok := systemMySQLUsers[name]
	return ok
}

func parseGrantLine(line string) (privs []string, object string, ok bool) {
	line = strings.TrimSpace(line)
	m := grantLineRe.FindStringSubmatch(line)
	if m == nil {
		return nil, "", false
	}
	return splitPrivileges(m[1]), m[2], true
}

func splitPrivileges(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		priv := strings.Join(strings.Fields(strings.ToUpper(part)), " ")
		if priv != "" {
			out = append(out, priv)
		}
	}
	return out
}

func attachParsedGrants(info *UserInfo) {
	var databases []DatabaseGrant
	var global []string
	for _, line := range info.Grants {
		privs, object, ok := parseGrantLine(line)
		if !ok {
			continue
		}
		schema, name, split := splitGrantObject(object)
		if !split {
			continue
		}
		if schema == "*" && name == "*" {
			if len(privs) == 1 && privs[0] == "USAGE" {
				continue
			}
			global = append(global, privs...)
			continue
		}
		if name != "*" {
			continue
		}
		databases = append(databases, DatabaseGrant{
			Name:       schema,
			Privileges: privs,
			Preset:     matchPreset(privs),
		})
	}
	if databases == nil {
		databases = []DatabaseGrant{}
	}
	if global == nil {
		global = []string{}
	}
	info.Databases = databases
	info.GlobalPrivileges = global
}

func splitGrantObject(object string) (schema, name string, ok bool) {
	i := strings.LastIndex(object, ".")
	if i <= 0 || i == len(object)-1 {
		return "", "", false
	}
	return unquoteIdent(object[:i]), unquoteIdent(object[i+1:]), true
}

func unquoteIdent(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		switch s[0] {
		case '`', '\'', '"':
			if s[len(s)-1] == s[0] {
				return s[1 : len(s)-1]
			}
		}
	}
	return s
}

func matchPreset(privs []string) *string {
	got := privilegeKey(privs)
	for name, spec := range PresetPrivileges {
		if privilegeKey(splitPrivileges(spec)) == got {
			preset := name
			return &preset
		}
	}
	return nil
}

func privilegeKey(privs []string) string {
	cp := append([]string(nil), privs...)
	for i := range cp {
		cp[i] = strings.ToUpper(strings.TrimSpace(cp[i]))
	}
	sort.Strings(cp)
	return strings.Join(cp, "\x00")
}
