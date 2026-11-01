package serverinfo

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"

	executil "abstrax/internal/exec"
)

const needsRestartingMissing = "needs-restarting is not installed"

// RebootStatus reports whether the system should be rebooted.
type RebootStatus struct {
	Required bool     `json:"required"`
	Packages []string `json:"packages"`
	Note     string   `json:"note,omitempty"`
}

// RebootFiles are the Debian/Ubuntu reboot-required paths.
// Tests inject temporary paths; production uses DefaultRebootFiles.
type RebootFiles struct {
	Required string
	Packages string
}

// DefaultRebootFiles returns the Debian/Ubuntu reboot-required paths.
func DefaultRebootFiles() RebootFiles {
	return RebootFiles{
		Required: "/var/run/reboot-required",
		Packages: "/var/run/reboot-required.pkgs",
	}
}

// ReadDebianReboot reads the reboot-required flag and package list.
// A missing flag file means no reboot is required.
func ReadDebianReboot(paths RebootFiles) (RebootStatus, error) {
	if _, err := os.Stat(paths.Required); err != nil {
		if os.IsNotExist(err) {
			return emptyReboot(false), nil
		}
		return RebootStatus{}, err
	}

	pkgs, err := readRebootPackages(paths.Packages)
	if err != nil {
		if os.IsNotExist(err) {
			return emptyReboot(true), nil
		}
		return RebootStatus{}, err
	}
	status := emptyReboot(true)
	status.Packages = pkgs
	return status, nil
}

// ReadRHELReboot runs needs-restarting -r.
// Exit code 1 means a reboot is required. A missing binary is not a failure:
// Required is false and Note explains that needs-restarting is not installed.
func ReadRHELReboot(ctx context.Context, runner *executil.Runner) (RebootStatus, error) {
	if runner == nil {
		runner = executil.New(false, false)
	}
	res, err := runner.RunQuery(ctx, "needs-restarting", "-r")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			status := emptyReboot(false)
			status.Note = needsRestartingMissing
			return status, nil
		}
		if res.ExitCode == 1 {
			status := emptyReboot(true)
			status.Packages = parseNeedsRestartingPackages(res.Stdout)
			return status, nil
		}
		return RebootStatus{}, err
	}
	return emptyReboot(false), nil
}

func emptyReboot(required bool) RebootStatus {
	return RebootStatus{Required: required, Packages: []string{}}
}

func readRebootPackages(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return nonEmptyLines(string(data)), nil
}

func parseNeedsRestartingPackages(output string) []string {
	var pkgs []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "*") {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(line, "*"))
		fields := strings.Fields(name)
		if len(fields) == 0 {
			continue
		}
		pkgs = append(pkgs, fields[0])
	}
	if pkgs == nil {
		pkgs = []string{}
	}
	return pkgs
}

func nonEmptyLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	if lines == nil {
		lines = []string{}
	}
	return lines
}
