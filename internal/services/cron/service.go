// Package cron manages cron jobs using files in /etc/cron.d.
package cron

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"abstrax/internal/platform"
	"abstrax/internal/validate"
)

const (
	metaPrefix    = "# abstrax:cron"
	jobPrefix     = "# abstrax:job"
	headerComment = "# Managed by Abstrax - do not edit manually"
)

var envLineRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// jobDefinition is the JSON metadata line. It is the source of truth for
// fields that used to be folded into the shell command.
type jobDefinition struct {
	Command     string            `json:"command"`
	Schedule    string            `json:"schedule,omitempty"`
	WorkingDir  string            `json:"working_dir,omitempty"`
	Output      string            `json:"output,omitempty"`
	ErrorOutput string            `json:"error_output,omitempty"`
	Append      bool              `json:"append,omitempty"`
	Discard     bool              `json:"discard,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
}

// Service manages cron jobs.
type Service struct {
	cronDir string
}

// New creates a Service.
func New() *Service {
	return &Service{cronDir: platform.Resolve().Paths().CronDir}
}

// Add creates a new managed cron job.
func (s *Service) Add(_ context.Context, opts AddOptions) (*CronJob, error) {
	path := s.jobPath(opts.ID)

	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("cron job %q already exists; use 'cron modify' to update it", opts.ID)
	}

	if err := os.MkdirAll(s.cronDir, 0755); err != nil {
		return nil, fmt.Errorf("creating cron dir: %w", err)
	}

	job := &CronJob{
		ID:            opts.ID,
		User:          opts.User,
		Command:       opts.Command,
		Schedule:      opts.Schedule,
		Enabled:       opts.Enabled,
		FilePath:      path,
		Env:           opts.Env,
		WorkingDir:    opts.WorkingDir,
		Output:        opts.Output,
		ErrorOutput:   opts.ErrorOutput,
		AppendOutput:  opts.AppendOutput,
		DiscardOutput: opts.DiscardOutput,
	}

	if err := s.writeJob(path, job); err != nil {
		return nil, err
	}

	return job, nil
}

// Remove removes a managed cron job.
func (s *Service) Remove(_ context.Context, id string) error {
	path := s.jobPath(id)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("cron job %q does not exist", id)
	}

	return os.Remove(path)
}

// Modify updates an existing cron job.
func (s *Service) Modify(_ context.Context, opts ModifyOptions) (*CronJob, error) {
	path := s.jobPath(opts.ID)

	job, err := s.readJob(path)
	if err != nil {
		return nil, fmt.Errorf("cron job %q not found", opts.ID)
	}

	if opts.Changed != nil {
		applyModify(job, opts)
	} else {
		if opts.Command != "" {
			job.Command = opts.Command
		}
		if opts.Schedule != "" {
			job.Schedule = opts.Schedule
		}
		if opts.User != "" {
			job.User = opts.User
		}
		if opts.Output != "" {
			job.Output = opts.Output
		}
		if opts.ErrorOutput != "" {
			job.ErrorOutput = opts.ErrorOutput
		}
		if opts.WorkingDir != "" {
			job.WorkingDir = opts.WorkingDir
		}
		for k, v := range opts.Env {
			if job.Env == nil {
				job.Env = make(map[string]string)
			}
			job.Env[k] = v
		}
	}
	if err := s.writeJob(path, job); err != nil {
		return nil, err
	}

	return job, nil
}

// List returns all managed cron jobs.
func (s *Service) List(_ context.Context) ([]CronJob, error) {
	entries, err := os.ReadDir(s.cronDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var jobs []CronJob
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "abstrax-") {
			continue
		}
		job, err := s.readJob(filepath.Join(s.cronDir, e.Name()))
		if err != nil {
			continue
		}
		jobs = append(jobs, *job)
	}
	return jobs, nil
}

// Info returns a single cron job by ID.
func (s *Service) Info(_ context.Context, id string) (*CronJob, error) {
	return s.readJob(s.jobPath(id))
}

// Enable un-comments the schedule line of a cron job.
func (s *Service) Enable(_ context.Context, id string) error {
	return s.setEnabled(id, true)
}

// Disable comments out the schedule line of a cron job.
func (s *Service) Disable(_ context.Context, id string) error {
	return s.setEnabled(id, false)
}

func (s *Service) jobPath(id string) string {
	return filepath.Join(s.cronDir, "abstrax-"+id)
}

func applyModify(job *CronJob, opts ModifyOptions) {
	if opts.Changed["command"] {
		job.Command = opts.Command
	}
	if opts.Changed["schedule"] {
		job.Schedule = opts.Schedule
	}
	if opts.Changed["user"] {
		job.User = opts.User
	}
	if opts.Changed["working-dir"] {
		job.WorkingDir = opts.WorkingDir
	}
	if opts.Changed["no-output"] {
		job.Output = ""
		job.ErrorOutput = ""
		job.AppendOutput = false
		job.DiscardOutput = false
	}
	if opts.Changed["output"] {
		job.Output = opts.Output
		job.DiscardOutput = false
	}
	if opts.Changed["error-output"] {
		job.ErrorOutput = opts.ErrorOutput
	}
	if opts.Changed["append-output"] {
		job.AppendOutput = opts.AppendOutput
	}
	if opts.Changed["discard-output"] {
		job.DiscardOutput = opts.DiscardOutput
	}
	if opts.Changed["env"] {
		for k, v := range opts.Env {
			if job.Env == nil {
				job.Env = make(map[string]string)
			}
			job.Env[k] = v
		}
	}
}

func (s *Service) writeJob(path string, job *CronJob) error {
	if err := validateJob(job); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if job.Created == "" {
		job.Created = now
	}
	job.Updated = now

	def := jobDefinition{
		Command:     job.Command,
		Schedule:    job.Schedule,
		WorkingDir:  job.WorkingDir,
		Output:      job.Output,
		ErrorOutput: job.ErrorOutput,
		Append:      job.AppendOutput,
		Discard:     job.DiscardOutput,
		Env:         job.Env,
	}
	payload, err := json.Marshal(def)
	if err != nil {
		return fmt.Errorf("encoding cron job: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(headerComment + "\n")
	sb.WriteString(fmt.Sprintf("# abstrax:cron id=%s user=%s created=%s updated=%s\n",
		job.ID, job.User, job.Created, job.Updated))
	sb.WriteString(jobPrefix + " ")
	sb.Write(payload)
	sb.WriteString("\n")
	sb.WriteString("#\n")

	keys := make([]string, 0, len(job.Env))
	for k := range job.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sb.WriteString(k + "=" + job.Env[k] + "\n")
	}

	prefix := ""
	if !job.Enabled {
		prefix = "#DISABLED# "
	}
	sb.WriteString(fmt.Sprintf("%s%s %s %s\n", prefix, job.Schedule, job.User, buildCronCommand(job)))

	return os.WriteFile(path, []byte(sb.String()), 0644)
}

func buildCronCommand(job *CronJob) string {
	cmd := job.Command
	if job.WorkingDir != "" {
		cmd = "cd " + shellQuote(job.WorkingDir) + " && " + cmd
	}
	switch {
	case job.DiscardOutput:
		cmd += " > /dev/null 2>&1"
	case job.Output != "" && job.ErrorOutput != "":
		stdout, stderr := ">", "2>"
		if job.AppendOutput {
			stdout, stderr = ">>", "2>>"
		}
		cmd += " " + stdout + " " + shellQuote(job.Output) + " " + stderr + " " + shellQuote(job.ErrorOutput)
	case job.Output != "":
		op := ">"
		if job.AppendOutput {
			op = ">>"
		}
		cmd += " " + op + " " + shellQuote(job.Output) + " 2>&1"
	case job.ErrorOutput != "":
		op := "2>"
		if job.AppendOutput {
			op = "2>>"
		}
		cmd += " " + op + " " + shellQuote(job.ErrorOutput)
	}
	return escapeCronPercent(cmd)
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n\"'\\$`!#&|;<>(){}[]*?~") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func (s *Service) readJob(path string) (*CronJob, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	job := &CronJob{
		FilePath: path,
		Enabled:  true,
		Env:      make(map[string]string),
	}

	var def jobDefinition
	hasDef := false

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, metaPrefix) {
			parseMetaLine(line, job)
			continue
		}
		if strings.HasPrefix(line, jobPrefix) {
			raw := strings.TrimSpace(strings.TrimPrefix(line, jobPrefix))
			if err := json.Unmarshal([]byte(raw), &def); err == nil {
				hasDef = true
			}
			continue
		}

		if strings.HasPrefix(line, "#DISABLED#") {
			job.Enabled = false
			line = strings.TrimPrefix(line, "#DISABLED# ")
		}

		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}

		if envLineRe.MatchString(line) {
			parts := strings.SplitN(line, "=", 2)
			job.Env[parts[0]] = parts[1]
			continue
		}

		if !hasDef {
			applyCronLine(line, job)
		}
	}

	if hasDef {
		job.Command = def.Command
		if def.Schedule != "" {
			job.Schedule = def.Schedule
		}
		job.WorkingDir = def.WorkingDir
		job.Output = def.Output
		job.ErrorOutput = def.ErrorOutput
		job.AppendOutput = def.Append
		job.DiscardOutput = def.Discard
		if def.Env != nil {
			job.Env = def.Env
		}
	}

	// Extract ID from path if not found in meta.
	if job.ID == "" {
		base := filepath.Base(path)
		job.ID = strings.TrimPrefix(base, "abstrax-")
	}

	return job, scanner.Err()
}

func parseMetaLine(line string, job *CronJob) {
	rest := strings.TrimPrefix(line, metaPrefix)
	for _, part := range strings.Fields(rest) {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "id":
			job.ID = kv[1]
		case "user":
			job.User = kv[1]
		case "created":
			job.Created = kv[1]
		case "updated":
			job.Updated = kv[1]
		}
	}
}

func applyCronLine(line string, job *CronJob) {
	parts := strings.Fields(line)
	if len(parts) >= 3 && strings.HasPrefix(parts[0], "@") {
		job.Schedule = parts[0]
		job.User = parts[1]
		job.Command = unescapeCronPercent(strings.Join(parts[2:], " "))
		return
	}
	if len(parts) >= 7 {
		job.Schedule = strings.Join(parts[:5], " ")
		job.User = parts[5]
		job.Command = unescapeCronPercent(strings.Join(parts[6:], " "))
	}
}

func validateJob(job *CronJob) error {
	fields := []struct {
		name  string
		value string
	}{
		{"command", job.Command},
		{"user", job.User},
		{"schedule", job.Schedule},
		{"working-dir", job.WorkingDir},
		{"output", job.Output},
		{"error-output", job.ErrorOutput},
	}
	for _, field := range fields {
		if err := validate.SingleLine(field.name, field.value); err != nil {
			return err
		}
	}
	if err := validate.Username(job.User); err != nil {
		return err
	}
	for key, value := range job.Env {
		if err := validate.SingleLine("env", key); err != nil {
			return err
		}
		if err := validate.EnvKey(key); err != nil {
			return err
		}
		if err := validate.SingleLine("env", value); err != nil {
			return err
		}
	}
	return nil
}

func escapeCronPercent(s string) string {
	return strings.ReplaceAll(s, "%", `\%`)
}

func unescapeCronPercent(s string) string {
	return strings.ReplaceAll(s, `\%`, "%")
}

func (s *Service) setEnabled(id string, enabled bool) error {
	path := s.jobPath(id)

	job, err := s.readJob(path)
	if err != nil {
		return fmt.Errorf("cron job %q not found", id)
	}

	if job.Enabled == enabled {
		state := "enabled"
		if !enabled {
			state = "disabled"
		}
		return fmt.Errorf("cron job %q is already %s", id, state)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	var out []string
	for _, line := range lines {
		body := strings.TrimPrefix(line, "#DISABLED# ")
		disabled := body != line
		if isScheduleLine(body) {
			if enabled {
				out = append(out, body)
			} else if disabled {
				out = append(out, line)
			} else {
				out = append(out, "#DISABLED# "+body)
			}
			continue
		}
		out = append(out, line)
	}

	return os.WriteFile(path, []byte(strings.Join(out, "\n")), 0644)
}

func isScheduleLine(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return false
	}
	if envLineRe.MatchString(line) {
		return false
	}
	fields := strings.Fields(line)
	if len(fields) >= 3 && strings.HasPrefix(fields[0], "@") {
		return true
	}
	return len(fields) >= 7
}
