package daemon

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	pidRe = regexp.MustCompile(`pid\s+(\d+)`)

	// Higher rank is worse. status summary uses the worst process.
	statusRank = map[string]int{
		"RUNNING":  1,
		"STOPPED":  2,
		"STARTING": 3,
		"STOPPING": 4,
		"EXITED":   5,
		"BACKOFF":  6,
		"FATAL":    7,
	}
)

func knownStatus(s string) bool {
	_, ok := statusRank[s]
	return ok
}

func parseStatusOutput(output string) map[string][]ProcessInfo {
	grouped := map[string][]ProcessInfo{}
	for _, line := range strings.Split(output, "\n") {
		program, proc, ok := parseStatusLine(line)
		if !ok {
			continue
		}
		grouped[program] = append(grouped[program], proc)
	}
	return grouped
}

func parseStatusLine(line string) (string, ProcessInfo, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "Error") || strings.HasPrefix(line, "ERROR") {
		return "", ProcessInfo{}, false
	}
	fields := strings.Fields(line)
	stateIdx := -1
	for i, field := range fields {
		if knownStatus(field) {
			stateIdx = i
			break
		}
	}
	if stateIdx < 1 {
		return "", ProcessInfo{}, false
	}

	name := strings.Join(fields[:stateIdx], " ")
	desc := strings.Join(fields[stateIdx+1:], " ")
	proc := ProcessInfo{
		Name:        name,
		Status:      fields[stateIdx],
		Description: desc,
	}
	if proc.Status == "RUNNING" {
		if m := pidRe.FindStringSubmatch(desc); m != nil {
			proc.PID, _ = strconv.Atoi(m[1])
		}
		proc.Uptime, proc.UptimeSeconds = parseUptime(desc)
	}

	program := name
	if i := strings.Index(name, ":"); i >= 0 {
		program = name[:i]
	}
	return program, proc, true
}

func parseUptime(desc string) (string, int) {
	i := strings.Index(desc, "uptime ")
	if i < 0 {
		return "", 0
	}
	raw := strings.TrimSpace(desc[i+len("uptime "):])
	days := 0
	clock := raw
	if strings.Contains(raw, "day") {
		fmt.Sscanf(raw, "%d", &days)
		if comma := strings.Index(raw, ","); comma >= 0 {
			clock = strings.TrimSpace(raw[comma+1:])
		}
	}
	var h, m, s int
	if _, err := fmt.Sscanf(clock, "%d:%d:%d", &h, &m, &s); err != nil {
		return raw, 0
	}
	return raw, days*86400 + h*3600 + m*60 + s
}

func summarize(procs []ProcessInfo) string {
	if len(procs) == 0 {
		return "UNKNOWN"
	}
	best := procs[0].Status
	bestRank := statusRank[best]
	for _, proc := range procs[1:] {
		if statusRank[proc.Status] > bestRank {
			best = proc.Status
			bestRank = statusRank[proc.Status]
		}
	}
	return best
}

func runningCount(procs []ProcessInfo) (running, fatal int) {
	for _, proc := range procs {
		switch proc.Status {
		case "RUNNING":
			running++
		case "FATAL":
			fatal++
		}
	}
	return running, fatal
}
