package cron

import (
	"io"
	"time"
)

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
//
// When Changed is set, only those flag names are applied. An empty
// --working-dir or --output clears that field. When Changed is nil, a
// non-empty field is applied and empty fields are left alone.
type ModifyOptions struct {
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
	DryRun        bool
	Changed       map[string]bool
}

// CronJob describes a managed cron job.
type CronJob struct {
	ID            string            `json:"id"`
	User          string            `json:"user"`
	Command       string            `json:"command"`
	Schedule      string            `json:"schedule"`
	Enabled       bool              `json:"enabled"`
	FilePath      string            `json:"file_path"`
	Env           map[string]string `json:"env,omitempty"`
	WorkingDir    string            `json:"working_dir,omitempty"`
	Output        string            `json:"output,omitempty"`
	ErrorOutput   string            `json:"error_output,omitempty"`
	AppendOutput  bool              `json:"append_output"`
	DiscardOutput bool              `json:"discard_output"`
	Created       string            `json:"created,omitempty"`
	Updated       string            `json:"updated,omitempty"`
}

// RunOptions holds options for running a managed cron job immediately.
// Timeout is how long the job may run. Zero means no limit.
type RunOptions struct {
	ID           string
	Timeout      time.Duration
	UseJobOutput bool
	DryRun       bool
	// Live receives combined stdout and stderr as it is produced.
	Live io.Writer
	// OnLine receives each output line without its trailing newline.
	OnLine func(string)
}

// RunResult is the outcome of an immediate cron run.
type RunResult struct {
	ID              string `json:"id"`
	User            string `json:"user"`
	Command         string `json:"command"`
	Disabled        bool   `json:"disabled"`
	StartedAt       string `json:"started_at"`
	FinishedAt      string `json:"finished_at"`
	DurationMS      int64  `json:"duration_ms"`
	ExitCode        *int   `json:"exit_code,omitempty"`
	TimedOut        bool   `json:"timed_out"`
	Output          string `json:"output"`
	OutputTruncated bool   `json:"output_truncated"`
	DryRun          bool   `json:"dry_run,omitempty"`
	// Argv is the command that was or would be executed. It is omitted from JSON.
	Argv []string `json:"-"`
}
