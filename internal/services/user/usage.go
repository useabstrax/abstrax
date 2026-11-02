package user

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// cronEnvLine matches crontab environment assignments such as MAILTO=ops.
var cronEnvLine = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// supervisorSuffixes are always scanned together. Debian writes .conf and
// RHEL writes .ini (SupervisorConfigExtension); a host can contain either.
func supervisorSuffixes(extra string) []string {
	suffixes := []string{".conf", ".ini"}
	if extra == "" {
		return suffixes
	}
	if !strings.HasPrefix(extra, ".") {
		extra = "." + extra
	}
	for _, suffix := range suffixes {
		if suffix == extra {
			return suffixes
		}
	}
	return append(suffixes, extra)
}

func (s *Service) stillUsedBy(ctx context.Context, username string) ([]UsedBy, error) {
	daemons, err := s.daemonUsage(username)
	if err != nil {
		return nil, err
	}
	crons, err := s.cronUsage(username)
	if err != nil {
		return nil, err
	}
	procs, err := s.processUsage(ctx, username)
	if err != nil {
		return nil, err
	}
	used := make([]UsedBy, 0, len(daemons)+len(crons)+len(procs))
	used = append(used, daemons...)
	used = append(used, crons...)
	used = append(used, procs...)
	return used, nil
}

func (s *Service) daemonUsage(username string) ([]UsedBy, error) {
	files, err := abstraxFiles(s.supervisorDir, supervisorSuffixes(s.supervisorExt))
	if err != nil {
		return nil, err
	}
	var used []UsedBy
	for _, path := range files {
		name, procs, ok, err := daemonReferences(path, username)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		used = append(used, UsedBy{Type: "daemon", Name: name, Processes: procs})
	}
	sort.Slice(used, func(i, j int) bool { return used[i].Name < used[j].Name })
	return used, nil
}

func daemonReferences(path, username string) (name string, procs int, ok bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, false, fmt.Errorf("reading daemon config %s: %w", path, err)
	}
	procs = 1
	var userVal string
	hasUser := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[program:") && strings.HasSuffix(line, "]") {
			name = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "[program:"), "]"))
			continue
		}
		key, val, cut := strings.Cut(line, "=")
		if !cut {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		switch key {
		case "user":
			userVal = val
			hasUser = true
		case "numprocs":
			if n, convErr := strconv.Atoi(val); convErr == nil && n > 0 {
				procs = n
			}
		}
	}
	if !hasUser || userVal != username {
		return "", 0, false, nil
	}
	if name == "" {
		base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		name = strings.TrimPrefix(base, "abstrax-")
	}
	if name == "" {
		return "", 0, false, nil
	}
	return name, procs, true, nil
}

func (s *Service) cronUsage(username string) ([]UsedBy, error) {
	files, err := abstraxFiles(s.cronDir, nil)
	if err != nil {
		return nil, err
	}
	var used []UsedBy
	for _, path := range files {
		match, err := cronFileReferences(path, username)
		if err != nil {
			return nil, err
		}
		if !match {
			continue
		}
		id := strings.TrimPrefix(filepath.Base(path), "abstrax-")
		if id == "" {
			continue
		}
		used = append(used, UsedBy{Type: "cron", Name: id})
	}
	sort.Slice(used, func(i, j int) bool { return used[i].Name < used[j].Name })
	return used, nil
}

func cronFileReferences(path, username string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("reading cron file %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		user, ok := cronRunUser(scanner.Text())
		if ok && user == username {
			return true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("reading cron file %s: %w", path, err)
	}
	return false, nil
}

// cronRunUser returns the user field from a /etc/cron.d schedule line.
// Comments and environment assignments are ignored.
// A line is "minute hour day month weekday user command" or "@daily user command".
func cronRunUser(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", false
	}
	if cronEnvLine.MatchString(line) {
		return "", false
	}
	fields := strings.Fields(line)
	if len(fields) >= 3 && strings.HasPrefix(fields[0], "@") {
		return fields[1], true
	}
	if len(fields) >= 7 {
		return fields[5], true
	}
	return "", false
}

func (s *Service) processUsage(ctx context.Context, username string) ([]UsedBy, error) {
	var procs []Process
	var err error
	if s.processes != nil {
		procs, err = s.processes(ctx, username)
	} else {
		procs, err = s.processesFromPS(ctx, username)
	}
	if err != nil {
		return nil, err
	}
	used := make([]UsedBy, 0, len(procs))
	for _, proc := range procs {
		if proc.PID <= 0 {
			continue
		}
		used = append(used, UsedBy{
			Type: "process",
			PID:  proc.PID,
			Args: strings.TrimSpace(proc.Args),
		})
	}
	sort.Slice(used, func(i, j int) bool { return used[i].PID < used[j].PID })
	return used, nil
}

// processesFromPS lists processes with `ps -u <user> -o pid=,args=`.
// ps exits 1 when the user has no processes; that is an empty result.
// The runner is not used, so a dry-run still sees live processes, and unit
// tests inject ProcessLister instead of depending on this command.
func (s *Service) processesFromPS(ctx context.Context, username string) ([]Process, error) {
	cmd := exec.CommandContext(ctx, "ps", "-u", username, "-o", "pid=,args=")
	out, err := cmd.Output()
	text := strings.TrimSpace(string(out))
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && text == "" {
			return nil, nil
		}
		if text == "" {
			return nil, fmt.Errorf("listing processes for %s: %w", username, err)
		}
	}
	return parsePS(text), nil
}

func parsePS(out string) []Process {
	var procs []Process
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pidStr, args, _ := strings.Cut(line, " ")
		pid, err := strconv.Atoi(pidStr)
		if err != nil || pid <= 0 {
			continue
		}
		procs = append(procs, Process{PID: pid, Args: strings.TrimSpace(args)})
	}
	return procs
}

func abstraxFiles(dir string, suffixes []string) ([]string, error) {
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", dir, err)
	}
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "abstrax-") {
			continue
		}
		if len(suffixes) > 0 && !hasSuffix(name, suffixes) {
			continue
		}
		paths = append(paths, filepath.Join(dir, name))
	}
	sort.Strings(paths)
	return paths, nil
}

func hasSuffix(name string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func (s *Service) writeUsedBy(username string, used []UsedBy) {
	w := s.out
	if w == nil {
		w = os.Stdout
	}
	fmt.Fprintln(w, formatUsedBy(username, used))
}

func formatUsedBy(username string, used []UsedBy) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s is still used by:\n", username)
	for i, item := range used {
		if i > 0 {
			b.WriteByte('\n')
		}
		switch item.Type {
		case "daemon":
			fmt.Fprintf(&b, "  daemon %s (%s)", item.Name, processCount(item.Processes))
		case "cron":
			fmt.Fprintf(&b, "  cron job %s", item.Name)
		case "process":
			if item.Args != "" {
				fmt.Fprintf(&b, "  process %d %s", item.PID, item.Args)
			} else {
				fmt.Fprintf(&b, "  process %d", item.PID)
			}
		default:
			fmt.Fprintf(&b, "  %s %s", item.Type, item.Name)
		}
	}
	return b.String()
}

func processCount(n int) string {
	if n < 1 {
		n = 1
	}
	if n == 1 {
		return "1 process"
	}
	return fmt.Sprintf("%d processes", n)
}
