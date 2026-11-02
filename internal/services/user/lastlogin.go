package user

import (
	"context"
	"regexp"
	"strings"
	"time"
)

const neverLoggedIn = "**Never logged in**"

// loginTimePattern matches lastlog, lastlog2, and `last -F` timestamps.
// Day-of-month may be space-padded (%e). A numeric zone is optional.
var loginTimePattern = regexp.MustCompile(
	`(?:Mon|Tue|Wed|Thu|Fri|Sat|Sun)\s+[A-Za-z]{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2}(?:\s+[+-]\d{4})?\s+\d{4}`,
)

type loginRecord struct {
	at   *string
	from *string
}

// applyLastLogin sets last login fields. Lookup failures leave them nil.
func (s *Service) applyLastLogin(ctx context.Context, info *UserInfo) {
	at, from := s.lookupLastLogin(ctx, info.Username)
	info.LastLoginAt = at
	info.LastLoginFrom = from
}

// lookupLastLogin tries lastlog2, then lastlog, then last. The first command
// that runs wins, including a "never logged in" result. If none run, both
// values are nil.
func (s *Service) lookupLastLogin(ctx context.Context, username string) (*string, *string) {
	attempts := []struct {
		name string
		args []string
	}{
		{name: "lastlog2", args: []string{"-u", username}},
		{name: "lastlog", args: []string{"-u", username}},
		{name: "last", args: []string{"-F", "-n", "1", "-w", username}},
	}
	for _, attempt := range attempts {
		res, err := s.runner.RunSilent(ctx, attempt.name, attempt.args...)
		if err != nil {
			continue
		}
		if rec, ok := parseLoginOutput(res.Stdout)[username]; ok {
			return rec.at, rec.from
		}
		return nil, nil
	}
	return nil, nil
}

// loadLastLogins reads every account once. lastlog2 is preferred. lastlog is
// the fallback. Per-user commands are not used.
func (s *Service) loadLastLogins(ctx context.Context) map[string]loginRecord {
	for _, name := range []string{"lastlog2", "lastlog"} {
		res, err := s.runner.RunSilent(ctx, name)
		if err != nil {
			continue
		}
		return parseLoginOutput(res.Stdout)
	}
	return map[string]loginRecord{}
}

func parseLoginOutput(out string) map[string]loginRecord {
	records := make(map[string]loginRecord)
	for _, line := range strings.Split(out, "\n") {
		user, rec, ok := parseLoginLine(line)
		if !ok || rec.at == nil {
			continue
		}
		if _, exists := records[user]; exists {
			continue
		}
		records[user] = rec
	}
	return records
}

func parseLoginLine(line string) (string, loginRecord, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", loginRecord{}, false
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "username") ||
		strings.HasPrefix(lower, "wtmp begins") ||
		strings.HasPrefix(lower, "btmp begins") {
		return "", loginRecord{}, false
	}
	if strings.Contains(trimmed, neverLoggedIn) {
		user := firstField(trimmed)
		if user == "" || user == "**Never" || skipLoginUser(user) {
			return "", loginRecord{}, false
		}
		return user, loginRecord{}, true
	}

	loc := loginTimePattern.FindStringIndex(trimmed)
	if loc == nil {
		return "", loginRecord{}, false
	}
	when := trimmed[loc[0]:loc[1]]
	fields := strings.Fields(trimmed[:loc[0]])
	if len(fields) == 0 || skipLoginUser(fields[0]) {
		return "", loginRecord{}, false
	}
	at := formatLoginTime(when)
	if at == nil {
		return "", loginRecord{}, false
	}
	rec := loginRecord{at: at}
	if len(fields) >= 3 && fields[len(fields)-1] != "" {
		rec.from = stringPtr(fields[len(fields)-1])
	}
	return fields[0], rec, true
}

func skipLoginUser(user string) bool {
	switch user {
	case "reboot", "shutdown":
		return true
	default:
		return false
	}
}

func firstField(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func formatLoginTime(value string) *string {
	layouts := []string{
		"Mon Jan _2 15:04:05 -0700 2006",
		"Mon Jan 2 15:04:05 -0700 2006",
		"Mon Jan _2 15:04:05 2006",
		"Mon Jan 2 15:04:05 2006",
	}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err != nil {
			continue
		}
		out := parsed.Format(time.RFC3339)
		return &out
	}
	return nil
}

func stringPtr(s string) *string {
	return &s
}
