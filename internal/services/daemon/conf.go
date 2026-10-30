package daemon

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

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
		val = strings.TrimSpace(val)
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
