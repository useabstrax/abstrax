package daemon

// AddOptions holds options for adding a daemon.
type AddOptions struct {
	Name              string
	Command           string
	Directory         string
	User              string
	Processes         int
	Autostart         bool
	Autorestart       string // always, on-failure, false
	StartSecs         int
	StartRetries      int
	StopSignal        string
	StopWaitSecs      int
	ExitCodes         string
	StdoutLogFile     string
	StderrLogFile     string
	Environment       map[string]string
	InstallSupervisor bool
	DryRun            bool
}

// RemoveOptions holds options for removing a daemon.
type RemoveOptions struct {
	Name       string
	Stop       bool
	DeleteLogs bool
	Force      bool
	DryRun     bool
}

// LogOptions holds options for fetching daemon logs.
type LogOptions struct {
	Name   string
	Lines  int
	Follow bool
	Stderr bool
	Stdout bool
}

// InstallOptions holds options for installing Supervisor.
type InstallOptions struct {
	DryRun bool
}

// InstallResult describes the outcome of installing Supervisor.
type InstallResult struct {
	Package          string `json:"package"`
	Service          string `json:"service"`
	AlreadyInstalled bool   `json:"already_installed,omitempty"`
}

// DaemonInfo describes a supervisor-managed daemon.
type DaemonInfo struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	PID         int    `json:"pid,omitempty"`
	Uptime      string `json:"uptime,omitempty"`
	Description string `json:"description,omitempty"`
	ConfigPath  string `json:"config_path"`
}
