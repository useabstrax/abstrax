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
	Name    string
	Lines   int
	Follow  bool
	Stderr  bool
	Stdout  bool
	Process int
}

// ListOptions holds options for listing daemons.
type ListOptions struct {
	All bool
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

// ProcessInfo is one Supervisor process belonging to a daemon.
type ProcessInfo struct {
	Name          string `json:"name"`
	Status        string `json:"status"`
	PID           int    `json:"pid,omitempty"`
	Uptime        string `json:"uptime,omitempty"`
	UptimeSeconds int    `json:"uptime_seconds,omitempty"`
	Description   string `json:"description,omitempty"`
}

// DaemonInfo describes a supervisor-managed daemon.
//
// Status is the worst process state. Processes lists each process.
// Numprocs is the configured process count from the Abstrax config.
type DaemonInfo struct {
	Name         string            `json:"name"`
	Managed      bool              `json:"managed"`
	ConfigPath   string            `json:"config_path,omitempty"`
	Status       string            `json:"status,omitempty"`
	Processes    []ProcessInfo     `json:"processes,omitempty"`
	Command      string            `json:"command,omitempty"`
	Directory    string            `json:"directory,omitempty"`
	User         string            `json:"user,omitempty"`
	Numprocs     int               `json:"numprocs,omitempty"`
	Autostart    bool              `json:"autostart,omitempty"`
	Autorestart  string            `json:"autorestart,omitempty"`
	StartSecs    int               `json:"startsecs,omitempty"`
	StartRetries int               `json:"startretries,omitempty"`
	StopSignal   string            `json:"stopsignal,omitempty"`
	StopWaitSecs int               `json:"stopwaitsecs,omitempty"`
	ExitCodes    string            `json:"exitcodes,omitempty"`
	StdoutLog    string            `json:"stdout_logfile,omitempty"`
	StderrLog    string            `json:"stderr_logfile,omitempty"`
	Environment  map[string]string `json:"environment,omitempty"`
}
