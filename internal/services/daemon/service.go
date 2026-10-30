// Package daemon manages background processes using Supervisor.
package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	executil "abstrax/internal/exec"
	"abstrax/internal/platform"
	"abstrax/internal/services/pkgmanager"
	"abstrax/internal/services/svcmanager"
)

// Service manages daemons via Supervisor.
type Service struct {
	runner  *executil.Runner
	confDir string
	confExt string
	svcName string
	pkgName string
}

// New creates a Service.
func New(dryRun, verbose bool) *Service {
	provider := platform.Resolve()
	return &Service{
		runner:  executil.New(dryRun, verbose),
		confDir: provider.SupervisorConfigDir(),
		confExt: provider.SupervisorConfigExtension(),
		svcName: provider.SupervisorServiceName(),
		pkgName: provider.SupervisorPackage(),
	}
}

func errSupervisorMissing() error {
	return fmt.Errorf("supervisor is not installed; run `sudo abstrax daemon install`")
}

// supervisorInstalled reports whether supervisorctl is on PATH.
// Tests replace it.
var supervisorInstalled = executil.Exists

func (s *Service) requireSupervisor() error {
	if supervisorInstalled("supervisorctl") {
		return nil
	}
	return errSupervisorMissing()
}

func supervisorTarget(name string) string {
	return name + ":*"
}

// Install installs Supervisor and enables/starts its service.
func (s *Service) Install(ctx context.Context, opts InstallOptions) (*InstallResult, error) {
	result := &InstallResult{
		Package: s.pkgName,
		Service: s.svcName,
	}
	if executil.Exists("supervisorctl") {
		result.AlreadyInstalled = true
		return result, nil
	}

	fmt.Printf("Installing %s...\n", s.pkgName)
	mgr, _, err := pkgmanager.NewFromPlatform(opts.DryRun, false)
	if err != nil {
		return nil, err
	}
	if err := mgr.Install(ctx, pkgmanager.InstallOptions{Name: s.pkgName, DryRun: opts.DryRun}); err != nil {
		return nil, fmt.Errorf("installing %s: %w", s.pkgName, err)
	}

	svcMgr := svcmanager.New(opts.DryRun, false)
	if err := svcMgr.Enable(ctx, s.svcName); err != nil {
		return nil, err
	}
	if err := svcMgr.Start(ctx, s.svcName); err != nil {
		return nil, err
	}

	if !opts.DryRun && !executil.Exists("supervisorctl") {
		return nil, fmt.Errorf("installed %s but supervisorctl was not found on PATH", s.pkgName)
	}
	return result, nil
}

// Add creates a new Supervisor-managed daemon.
func (s *Service) Add(ctx context.Context, opts AddOptions) (*DaemonInfo, error) {
	if !executil.Exists("supervisorctl") {
		if opts.InstallSupervisor {
			if _, err := s.Install(ctx, InstallOptions{DryRun: opts.DryRun}); err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("supervisor is not installed; run `sudo abstrax daemon install` (or pass --install-supervisor)")
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
	if err := s.requireSupervisor(); err != nil {
		return err
	}

	path := s.confPath(opts.Name)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("daemon %q does not exist", opts.Name)
	}

	if opts.Stop {
		if _, err := s.runner.Run(ctx, "supervisorctl", "stop", supervisorTarget(opts.Name)); err != nil && !opts.Force {
			return fmt.Errorf("stopping daemon %q: %w", opts.Name, err)
		}
	}

	if opts.DeleteLogs {
		stdoutLog, stderrLog := logPathsFromConf(path, opts.Name)
		_ = os.Remove(stdoutLog)
		_ = os.Remove(stderrLog)
	}

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("removing daemon config: %w", err)
	}

	return s.rereadUpdate(ctx)
}

// Modify updates an existing daemon's configuration.
func (s *Service) Modify(ctx context.Context, opts AddOptions) (*DaemonInfo, error) {
	if err := s.requireSupervisor(); err != nil {
		return nil, err
	}

	path := s.confPath(opts.Name)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("daemon %q does not exist", opts.Name)
	}

	if err := s.writeConf(path, opts); err != nil {
		return nil, err
	}

	if err := s.rereadUpdate(ctx); err != nil {
		return nil, err
	}

	return &DaemonInfo{Name: opts.Name, ConfigPath: path}, nil
}

// Start starts a daemon. name:* addresses a single process and a group.
func (s *Service) Start(ctx context.Context, name string) error {
	return s.control(ctx, "start", name)
}

// Stop stops a daemon.
func (s *Service) Stop(ctx context.Context, name string) error {
	return s.control(ctx, "stop", name)
}

// Restart restarts a daemon.
func (s *Service) Restart(ctx context.Context, name string) error {
	return s.control(ctx, "restart", name)
}

func (s *Service) control(ctx context.Context, action, name string) error {
	if err := s.requireSupervisor(); err != nil {
		return err
	}
	_, err := s.runner.Run(ctx, "supervisorctl", action, supervisorTarget(name))
	return err
}

// Status returns one daemon and each of its processes.
func (s *Service) Status(ctx context.Context, name string) (*DaemonInfo, error) {
	if err := s.requireSupervisor(); err != nil {
		return nil, err
	}

	res, err := s.runner.RunSilent(ctx, "supervisorctl", "status", supervisorTarget(name))
	if err != nil && strings.TrimSpace(res.Stdout) == "" {
		return nil, fmt.Errorf("supervisor status %s: %w", name, err)
	}

	grouped := parseStatusOutput(res.Stdout)
	procs := grouped[name]
	if len(procs) == 0 {
		return nil, fmt.Errorf("daemon %q not found", name)
	}
	info := &DaemonInfo{
		Name:       name,
		Status:     summarize(procs),
		Processes:  procs,
		ConfigPath: s.confPath(name),
		Managed:    s.isManaged(name),
	}
	if !info.Managed {
		info.ConfigPath = ""
	}
	return info, nil
}

// List returns daemons. By default only Abstrax-managed configs are included.
func (s *Service) List(ctx context.Context, opts ListOptions) ([]DaemonInfo, error) {
	if err := s.requireSupervisor(); err != nil {
		return nil, err
	}

	res, err := s.runner.RunSilent(ctx, "supervisorctl", "status")
	if err != nil && strings.TrimSpace(res.Stdout) == "" {
		return nil, fmt.Errorf("supervisor status: %w", err)
	}

	grouped := parseStatusOutput(res.Stdout)
	managed := s.managedConfigs()

	names := map[string]struct{}{}
	if opts.All {
		for name := range grouped {
			names[name] = struct{}{}
		}
	}
	for name := range managed {
		names[name] = struct{}{}
	}

	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)

	var daemons []DaemonInfo
	for _, name := range ordered {
		procs := grouped[name]
		info := DaemonInfo{
			Name:      name,
			Status:    summarize(procs),
			Processes: procs,
		}
		if path, ok := managed[name]; ok {
			info.Managed = true
			info.ConfigPath = path
		}
		daemons = append(daemons, info)
	}
	return daemons, nil
}

// Info returns the configured settings and, when Supervisor is available, live status.
func (s *Service) Info(ctx context.Context, name string) (*DaemonInfo, error) {
	path := s.confPath(name)
	opts, err := readConf(path)
	if err != nil {
		return nil, fmt.Errorf("daemon %q does not exist", name)
	}
	info := &DaemonInfo{
		Managed:    true,
		ConfigPath: path,
	}
	applyConf(info, opts)
	if supervisorInstalled("supervisorctl") {
		if live, err := s.Status(ctx, name); err == nil {
			info.Status = live.Status
			info.Processes = live.Processes
		}
	}
	return info, nil
}

// Logs returns the last N log lines, or follows the log.
func (s *Service) Logs(ctx context.Context, opts LogOptions) (string, error) {
	if err := s.requireSupervisor(); err != nil {
		return "", err
	}
	if opts.Process < 0 {
		return "", fmt.Errorf("process number must be 0 or greater")
	}

	conf, confErr := readConf(s.confPath(opts.Name))
	numprocs := 1
	if confErr == nil && conf.Processes > 0 {
		numprocs = conf.Processes
	}
	if numprocs > 1 && opts.Process >= numprocs {
		return "", fmt.Errorf("process %d is out of range (0-%d)", opts.Process, numprocs-1)
	}

	stream := ""
	switch {
	case opts.Stderr && !opts.Stdout:
		stream = "stderr"
	case opts.Stdout && !opts.Stderr:
		stream = "stdout"
	}
	process := opts.Name
	if numprocs > 1 {
		process = fmt.Sprintf("%s:%s_%02d", opts.Name, opts.Name, opts.Process)
	}

	if opts.Follow {
		args := []string{"tail", "-f", process}
		if stream != "" {
			args = append(args, stream)
		}
		_, err := s.runner.Run(ctx, "supervisorctl", args...)
		return "", err
	}

	lines := 50
	if opts.Lines > 0 {
		lines = opts.Lines
	}
	stdoutPath, stderrPath := logPathsFromConf(s.confPath(opts.Name), opts.Name)
	path := stdoutPath
	if stream == "stderr" {
		path = stderrPath
	}
	return tailLines(path, lines)
}

func (s *Service) managedConfigs() map[string]string {
	out := map[string]string{}
	entries, err := os.ReadDir(s.confDir)
	if err != nil {
		return out
	}
	ext := s.confExt
	if ext == "" {
		ext = ".conf"
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "abstrax-") || !strings.HasSuffix(name, ext) {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(name, "abstrax-"), ext)
		if id == "" {
			continue
		}
		out[id] = filepath.Join(s.confDir, name)
	}
	return out
}

func (s *Service) isManaged(name string) bool {
	_, ok := s.managedConfigs()[name]
	return ok
}

func (s *Service) confPath(name string) string {
	ext := s.confExt
	if ext == "" {
		ext = ".conf"
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return filepath.Join(s.confDir, "abstrax-"+name+ext)
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

func logPathsFromConf(path, name string) (stdout, stderr string) {
	stdout = fmt.Sprintf("/var/log/supervisor/%s-stdout.log", name)
	stderr = fmt.Sprintf("/var/log/supervisor/%s-stderr.log", name)

	data, err := os.ReadFile(path)
	if err != nil {
		return stdout, stderr
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "stdout_logfile="):
			stdout = strings.TrimPrefix(line, "stdout_logfile=")
		case strings.HasPrefix(line, "stderr_logfile="):
			stderr = strings.TrimPrefix(line, "stderr_logfile=")
		}
	}
	return stdout, stderr
}
