package cron

// AddOptions holds options for adding a cron job.
type AddOptions struct {
	ID            string
	User          string
	Command       string
	Schedule      string
	Output        string
	ErrorOutput   string
	AppendOutput  bool
	DiscardOutput bool
	WorkingDir    string
	Env           map[string]string
	Enabled       bool
	DryRun        bool
}

// ModifyOptions holds options for modifying a cron job.
type ModifyOptions struct {
	ID          string
	User        string
	Command     string
	Schedule    string
	Output      string
	ErrorOutput string
	WorkingDir  string
	Env         map[string]string
	DryRun      bool
}

// CronJob describes a managed cron job.
type CronJob struct {
	ID       string            `json:"id"`
	User     string            `json:"user"`
	Command  string            `json:"command"`
	Schedule string            `json:"schedule"`
	Enabled  bool              `json:"enabled"`
	FilePath string            `json:"file_path"`
	Env      map[string]string `json:"env,omitempty"`
}
