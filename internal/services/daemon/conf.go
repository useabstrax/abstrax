package daemon

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"abstrax/internal/validate"
)

var exitCodesRe = regexp.MustCompile(`^\d{1,3}(,\d{1,3})*$`)

var stopSignals = map[string]struct{}{
	"TERM": {},
	"HUP":  {},
	"INT":  {},
	"QUIT": {},
	"KILL": {},
	"USR1": {},
	"USR2": {},
}

func readConf(path string) (AddOptions, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AddOptions{}, err
	}

	opts := AddOptions{
		Processes:    1,
		Autostart:    true,
		Autorestart:  "unexpected",
		StartSecs:    1,
		StartRetries: 3,
		StopSignal:   "TERM",
		StopWaitSecs: 10,
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[program:") && strings.HasSuffix(line, "]") {
			opts.Name = strings.TrimSuffix(strings.TrimPrefix(line, "[program:"), "]")
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = unescapeSupervisor(strings.TrimSpace(val))
		switch key {
		case "command":
			opts.Command = val
		case "directory":
			opts.Directory = val
		case "user":
			opts.User = val
		case "numprocs":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				opts.Processes = n
			}
		case "autostart":
			opts.Autostart = strings.EqualFold(val, "true")
		case "autorestart":
			opts.Autorestart = val
		case "startsecs":
			if n, err := strconv.Atoi(val); err == nil {
				opts.StartSecs = n
			}
		case "startretries":
			if n, err := strconv.Atoi(val); err == nil {
				opts.StartRetries = n
			}
		case "stopsignal":
			opts.StopSignal = val
		case "stopwaitsecs":
			if n, err := strconv.Atoi(val); err == nil {
				opts.StopWaitSecs = n
			}
		case "exitcodes":
			opts.ExitCodes = val
		case "stdout_logfile":
			opts.StdoutLogFile = val
		case "stderr_logfile":
			opts.StderrLogFile = val
		case "environment":
			opts.Environment = parseSupervisorEnv(val)
		}
	}
	if opts.Name == "" {
		return AddOptions{}, fmt.Errorf("supervisor config %s has no program name", path)
	}
	return opts, nil
}

func parseSupervisorEnv(s string) map[string]string {
	env := map[string]string{}
	var b strings.Builder
	inQuote := false
	var parts []string
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			b.WriteRune(r)
		case r == ',' && !inQuote:
			parts = append(parts, b.String())
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		parts = append(parts, b.String())
	}
	for _, part := range parts {
		key, val, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"`)
		env[strings.TrimSpace(key)] = val
	}
	if len(env) == 0 {
		return nil
	}
	return env
}

func mergeDaemonOptions(current, opts AddOptions) (AddOptions, error) {
	out := current
	out.Name = current.Name
	changed := opts.Changed
	if changed == nil {
		changed = map[string]bool{}
	}

	if changed["command"] {
		if strings.TrimSpace(opts.Command) == "" {
			return AddOptions{}, errEmpty("command")
		}
		out.Command = opts.Command
	}
	if changed["directory"] {
		out.Directory = opts.Directory
	}
	if changed["user"] {
		out.User = opts.User
	}
	if changed["processes"] {
		if opts.Processes < 1 {
			return AddOptions{}, errEmpty("processes")
		}
		out.Processes = opts.Processes
	}
	if changed["autostart"] {
		out.Autostart = opts.Autostart
	}
	if changed["autorestart"] {
		if err := validateAutorestart(opts.Autorestart); err != nil {
			return AddOptions{}, err
		}
		out.Autorestart = opts.Autorestart
	}
	if changed["startsecs"] {
		out.StartSecs = opts.StartSecs
	}
	if changed["startretries"] {
		out.StartRetries = opts.StartRetries
	}
	if changed["stopsignal"] {
		out.StopSignal = opts.StopSignal
	}
	if changed["stopwaitsecs"] {
		out.StopWaitSecs = opts.StopWaitSecs
	}
	if changed["exitcodes"] {
		out.ExitCodes = opts.ExitCodes
	}
	if changed["stdout-logfile"] {
		out.StdoutLogFile = opts.StdoutLogFile
	}
	if changed["stderr-logfile"] {
		out.StderrLogFile = opts.StderrLogFile
	}
	if changed["clear-environment"] {
		out.Environment = nil
	}
	if changed["environment"] {
		out.Environment = cloneEnv(out.Environment)
		for k, v := range opts.Environment {
			out.Environment[k] = v
		}
	}
	if changed["unset-environment"] {
		out.Environment = cloneEnv(out.Environment)
		for _, key := range opts.UnsetEnvironment {
			delete(out.Environment, key)
		}
	}
	return out, nil
}

func cloneEnv(env map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range env {
		out[k] = v
	}
	return out
}

func errEmpty(field string) error {
	return &fieldError{field: field}
}

type fieldError struct{ field string }

func (e *fieldError) Error() string {
	if e.field == "processes" {
		return "processes must be 1 or greater"
	}
	return e.field + " cannot be empty"
}

// ValidateOptions checks every value that would be written to a Supervisor config.
func ValidateOptions(opts AddOptions) error {
	if err := validate.DaemonName(opts.Name); err != nil {
		return err
	}
	if err := validate.SingleLine("command", opts.Command); err != nil {
		return err
	}
	if opts.Directory != "" {
		if err := validate.FilePath(opts.Directory); err != nil {
			return fmt.Errorf("directory: %w", err)
		}
	}
	if opts.User != "" {
		if err := validate.Username(opts.User); err != nil {
			return err
		}
	}
	switch opts.Autorestart {
	case "", "true", "false", "unexpected", "always":
	default:
		return fmt.Errorf("autorestart must be true, false, unexpected, or always, got %q", opts.Autorestart)
	}
	if opts.StopSignal != "" {
		if _, ok := stopSignals[opts.StopSignal]; !ok {
			return fmt.Errorf("stopsignal must be one of TERM, HUP, INT, QUIT, KILL, USR1, USR2, got %q", opts.StopSignal)
		}
	}
	if opts.ExitCodes != "" && !exitCodesRe.MatchString(opts.ExitCodes) {
		return fmt.Errorf("exitcodes must be a comma-separated list of numbers, got %q", opts.ExitCodes)
	}
	if opts.StdoutLogFile != "" {
		if err := validate.FilePath(opts.StdoutLogFile); err != nil {
			return fmt.Errorf("stdout-logfile: %w", err)
		}
	}
	if opts.StderrLogFile != "" {
		if err := validate.FilePath(opts.StderrLogFile); err != nil {
			return fmt.Errorf("stderr-logfile: %w", err)
		}
	}
	for key, value := range opts.Environment {
		if err := validate.SingleLine("environment", key); err != nil {
			return err
		}
		if err := validate.EnvKey(key); err != nil {
			return err
		}
		if err := validate.SingleLine("environment", value); err != nil {
			return err
		}
		if strings.ContainsAny(value, "\\\"") {
			return fmt.Errorf("environment value for %s cannot contain \" or \\", key)
		}
	}
	return nil
}

func formatEnvironment(env map[string]string) (string, error) {
	if len(env) == 0 {
		return "", nil
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, key, escapeSupervisor(env[key])))
	}
	return strings.Join(parts, ","), nil
}

func escapeSupervisor(s string) string {
	return strings.ReplaceAll(s, "%", "%%")
}

func unescapeSupervisor(s string) string {
	return strings.ReplaceAll(s, "%%", "%")
}

func applyConf(info *DaemonInfo, opts AddOptions) {
	info.Name = opts.Name
	info.Command = opts.Command
	info.Directory = opts.Directory
	info.User = opts.User
	info.Numprocs = opts.Processes
	info.Autostart = opts.Autostart
	info.Autorestart = opts.Autorestart
	info.StartSecs = opts.StartSecs
	info.StartRetries = opts.StartRetries
	info.StopSignal = opts.StopSignal
	info.StopWaitSecs = opts.StopWaitSecs
	info.ExitCodes = opts.ExitCodes
	info.StdoutLog = opts.StdoutLogFile
	info.StderrLog = opts.StderrLogFile
	info.Environment = opts.Environment
}
