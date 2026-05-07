package user

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
	DryRun        bool
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
	Username string
	DryRun   bool
}

// ListOptions holds filters for user list.
type ListOptions struct {
	System  bool
	Regular bool
	Sudo    bool
}

// UserInfo holds information about a user.
type UserInfo struct {
	Username string   `json:"username"`
	UID      string   `json:"uid"`
	GID      string   `json:"gid"`
	Comment  string   `json:"comment"`
	Home     string   `json:"home"`
	Shell    string   `json:"shell"`
	Groups   []string `json:"groups"`
	IsSudo   bool     `json:"is_sudo"`
	IsSystem bool     `json:"is_system"`
	Locked   bool     `json:"locked"`
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
