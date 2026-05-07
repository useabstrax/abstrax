// Package cron manages cron jobs using files in /etc/cron.d.
package cron

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"abstrax/internal/backup"
	"abstrax/internal/platform/debian"
)

const (
	metaPrefix    = "# abstrax:cron"
	headerComment = "# Managed by Abstrax – do not edit manually"
)

// Service manages cron jobs.
type Service struct {
	cronDir string
}

// New creates a Service.
func New() *Service {
	return &Service{cronDir: debian.CronDir}
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
		ID:       opts.ID,
		User:     opts.User,
		Command:  opts.Command,
		Schedule: opts.Schedule,
		Enabled:  opts.Enabled,
		FilePath: path,
		Env:      opts.Env,
	}

	if err := s.writeJob(path, job, opts); err != nil {
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

	if _, err := backup.File(path); err != nil {
		return fmt.Errorf("backing up cron file: %w", err)
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

	if opts.Command != "" {
		job.Command = opts.Command
	}
	if opts.Schedule != "" {
		job.Schedule = opts.Schedule
	}
	if opts.User != "" {
		job.User = opts.User
	}
	for k, v := range opts.Env {
		if job.Env == nil {
			job.Env = make(map[string]string)
		}
		job.Env[k] = v
	}

	if _, err := backup.File(path); err != nil {
		return nil, err
	}

	addOpts := AddOptions{
		ID:       job.ID,
		User:     job.User,
		Command:  job.Command,
		Schedule: job.Schedule,
		Enabled:  job.Enabled,
		Env:      job.Env,
	}
	if err := s.writeJob(path, job, addOpts); err != nil {
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

func (s *Service) writeJob(path string, job *CronJob, opts AddOptions) error {
	var sb strings.Builder
	sb.WriteString(headerComment + "\n")
	sb.WriteString(fmt.Sprintf("# abstrax:cron id=%s user=%s created=%s\n",
		job.ID, job.User, time.Now().Format(time.RFC3339)))
	sb.WriteString("#\n")

	for k, v := range opts.Env {
		sb.WriteString(fmt.Sprintf("%s=%s\n", k, v))
	}

	output := opts.Output
	if opts.DiscardOutput {
		output = "/dev/null"
	}

	cmd := job.Command
	if opts.WorkingDir != "" {
		cmd = fmt.Sprintf("cd %s && %s", opts.WorkingDir, cmd)
	}
	if output != "" {
		if opts.AppendOutput {
			cmd = fmt.Sprintf("%s >> %s 2>&1", cmd, output)
		} else {
			cmd = fmt.Sprintf("%s > %s 2>&1", cmd, output)
		}
	} else if opts.ErrorOutput != "" {
		cmd = fmt.Sprintf("%s 2>> %s", cmd, opts.ErrorOutput)
	}

	prefix := ""
	if !job.Enabled {
		prefix = "#DISABLED# "
	}

	sb.WriteString(fmt.Sprintf("%s%s %s %s\n", prefix, job.Schedule, job.User, cmd))

	return os.WriteFile(path, []byte(sb.String()), 0644)
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

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, metaPrefix) {
			parseMetaLine(line, job)
			continue
		}

		if strings.HasPrefix(line, "#DISABLED#") {
			job.Enabled = false
			line = strings.TrimPrefix(line, "#DISABLED# ")
		}

		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}

		// env line: KEY=VALUE
		if strings.Contains(line, "=") && !strings.Contains(line, " ") {
			parts := strings.SplitN(line, "=", 2)
			job.Env[parts[0]] = parts[1]
			continue
		}

		// cron line: schedule (5 fields) + user + command
		parts := strings.Fields(line)
		if len(parts) >= 7 {
			job.Schedule = strings.Join(parts[:5], " ")
			job.User = parts[5]
			job.Command = strings.Join(parts[6:], " ")
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
		}
	}
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

	if _, err := backup.File(path); err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	var out []string
	for _, line := range lines {
		if enabled {
			out = append(out, strings.TrimPrefix(line, "#DISABLED# "))
		} else if !strings.HasPrefix(line, "#") && strings.TrimSpace(line) != "" {
			// Check if it looks like a cron entry.
			parts := strings.Fields(line)
			if len(parts) >= 7 {
				out = append(out, "#DISABLED# "+line)
				continue
			}
		}
		out = append(out, line)
	}

	return os.WriteFile(path, []byte(strings.Join(out, "\n")), 0644)
}
