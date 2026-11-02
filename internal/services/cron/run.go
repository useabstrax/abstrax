package cron

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"sort"
	"sync"
	"syscall"
	"time"

	executil "abstrax/internal/exec"
)

const maxRunOutput = 64 * 1024

// executeArgv runs a command with a process-group timeout.
// Tests replace it to avoid switching users.
var executeArgv = runLimited

// runUserExists reports whether runuser is on PATH. Tests may replace it.
var runUserExists = executil.Exists

// lookupUser resolves a local account. Tests may replace it.
var lookupUser = user.Lookup

// Run executes a managed cron job immediately and returns the result.
// A disabled job still runs. Dry-run records the command and does not execute it.
func (s *Service) Run(ctx context.Context, opts RunOptions) (*RunResult, error) {
	if opts.Timeout < 0 {
		return nil, fmt.Errorf("timeout must be >= 0")
	}
	job, err := s.readJob(s.jobPath(opts.ID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("cron job %q not found", opts.ID)
		}
		return nil, err
	}

	argv := buildRunArgv(job, opts.UseJobOutput)
	started := time.Now().UTC()
	if s.runner != nil {
		s.runner.Note(argv[0], argv[1:]...)
	}
	if opts.DryRun {
		return finishRun(job, started, "", false, nil, false, true, argv), nil
	}

	if s.runner != nil {
		res, ok, callErr := s.runner.TryCommand(ctx, argv[0], argv[1:]...)
		if ok {
			if callErr != nil && res.ExitCode == 0 && res.Stdout == "" && res.Stderr == "" {
				return nil, callErr
			}
			output, truncated := tailOutput(res.Stdout + res.Stderr)
			code := res.ExitCode
			return finishRun(job, started, output, truncated, &code, false, false, argv), nil
		}
	}

	capture := newCapture(maxRunOutput, opts.Live, opts.OnLine)
	code, timedOut, err := executeArgv(ctx, argv, opts.Timeout, capture.stdout(), capture.stderr())
	capture.flush()
	if err != nil {
		return nil, err
	}
	output, truncated := capture.output()
	return finishRun(job, started, output, truncated, &code, timedOut, false, argv), nil
}

func finishRun(job *CronJob, started time.Time, output string, truncated bool, exitCode *int, timedOut, dryRun bool, argv []string) *RunResult {
	finished := time.Now().UTC()
	return &RunResult{
		ID:              job.ID,
		User:            job.User,
		Command:         job.Command,
		Disabled:        !job.Enabled,
		StartedAt:       started.Format(time.RFC3339),
		FinishedAt:      finished.Format(time.RFC3339),
		DurationMS:      finished.Sub(started).Milliseconds(),
		ExitCode:        exitCode,
		TimedOut:        timedOut,
		Output:          output,
		OutputTruncated: truncated,
		DryRun:          dryRun,
		Argv:            append([]string(nil), argv...),
	}
}

func buildRunArgv(job *CronJob, useJobOutput bool) []string {
	tool := "sudo"
	if runUserExists("runuser") {
		tool = "runuser"
	}
	argv := []string{tool, "-u", job.User, "--", "env", "-i"}
	argv = append(argv, envAssignments(job)...)
	argv = append(argv, "/bin/sh", "-c", commandScript(job, useJobOutput))
	return argv
}

func envAssignments(job *CronJob) []string {
	env := map[string]string{
		"SHELL": "/bin/sh",
		"PATH":  "/usr/bin:/bin",
		"HOME":  homeDir(job.User),
	}
	for k, v := range job.Env {
		env[k] = v
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	return out
}

func homeDir(name string) string {
	if u, err := lookupUser(name); err == nil && u != nil && u.HomeDir != "" {
		return u.HomeDir
	}
	if name == "root" {
		return "/root"
	}
	return "/home/" + name
}

func tailOutput(s string) (string, bool) {
	if len(s) <= maxRunOutput {
		return s, false
	}
	return s[len(s)-maxRunOutput:], true
}

// runLimited runs argv and kills its process group when timeout elapses.
// timeout of zero waits without a limit. The exit code is 124 on timeout.
func runLimited(ctx context.Context, argv []string, timeout time.Duration, stdout, stderr io.Writer) (int, bool, error) {
	if len(argv) == 0 {
		return 0, false, fmt.Errorf("empty command")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return 0, false, err
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	var timer <-chan time.Time
	if timeout > 0 {
		t := time.NewTimer(timeout)
		defer t.Stop()
		timer = t.C
	}

	select {
	case err := <-waitCh:
		return exitCodeOf(err), false, nil
	case <-timer:
		killGroup(cmd)
		<-waitCh
		return 124, true, nil
	case <-ctx.Done():
		killGroup(cmd)
		<-waitCh
		return 0, false, ctx.Err()
	}
}

func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}

func killGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	if pid <= 0 {
		return
	}
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		_ = cmd.Process.Kill()
	}
}

// captureWriter keeps the tail of combined output and optionally streams it.
type captureWriter struct {
	mu        sync.Mutex
	max       int
	tail      []byte
	truncated bool
	live      io.Writer
	onLine    func(string)
	streams   []*captureStream
}

type captureStream struct {
	parent  *captureWriter
	partial []byte
}

func newCapture(max int, live io.Writer, onLine func(string)) *captureWriter {
	if max <= 0 {
		max = maxRunOutput
	}
	return &captureWriter{max: max, live: live, onLine: onLine}
}

func (c *captureWriter) stdout() io.Writer {
	return c.stream()
}

func (c *captureWriter) stderr() io.Writer {
	return c.stream()
}

func (c *captureWriter) stream() io.Writer {
	s := &captureStream{parent: c}
	c.mu.Lock()
	c.streams = append(c.streams, s)
	c.mu.Unlock()
	return s
}

func (s *captureStream) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	parent := s.parent
	parent.mu.Lock()
	defer parent.mu.Unlock()

	parent.tail = append(parent.tail, p...)
	if len(parent.tail) > parent.max {
		parent.truncated = true
		extra := len(parent.tail) - parent.max
		parent.tail = append([]byte(nil), parent.tail[extra:]...)
	}
	if parent.live != nil {
		_, _ = parent.live.Write(p)
	}
	if parent.onLine == nil {
		return len(p), nil
	}
	s.partial = append(s.partial, p...)
	for {
		i := bytes.IndexByte(s.partial, '\n')
		if i < 0 {
			break
		}
		line := string(s.partial[:i])
		s.partial = append([]byte(nil), s.partial[i+1:]...)
		parent.onLine(line)
	}
	return len(p), nil
}

func (c *captureWriter) flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.onLine == nil {
		return
	}
	for _, s := range c.streams {
		if len(s.partial) == 0 {
			continue
		}
		c.onLine(string(s.partial))
		s.partial = nil
	}
}

func (c *captureWriter) output() (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return string(c.tail), c.truncated
}
