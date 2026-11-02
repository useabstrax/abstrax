package serverinfo

import (
	"context"
	"fmt"
	"strings"

	executil "abstrax/internal/exec"
)

// RebootOptions selects how the server is rebooted.
type RebootOptions struct {
	// DelaySeconds waits before rebooting. Zero reboots immediately.
	// A negative value is an error.
	DelaySeconds int
	// Exists reports whether a helper binary is on PATH.
	// Nil uses exec.Exists.
	Exists func(name string) bool
}

// RebootResult is returned before the reboot command runs.
type RebootResult struct {
	Scheduled    bool   `json:"scheduled"`
	DelaySeconds int    `json:"delay_seconds"`
	Command      string `json:"command"`
}

type rebootCommands struct {
	journal []string
	reboot  []string
	display string
}

// NewReboot creates a Service for server reboot.
// Status and the other read-only server commands keep using New.
func NewReboot(dryRun, verbose bool) *Service {
	return &Service{runner: executil.New(dryRun, verbose)}
}

// Reboot writes a journal line, calls beforeReboot so the caller can print
// the result, then runs the reboot command. beforeReboot is invoked only
// after the journal line is recorded and before the reboot command.
func (s *Service) Reboot(ctx context.Context, opts RebootOptions, beforeReboot func(*RebootResult) error) (*RebootResult, error) {
	plan, err := rebootPlan(opts.DelaySeconds, opts.exists)
	if err != nil {
		return nil, err
	}

	if err := s.runArgv(ctx, plan.journal); err != nil {
		return nil, fmt.Errorf("writing reboot journal: %w", err)
	}

	res := &RebootResult{
		Scheduled:    true,
		DelaySeconds: opts.DelaySeconds,
		Command:      plan.display,
	}
	if beforeReboot != nil {
		if err := beforeReboot(res); err != nil {
			return nil, err
		}
	}

	if err := s.runArgv(ctx, plan.reboot); err != nil {
		return nil, fmt.Errorf("rebooting server: %w", err)
	}
	return res, nil
}

func (o RebootOptions) exists(name string) bool {
	if o.Exists != nil {
		return o.Exists(name)
	}
	return executil.Exists(name)
}

func (s *Service) runArgv(ctx context.Context, argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("empty command")
	}
	_, err := s.runner.Run(ctx, argv[0], argv[1:]...)
	return err
}

// rebootPlan chooses the journal and reboot commands.
// A delay uses systemd-run --on-active=<seconds>s when systemd-run exists,
// and otherwise shutdown -r with the delay rounded up to whole minutes.
func rebootPlan(delaySeconds int, exists func(string) bool) (rebootCommands, error) {
	if delaySeconds < 0 {
		return rebootCommands{}, fmt.Errorf("delay must be zero or greater")
	}
	if exists == nil {
		exists = executil.Exists
	}

	plan := rebootCommands{
		journal: journalArgv(delaySeconds, exists("systemd-cat")),
	}
	if delaySeconds == 0 {
		plan.reboot = []string{"systemctl", "reboot"}
		plan.display = "systemctl reboot"
		return plan, nil
	}
	if exists("systemd-run") {
		onActive := fmt.Sprintf("--on-active=%ds", delaySeconds)
		plan.reboot = []string{"systemd-run", onActive, "--", "systemctl", "reboot"}
		plan.display = strings.Join(plan.reboot, " ")
		return plan, nil
	}

	minutes := shutdownDelayMinutes(delaySeconds)
	plus := fmt.Sprintf("+%d", minutes)
	plan.reboot = []string{"shutdown", "-r", plus}
	plan.display = "shutdown -r " + plus
	return plan, nil
}

func journalArgv(delaySeconds int, systemdCat bool) []string {
	message := "Restart requested through Abstrax"
	if delaySeconds > 0 {
		message = fmt.Sprintf("Restart requested through Abstrax in %d seconds", delaySeconds)
	}
	if systemdCat {
		return []string{"systemd-cat", "-t", "abstrax", "--", "echo", message}
	}
	return []string{"logger", "-t", "abstrax", message}
}

// shutdownDelayMinutes rounds seconds up to a whole number of minutes.
// shutdown -r +N only accepts minutes. 90 seconds becomes 2.
func shutdownDelayMinutes(seconds int) int {
	if seconds <= 0 {
		return 0
	}
	return (seconds + 59) / 60
}
