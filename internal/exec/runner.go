// Package exec provides a safe wrapper around os/exec that supports dry-run
// mode, verbose logging, and structured output capturing.
package exec

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Result holds the output of a completed command.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Command  string
}

// Runner executes OS commands.
type Runner struct {
	dryRun  bool
	verbose bool
}

// New creates a Runner.
func New(dryRun, verbose bool) *Runner {
	return &Runner{dryRun: dryRun, verbose: verbose}
}

// Run executes a command and returns its output.
func (r *Runner) Run(ctx context.Context, name string, args ...string) (Result, error) {
	cmdStr := name + " " + strings.Join(args, " ")

	if r.verbose {
		fmt.Printf("[exec] %s\n", cmdStr)
	}

	if r.dryRun {
		fmt.Printf("[dry-run] would run: %s\n", cmdStr)
		return Result{Command: cmdStr}, nil
	}

	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	return Result{
		Stdout:   strings.TrimSpace(stdout.String()),
		Stderr:   strings.TrimSpace(stderr.String()),
		ExitCode: exitCode,
		Command:  cmdStr,
	}, err
}

// RunSilent executes a command without printing anything regardless of verbose.
func (r *Runner) RunSilent(ctx context.Context, name string, args ...string) (Result, error) {
	if r.dryRun {
		cmdStr := name + " " + strings.Join(args, " ")
		return Result{Command: cmdStr}, nil
	}

	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	return Result{
		Stdout:   strings.TrimSpace(stdout.String()),
		Stderr:   strings.TrimSpace(stderr.String()),
		ExitCode: exitCode,
		Command:  name + " " + strings.Join(args, " "),
	}, err
}

// Exists reports whether the named binary can be found in PATH.
func Exists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// Which returns the full path of a binary, or an empty string if not found.
func Which(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}

// systemctlAvailable caches the result of the systemctl connectivity check.
var systemctlChecked bool
var systemctlOK bool

// SystemctlWorks reports whether systemctl is installed AND can communicate
// with a running systemd instance.  The result is cached after the first call.
func SystemctlWorks() bool {
	if systemctlChecked {
		return systemctlOK
	}
	systemctlChecked = true
	if !Exists("systemctl") {
		return false
	}
	cmd := exec.Command("systemctl", "is-system-running")
	out, _ := cmd.Output()
	state := strings.TrimSpace(string(out))
	// "running", "degraded", "starting" all mean systemd is alive.
	systemctlOK = state == "running" || state == "degraded" || state == "starting" || state == "initializing"
	return systemctlOK
}
