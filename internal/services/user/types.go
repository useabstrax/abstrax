package user

import "context"

// AddOptions holds options for creating a user.
type AddOptions struct {
	Username         string
	CreateHome       bool
	NoCreateHome     bool
	GrantSudo        bool
	Groups           []string
	Shell            string
	UID              string
	System           bool
	Password         string
	DisabledPassword bool
	Comment          string
	DryRun           bool
}

// RemoveOptions holds options for removing a user.
type RemoveOptions struct {
	Username      string
	DeleteHome    bool
	KeepHome      bool
	RemoveCron    bool
	KillProcesses bool
	Force         bool
	IfUnused      bool
	DryRun        bool
}

// UsedBy is one daemon, cron job, or process that still runs as a user.
type UsedBy struct {
	Type      string `json:"type"`
	Name      string `json:"name,omitempty"`
	Processes int    `json:"processes,omitempty"`
	PID       int    `json:"pid,omitempty"`
	Args      string `json:"args,omitempty"`
}

// RemoveResult is returned by Remove.
type RemoveResult struct {
	StillUsedBy []UsedBy `json:"still_used_by"`
}

// Process is one running process owned by a user.
type Process struct {
	PID  int
	Args string
}

// ProcessLister lists processes owned by username.
// Tests inject this so removal checks do not depend on live processes.
type ProcessLister func(ctx context.Context, username string) ([]Process, error)

// InUseError is returned when --if-unused refuses to remove a user.
type InUseError struct {
	Username string
	UsedBy   []UsedBy
}

func (e *InUseError) Error() string {
	if e == nil {
		return "user is still in use"
	}
	return formatUsedBy(e.Username, e.UsedBy)
}

// ModifyGroupsOptions holds options for group modification commands.
type ModifyGroupsOptions struct {
	Username string
	Groups   []string
	DryRun   bool
}

// SetShellOptions holds options for set-shell.
type SetShellOptions struct {
	Username string
	Shell    string
	DryRun   bool
}

// LockOptions holds options for lock/unlock.
type LockOptions struct {
	Username     string
	PasswordOnly bool
	DryRun       bool
}

// ListOptions holds filters for user list.
type ListOptions struct {
	System  bool
	Regular bool
	Sudo    bool
}

// UserInfo holds information about a user.
type UserInfo struct {
	Username      string   `json:"username"`
	UID           string   `json:"uid"`
	GID           string   `json:"gid"`
	Comment       string   `json:"comment"`
	Home          string   `json:"home"`
	Shell         string   `json:"shell"`
	Groups        []string `json:"groups"`
	IsSudo        bool     `json:"is_sudo"`
	IsSystem      bool     `json:"is_system"`
	Locked        bool     `json:"locked"`
	Expired       bool     `json:"expired"`
	LoginBlocked  bool     `json:"login_blocked"`
	LastLoginAt   *string  `json:"last_login_at"`
	LastLoginFrom *string  `json:"last_login_from"`
}

// Session is one interactive login session.
// Idle is nil when the source did not report an idle time.
type Session struct {
	ID        string  `json:"id"`
	TTY       string  `json:"tty"`
	SourceIP  string  `json:"source_ip"`
	LoginTime string  `json:"login_time"`
	Idle      *string `json:"idle"`
}

// LogoutOptions selects login sessions to end.
// Exactly one of SessionID or All must be set.
type LogoutOptions struct {
	Username  string
	SessionID string
	All       bool
	DryRun    bool
}

// LogoutResult reports the login sessions that were ended.
type LogoutResult struct {
	Username string    `json:"username"`
	Ended    []Session `json:"ended"`
}

// AddResult is returned by Add.
type AddResult struct {
	Username       string   `json:"username"`
	UID            string   `json:"uid"`
	Home           string   `json:"home"`
	Shell          string   `json:"shell"`
	Groups         []string `json:"groups"`
	Sudo           bool     `json:"sudo"`
	Created        bool     `json:"created"`
	AlreadyExisted bool     `json:"already_existed,omitempty"`
}
