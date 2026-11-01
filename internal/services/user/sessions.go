package user

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"
)

const (
	sessionBackendLoginctl = "loginctl"
	sessionBackendWho      = "who"
)

var (
	sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9]+$`)
	idleTokenPattern = regexp.MustCompile(`^\d+:\d{2}(:\d{2})?$`)
	pidTokenPattern  = regexp.MustCompile(`^\d+$`)
)

// ValidateSessionID reports whether id can be passed to loginctl or matched
// against a who(1) process id. It rejects flags and shell metacharacters.
func ValidateSessionID(id string) error {
	if !sessionIDPattern.MatchString(id) {
		return fmt.Errorf("invalid session id %q", id)
	}
	return nil
}

// Sessions lists interactive login sessions for username.
// loginctl is used when it returns JSON. Otherwise the list comes from who -u.
// A user with no sessions returns an empty list.
func (s *Service) Sessions(ctx context.Context, username string) ([]Session, error) {
	sessions, _, err := s.listLoginSessions(ctx, username)
	if err != nil {
		return nil, err
	}
	if sessions == nil {
		return []Session{}, nil
	}
	return sessions, nil
}

// Logout ends one login session, or each of the user's login sessions.
// With logind it runs loginctl terminate-session for each selected id.
// Without logind it signals only that session's sshd: <user>@pts/N process.
// It does not run loginctl terminate-user and does not signal other processes
// the user owns, including daemons and cron jobs.
func (s *Service) Logout(ctx context.Context, opts LogoutOptions) (*LogoutResult, error) {
	if opts.All && opts.SessionID != "" {
		return nil, fmt.Errorf("pass either --session or --all, not both")
	}
	if !opts.All && opts.SessionID == "" {
		return nil, fmt.Errorf("pass --session or --all")
	}
	if opts.SessionID != "" {
		if err := ValidateSessionID(opts.SessionID); err != nil {
			return nil, err
		}
	}

	listed, backend, err := s.listLoginSessions(ctx, opts.Username)
	if err != nil {
		return nil, err
	}
	targets, err := selectSessions(listed, opts)
	if err != nil {
		return nil, err
	}
	if backend == sessionBackendWho {
		if !opts.All && len(targets) == 1 && !strings.HasPrefix(targets[0].TTY, "pts/") {
			return nil, fmt.Errorf("session %s on %s is not an interactive SSH session", targets[0].ID, targets[0].TTY)
		}
		if opts.All {
			targets = interactiveSSHSessions(targets)
		}
	}

	procText := ""
	if backend == sessionBackendWho && len(targets) > 0 {
		ps, err := s.runner.RunSilent(ctx, "ps", "-eo", "pid=,args=")
		if err != nil {
			return nil, fmt.Errorf("listing processes: %w", err)
		}
		procText = ps.Stdout
	}

	cmds, err := terminateCommands(backend, opts.Username, targets, procText)
	if err != nil {
		return nil, err
	}
	var firstErr error
	for _, c := range cmds {
		if _, err := s.runner.Run(ctx, c[0], c[1:]...); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("%s: %w", strings.Join(c, " "), err)
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	if targets == nil {
		targets = []Session{}
	}
	return &LogoutResult{Username: opts.Username, Ended: targets}, nil
}

func (s *Service) listLoginSessions(ctx context.Context, username string) ([]Session, string, error) {
	res, err := s.runner.RunSilent(ctx, "loginctl", "list-sessions", "--output=json")
	if err == nil {
		sessions, perr := parseLoginctlSessions(res.Stdout, username)
		if perr == nil {
			return sessions, sessionBackendLoginctl, nil
		}
	}

	whoRes, werr := s.runner.RunSilent(ctx, "who", "-u")
	if werr != nil {
		return nil, "", fmt.Errorf("listing sessions for %s: %w", username, werr)
	}
	sessions, perr := parseWhoSessions(whoRes.Stdout, username)
	if perr != nil {
		return nil, "", perr
	}
	return sessions, sessionBackendWho, nil
}

func selectSessions(listed []Session, opts LogoutOptions) ([]Session, error) {
	if opts.All {
		if listed == nil {
			return []Session{}, nil
		}
		return listed, nil
	}
	for _, sess := range listed {
		if sess.ID == opts.SessionID {
			return []Session{sess}, nil
		}
	}
	return nil, fmt.Errorf("session %s not found for user %s", opts.SessionID, opts.Username)
}

func interactiveSSHSessions(sessions []Session) []Session {
	out := make([]Session, 0, len(sessions))
	for _, sess := range sessions {
		if strings.HasPrefix(sess.TTY, "pts/") {
			out = append(out, sess)
		}
	}
	return out
}

// terminateCommands builds the commands that end the given login sessions.
// loginctl sessions become terminate-session. who sessions become SIGHUP and
// then SIGKILL for the matching sshd: user@pts/N process only.
func terminateCommands(backend, username string, sessions []Session, processes string) ([][]string, error) {
	switch backend {
	case sessionBackendLoginctl:
		cmds := make([][]string, 0, len(sessions))
		for _, sess := range sessions {
			if sess.ID == "" {
				return nil, fmt.Errorf("session is missing an id")
			}
			cmds = append(cmds, []string{"loginctl", "terminate-session", sess.ID})
		}
		return cmds, nil
	case sessionBackendWho:
		return sshdSignalCommands(username, sessions, processes)
	default:
		return nil, fmt.Errorf("unknown session backend %q", backend)
	}
}

func sshdSignalCommands(username string, sessions []Session, processes string) ([][]string, error) {
	procs := parseProcessLines(processes)
	cmds := make([][]string, 0, len(sessions)*2)
	for _, sess := range sessions {
		if !strings.HasPrefix(sess.TTY, "pts/") {
			return nil, fmt.Errorf("session %s on %s is not an interactive SSH session", sess.ID, sess.TTY)
		}
		pids := sshdPIDs(procs, username, sess.TTY)
		if len(pids) == 0 {
			return nil, fmt.Errorf("no sshd process for %s on %s", username, sess.TTY)
		}
		for _, pid := range pids {
			cmds = append(cmds,
				[]string{"kill", "-HUP", pid},
				[]string{"kill", "-KILL", pid},
			)
		}
	}
	return cmds, nil
}

type processLine struct {
	PID  string
	Args string
}

func parseProcessLines(out string) []processLine {
	var procs []processLine
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pid, args, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		pid = strings.TrimSpace(pid)
		args = strings.TrimSpace(args)
		if !pidTokenPattern.MatchString(pid) || args == "" {
			continue
		}
		procs = append(procs, processLine{PID: pid, Args: args})
	}
	return procs
}

// sshdPIDs returns pids whose command is sshd: <user>@<tty>.
// sshd: user@notty, sshd: user [priv], and other users do not match.
// pts/1 does not match pts/10.
func sshdPIDs(procs []processLine, username, tty string) []string {
	re := regexp.MustCompile(`(?:^|\s)sshd:\s+` + regexp.QuoteMeta(username) + `@` + regexp.QuoteMeta(tty) + `(?:$|\s)`)
	var pids []string
	for _, proc := range procs {
		if re.MatchString(proc.Args) {
			pids = append(pids, proc.PID)
		}
	}
	return pids
}

type loginctlSession struct {
	Session    string          `json:"session"`
	User       string          `json:"user"`
	UID        json.RawMessage `json:"uid"`
	TTY        string          `json:"tty"`
	Remote     json.RawMessage `json:"remote"`
	RemoteHost string          `json:"remote_host"`
	Timestamp  string          `json:"timestamp"`
	State      json.RawMessage `json:"state"`
	Idle       json.RawMessage `json:"idle"`
}

func parseLoginctlSessions(out, username string) ([]Session, error) {
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, fmt.Errorf("empty loginctl output")
	}
	var rows []loginctlSession
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		return nil, err
	}
	sessions := make([]Session, 0)
	for _, row := range rows {
		if row.User != username || row.Session == "" {
			continue
		}
		sessions = append(sessions, Session{
			ID:        row.Session,
			TTY:       row.TTY,
			SourceIP:  strings.TrimSpace(row.RemoteHost),
			LoginTime: strings.TrimSpace(row.Timestamp),
			Idle:      idleFromRaw(row.Idle),
		})
	}
	return sessions, nil
}

func idleFromRaw(raw json.RawMessage) *string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) || raw[0] != '"' {
		return nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil || value == "" {
		return nil
	}
	return &value
}

func parseWhoSessions(out, username string) ([]Session, error) {
	sessions := make([]Session, 0)
	for _, line := range strings.Split(out, "\n") {
		sess, ok := parseWhoLine(line, username)
		if !ok {
			continue
		}
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

func parseWhoLine(line, username string) (Session, bool) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) < 4 {
		return Session{}, false
	}
	if strings.EqualFold(fields[0], "NAME") || strings.EqualFold(fields[0], "USER") {
		return Session{}, false
	}
	if fields[0] != username {
		return Session{}, false
	}

	var idle string
	var haveIdle bool
	var pid string
	var source string
	for _, tok := range fields[4:] {
		switch {
		case !haveIdle && isIdleToken(tok):
			idle = tok
			haveIdle = true
		case pid == "" && pidTokenPattern.MatchString(tok):
			pid = tok
		case source == "":
			if addr, ok := sourceToken(tok); ok {
				source = addr
			}
		}
	}

	id := pid
	if id == "" {
		id = fields[1]
	}
	return Session{
		ID:        id,
		TTY:       fields[1],
		SourceIP:  source,
		LoginTime: fields[2] + " " + fields[3],
		Idle:      idlePtr(idle, haveIdle),
	}, true
}

func isIdleToken(tok string) bool {
	return tok == "." || tok == "old" || idleTokenPattern.MatchString(tok)
}

func sourceToken(tok string) (string, bool) {
	if strings.HasPrefix(tok, "(") && strings.HasSuffix(tok, ")") && len(tok) > 2 {
		inner := tok[1 : len(tok)-1]
		if inner != "" {
			return inner, true
		}
	}
	if net.ParseIP(tok) != nil {
		return tok, true
	}
	return "", false
}

func idlePtr(value string, present bool) *string {
	if !present || value == "" {
		return nil
	}
	v := value
	return &v
}
