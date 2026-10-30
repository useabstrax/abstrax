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

// CommandFunc handles a command instead of executing it.
// ok reports whether the function handled the command. When ok is false,
// the runner falls through to dry-run or real execution.
type CommandFunc func(ctx context.Context, name string, args []string) (res Result, ok bool, err error)

// Runner executes OS commands.
type Runner struct {
	dryRun      bool
	verbose     bool
	commands    [][]string
	commandFunc CommandFunc
}

// New creates a Runner.
func New(dryRun, verbose bool) *Runner {
	return &Runner{dryRun: dryRun, verbose: verbose}
}

// SetCommandFunc installs a test or dry-run hook. See CommandFunc.
func (r *Runner) SetCommandFunc(fn CommandFunc) {
	r.commandFunc = fn
}

// Commands returns every command passed to Run or RunSilent, including
// commands answered by a CommandFunc or skipped in dry-run.
func (r *Runner) Commands() [][]string {
	out := make([][]string, len(r.commands))
	for i, cmd := range r.commands {
		out[i] = append([]string(nil), cmd...)
	}
	return out
}

func (r *Runner) record(name string, args []string) {
	r.commands = append(r.commands, append([]string{name}, args...))
}

func (r *Runner) handled(ctx context.Context, name string, args []string) (Result, bool, error) {
	if r.commandFunc == nil {
		return Result{}, false, nil
	}
	res, ok, err := r.commandFunc(ctx, name, args)
	if !ok {
		return Result{}, false, nil
	}
	return res, true, err
}

// Run executes a command and returns its output.
func (r *Runner) Run(ctx context.Context, name string, args ...string) (Result, error) {
	r.record(name, args)
	if res, ok, err := r.handled(ctx, name, args); ok {
		return res, err
	}

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
	r.record(name, args)
	if res, ok, err := r.handled(ctx, name, args); ok {
		return res, err
	}

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
