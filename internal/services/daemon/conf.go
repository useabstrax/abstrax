package daemon

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
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
	if opts.Autorestart != "" && opts.Autorestart != "true" && opts.Autorestart != "false" && opts.Autorestart != "unexpected" {
		return fmt.Errorf("autorestart must be true, false or unexpected, got %q", opts.Autorestart)
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

func readConf(path string) (AddOptions, error) {
	f, err := os.Open(path)
	if err != nil {
		return AddOptions{}, err
	}
	defer f.Close()

	opts := AddOptions{Environment: map[string]string{}}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[program:") && strings.HasSuffix(line, "]") {
			opts.Name = strings.TrimSuffix(strings.TrimPrefix(line, "[program:"), "]")
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = unescapeSupervisor(value)
		switch key {
		case "command":
			opts.Command = value
		case "directory":
			opts.Directory = value
		case "user":
			opts.User = value
		case "numprocs":
			fmt.Sscan(value, &opts.Processes)
		case "autostart":
			opts.Autostart = value == "true"
		case "autorestart":
			opts.Autorestart = value
		case "startsecs":
			fmt.Sscan(value, &opts.StartSecs)
		case "startretries":
			fmt.Sscan(value, &opts.StartRetries)
		case "stopsignal":
			opts.StopSignal = value
		case "stopwaitsecs":
			fmt.Sscan(value, &opts.StopWaitSecs)
		case "exitcodes":
			opts.ExitCodes = value
		case "stdout_logfile":
			opts.StdoutLogFile = value
		case "stderr_logfile":
			opts.StderrLogFile = value
		case "environment":
			env, err := parseEnvironment(value)
			if err != nil {
				return AddOptions{}, err
			}
			opts.Environment = env
		}
	}
	return opts, scanner.Err()
}

func parseEnvironment(raw string) (map[string]string, error) {
	env := map[string]string{}
	rest := raw
	for rest != "" {
		key, after, ok := strings.Cut(rest, "=")
		if !ok || key == "" || !strings.HasPrefix(after, `"`) {
			return nil, fmt.Errorf("invalid environment entry %q", raw)
		}
		value, next, err := readQuoted(after)
		if err != nil {
			return nil, err
		}
		env[key] = value
		rest = next
		if rest == "" {
			break
		}
		if !strings.HasPrefix(rest, ",") {
			return nil, fmt.Errorf("invalid environment entry %q", raw)
		}
		rest = rest[1:]
	}
	return env, nil
}

func readQuoted(s string) (string, string, error) {
	if len(s) < 2 || s[0] != '"' {
		return "", "", fmt.Errorf("expected a quoted environment value")
	}
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			b.WriteByte(s[i+1])
			i++
			continue
		}
		if s[i] == '"' {
			return b.String(), s[i+1:], nil
		}
		b.WriteByte(s[i])
	}
	return "", "", fmt.Errorf("unterminated environment value")
}
