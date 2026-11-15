package user

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

const (
	defaultUIDMin = 1000
	defaultUIDMax = 60000
)

// loginDefsPath is the login.defs file. Tests may override it.
var loginDefsPath = "/etc/login.defs"

type uidRange struct {
	min int
	max int
}

func loadUIDRange() uidRange {
	r := uidRange{min: defaultUIDMin, max: defaultUIDMax}
	f, err := os.Open(loginDefsPath)
	if err != nil {
		return r
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		n, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		switch fields[0] {
		case "UID_MIN":
			r.min = n
		case "UID_MAX":
			r.max = n
		}
	}
	if r.min <= 0 {
		r.min = defaultUIDMin
	}
	if r.max < r.min {
		r.max = defaultUIDMax
	}
	return r
}

// isSystemUID reports whether uid is a system account.
// UIDs outside UID_MIN..UID_MAX from login.defs are system accounts.
// Root (uid 0) is not treated as a system user for list filters, matching user info.
func isSystemUID(uid int, r uidRange) bool {
	if uid == 0 {
		return false
	}
	return uid < r.min || uid > r.max
}
