// Package daemon manages background processes using Supervisor.
package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"abstrax/internal/backup"
	executil "abstrax/internal/exec"
	"abstrax/internal/platform/debian"
	"abstrax/internal/services/pkgmanager"
)

// Service manages daemons via Supervisor.
type Service struct {
	runner  *executil.Runner
	confDir string
}

// New creates a Service.
func New(dryRun, verbose bool) *Service {
	return &Service{
		runner:  executil.New(dryRun, verbose),
		confDir: debian.SupervisorConfDir,
	}
}

// Add creates a new Supervisor-managed daemon.
func (s *Service) Add(ctx context.Context, opts AddOptions) (*DaemonInfo, error) {
	if !executil.Exists("supervisorctl") {
		if opts.InstallSupervisor {
			mgr := pkgmanager.NewApt(false, false)
			if err := mgr.Install(ctx, pkgmanager.InstallOptions{Name: "supervisor"}); err != nil {
				return nil, fmt.Errorf("installing supervisor: %w", err)
			}
			if _, err := executil.New(false, false).Run(ctx, "systemctl", "enable", "--now", "supervisor"); err != nil {
				return nil, fmt.Errorf("enabling supervisor: %w", err)
			}
		} else {
			return nil, fmt.Errorf("supervisor is not installed; pass --install-supervisor to install it")
		}
	}

	path := s.confPath(opts.Name)

	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("daemon %q already exists; use 'daemon modify' to update it", opts.Name)
	}

	if err := os.MkdirAll(s.confDir, 0755); err != nil {
		return nil, fmt.Errorf("creating supervisor conf dir: %w", err)
	}

	if err := s.writeConf(path, opts); err != nil {
		return nil, err
	}

	if err := s.rereadUpdate(ctx); err != nil {
		return nil, err
	}

	return &DaemonInfo{
		Name:       opts.Name,
		ConfigPath: path,
	}, nil
}

// Remove removes a daemon.
func (s *Service) Remove(ctx context.Context, opts RemoveOptions) error {
	path := s.confPath(opts.Name)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("daemon %q does not exist", opts.Name)
	}

	if opts.Stop {
		_, _ = s.runner.Run(ctx, "supervisorctl", "stop", opts.Name)
	}

	if _, err := backup.File(path); err != nil {
		return err
	}

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("removing daemon config: %w", err)
	}

	return s.rereadUpdate(ctx)
}

// Modify updates an existing daemon's configuration.
func (s *Service) Modify(ctx context.Context, opts AddOptions) (*DaemonInfo, error) {
	path := s.confPath(opts.Name)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("daemon %q does not exist", opts.Name)
	}

	if _, err := backup.File(path); err != nil {
		return nil, err
	}

	if err := s.writeConf(path, opts); err != nil {
		return nil, err
	}

	if err := s.rereadUpdate(ctx); err != nil {
		return nil, err
	}

	return &DaemonInfo{Name: opts.Name, ConfigPath: path}, nil
}

// Start starts a daemon.
func (s *Service) Start(ctx context.Context, name string) error {
	_, err := s.runner.Run(ctx, "supervisorctl", "start", name)
	return err
}

// Stop stops a daemon.
func (s *Service) Stop(ctx context.Context, name string) error {
	_, err := s.runner.Run(ctx, "supervisorctl", "stop", name)
	return err
}

// Restart restarts a daemon.
func (s *Service) Restart(ctx context.Context, name string) error {
	_, err := s.runner.Run(ctx, "supervisorctl", "restart", name)
	return err
}

// Status returns the status of a daemon.
func (s *Service) Status(ctx context.Context, name string) (*DaemonInfo, error) {
	res, err := s.runner.RunSilent(ctx, "supervisorctl", "status", name)
	if err != nil {
		return nil, fmt.Errorf("supervisor status %s: %w", name, err)
	}

	info := &DaemonInfo{Name: name, ConfigPath: s.confPath(name)}
	parts := strings.Fields(res.Stdout)
	if len(parts) >= 2 {
		info.Status = parts[1]
	}
	if len(parts) >= 4 {
		info.Description = strings.Join(parts[2:], " ")
	}
	return info, nil
}

// List returns all supervisor-managed daemons.
func (s *Service) List(ctx context.Context) ([]DaemonInfo, error) {
	res, err := s.runner.RunSilent(ctx, "supervisorctl", "status")
	if err != nil && res.ExitCode == 0 {
		return nil, fmt.Errorf("supervisor status: %w", err)
	}

	var daemons []DaemonInfo
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		d := DaemonInfo{ConfigPath: s.confPath(parts[0])}
		if len(parts) >= 1 {
			d.Name = parts[0]
		}
		if len(parts) >= 2 {
			d.Status = parts[1]
		}
		if len(parts) >= 3 {
			d.Description = strings.Join(parts[2:], " ")
		}
		daemons = append(daemons, d)
	}
	return daemons, nil
}

// Logs returns the log output for a daemon.
func (s *Service) Logs(ctx context.Context, opts LogOptions) (string, error) {
	if opts.Follow {
		_, err := s.runner.Run(ctx, "supervisorctl", "tail", "-f", opts.Name)
		return "", err
	}

	lines := 50
	if opts.Lines > 0 {
		lines = opts.Lines
	}
	res, err := s.runner.RunSilent(ctx, "supervisorctl", "tail",
		fmt.Sprintf("-%d", lines), opts.Name)
	if err != nil {
		return "", err
	}
	return res.Stdout, nil
}

func (s *Service) confPath(name string) string {
	return filepath.Join(s.confDir, "abstrax-"+name+".conf")
}

func (s *Service) rereadUpdate(ctx context.Context) error {
	if _, err := s.runner.Run(ctx, "supervisorctl", "reread"); err != nil {
		return fmt.Errorf("supervisor reread: %w", err)
	}
	if _, err := s.runner.Run(ctx, "supervisorctl", "update"); err != nil {
		return fmt.Errorf("supervisor update: %w", err)
	}
	return nil
}

func (s *Service) writeConf(path string, opts AddOptions) error {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[program:%s]\n", opts.Name))
	sb.WriteString(fmt.Sprintf("command=%s\n", opts.Command))

	if opts.Directory != "" {
		sb.WriteString(fmt.Sprintf("directory=%s\n", opts.Directory))
	}
	if opts.User != "" {
		sb.WriteString(fmt.Sprintf("user=%s\n", opts.User))
	}

	numprocs := opts.Processes
	if numprocs < 1 {
		numprocs = 1
	}
	sb.WriteString(fmt.Sprintf("numprocs=%d\n", numprocs))

	if numprocs > 1 {
		sb.WriteString(fmt.Sprintf("process_name=%%(program_name)s_%%(process_num)02d\n"))
	}

	autostart := "true"
	if !opts.Autostart {
		autostart = "false"
	}
	sb.WriteString(fmt.Sprintf("autostart=%s\n", autostart))

	autorestart := opts.Autorestart
	if autorestart == "" {
		autorestart = "unexpected"
	}
	sb.WriteString(fmt.Sprintf("autorestart=%s\n", autorestart))

	startsecs := opts.StartSecs
	if startsecs == 0 {
		startsecs = 1
	}
	sb.WriteString(fmt.Sprintf("startsecs=%d\n", startsecs))

	startretries := opts.StartRetries
	if startretries == 0 {
		startretries = 3
	}
	sb.WriteString(fmt.Sprintf("startretries=%d\n", startretries))

	stopsignal := opts.StopSignal
	if stopsignal == "" {
		stopsignal = "TERM"
	}
	sb.WriteString(fmt.Sprintf("stopsignal=%s\n", stopsignal))

	stopwait := opts.StopWaitSecs
	if stopwait == 0 {
		stopwait = 10
	}
	sb.WriteString(fmt.Sprintf("stopwaitsecs=%d\n", stopwait))

	if opts.ExitCodes != "" {
		sb.WriteString(fmt.Sprintf("exitcodes=%s\n", opts.ExitCodes))
	}

	if opts.StdoutLogFile != "" {
		sb.WriteString(fmt.Sprintf("stdout_logfile=%s\n", opts.StdoutLogFile))
	} else {
		sb.WriteString(fmt.Sprintf("stdout_logfile=/var/log/supervisor/%s-stdout.log\n", opts.Name))
	}

	if opts.StderrLogFile != "" {
		sb.WriteString(fmt.Sprintf("stderr_logfile=%s\n", opts.StderrLogFile))
	} else {
		sb.WriteString(fmt.Sprintf("stderr_logfile=/var/log/supervisor/%s-stderr.log\n", opts.Name))
	}

	if len(opts.Environment) > 0 {
		var envParts []string
		for k, v := range opts.Environment {
			envParts = append(envParts, fmt.Sprintf(`%s="%s"`, k, v))
		}
		sb.WriteString(fmt.Sprintf("environment=%s\n", strings.Join(envParts, ",")))
	}

	return os.WriteFile(path, []byte(sb.String()), 0644)
}
