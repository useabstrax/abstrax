package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"abstrax/internal/actions"
	"abstrax/internal/globals"
	"abstrax/internal/output"
	"abstrax/internal/services/daemon"
	"abstrax/internal/validate"
)

// NewDaemonCmd returns the daemon command.
func NewDaemonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Manage background processes (Supervisor)",
	}

	cmd.AddCommand(newDaemonInstallCmd())
	cmd.AddCommand(newDaemonAddCmd())
	cmd.AddCommand(newDaemonRemoveCmd())
	cmd.AddCommand(newDaemonModifyCmd())
	cmd.AddCommand(newDaemonStartCmd())
	cmd.AddCommand(newDaemonStopCmd())
	cmd.AddCommand(newDaemonRestartCmd())
	cmd.AddCommand(newDaemonStatusCmd())
	cmd.AddCommand(newDaemonListCmd())
	cmd.AddCommand(newDaemonInfoCmd())
	cmd.AddCommand(newDaemonLogsCmd())

	return cmd
}

func newDaemonInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install Supervisor",
		Long: `Install Supervisor and enable/start its service.

  Debian/Ubuntu: package supervisor, service supervisor
  Rocky/Alma/RHEL: package supervisor, service supervisord

After install, manage processes with abstrax daemon add/list/start/stop.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireRootAndSupported(); err != nil {
				return err
			}

			svc := daemon.New(globals.Flags.DryRun, globals.Flags.Verbose)
			result, err := svc.Install(cmd.Context(), daemon.InstallOptions{
				DryRun: globals.Flags.DryRun,
			})
			if err != nil {
				return err
			}

			msg := fmt.Sprintf("%s installed.", result.Package)
			if result.AlreadyInstalled {
				msg = fmt.Sprintf("%s is already installed.", result.Package)
			}
			return printSimpleResult(actions.DaemonInstall, msg, result)
		},
	}
}

func newDaemonAddCmd() *cobra.Command {
	opts := daemon.AddOptions{
		Autostart:   true,
		Autorestart: "unexpected",
	}
	var envPairs []string

	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a new managed daemon",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Name = args[0]
			opts.DryRun = globals.Flags.DryRun

			if err := validate.DaemonName(opts.Name); err != nil {
				return err
			}
			if err := requireRootAndSupported(); err != nil {
				return err
			}
			if opts.Command == "" {
				return fmt.Errorf("--command is required")
			}

			opts.Environment = parseEnvPairs(envPairs)

			svc := daemon.New(opts.DryRun, globals.Flags.Verbose)
			info, err := svc.Add(cmd.Context(), opts)
			if err != nil {
				return err
			}

			p := printer()
			r := output.Success(actions.DaemonAdd,
				fmt.Sprintf("Daemon %s added.", opts.Name), info)

			if machineOutput() {
				emitResult(r)
				return nil
			}

			p.Success("Daemon %s added.", opts.Name)
			p.Line("  Config: %s", info.ConfigPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.Command, "command", "", "Command to run")
	cmd.Flags().StringVar(&opts.Directory, "directory", "", "Working directory")
	cmd.Flags().StringVar(&opts.User, "user", "", "User to run as")
	cmd.Flags().IntVar(&opts.Processes, "processes", 1, "Number of processes")
	cmd.Flags().BoolVar(&opts.Autostart, "autostart", true, "Start automatically on supervisor start")
	cmd.Flags().BoolVar(&opts.InstallSupervisor, "install-supervisor", false, "Install supervisor if not present")
	cmd.Flags().StringVar(&opts.Autorestart, "autorestart", "unexpected", "Autorestart mode (always/unexpected/false)")
	cmd.Flags().IntVar(&opts.StartSecs, "startsecs", 1, "Seconds before considering process started")
	cmd.Flags().IntVar(&opts.StartRetries, "startretries", 3, "Number of start retries")
	cmd.Flags().StringVar(&opts.StopSignal, "stopsignal", "TERM", "Stop signal")
	cmd.Flags().IntVar(&opts.StopWaitSecs, "stopwaitsecs", 10, "Seconds to wait for process to stop")
	cmd.Flags().StringVar(&opts.ExitCodes, "exitcodes", "", "Expected exit codes")
	cmd.Flags().StringVar(&opts.StdoutLogFile, "stdout-logfile", "", "Stdout log file path")
	cmd.Flags().StringVar(&opts.StderrLogFile, "stderr-logfile", "", "Stderr log file path")
	cmd.Flags().StringArrayVar(&envPairs, "environment", nil, "Environment variables (KEY=VALUE)")

	return cmd
}

func newDaemonRemoveCmd() *cobra.Command {
	opts := daemon.RemoveOptions{}

	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a daemon",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Name = args[0]
			opts.DryRun = globals.Flags.DryRun

			if err := validate.DaemonName(opts.Name); err != nil {
				return err
			}
			if err := requireRootAndSupported(); err != nil {
				return err
			}

			svc := daemon.New(opts.DryRun, globals.Flags.Verbose)
			if err := svc.Remove(cmd.Context(), opts); err != nil {
				return err
			}

			return printSimpleResult(actions.DaemonRemove,
				fmt.Sprintf("Daemon %s removed.", opts.Name), nil)
		},
	}

	cmd.Flags().BoolVar(&opts.Stop, "stop", true, "Stop daemon before removing")
	cmd.Flags().BoolVar(&opts.DeleteLogs, "delete-logs", false, "Delete log files")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "Force removal")

	return cmd
}

func newDaemonModifyCmd() *cobra.Command {
	opts := daemon.AddOptions{}
	var envPairs []string

	cmd := &cobra.Command{
		Use:   "modify <name>",
		Short: "Modify a daemon's configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Name = args[0]
			opts.DryRun = globals.Flags.DryRun

			if err := validate.DaemonName(opts.Name); err != nil {
				return err
			}
			if err := requireRootAndSupported(); err != nil {
				return err
			}

			opts.Environment = parseEnvPairs(envPairs)

			svc := daemon.New(opts.DryRun, globals.Flags.Verbose)
			info, err := svc.Modify(cmd.Context(), opts)
			if err != nil {
				return err
			}

			return printSimpleResult(actions.DaemonModify,
				fmt.Sprintf("Daemon %s updated.", info.Name), info)
		},
	}

	cmd.Flags().StringVar(&opts.Command, "command", "", "Command to run")
	cmd.Flags().StringVar(&opts.Directory, "directory", "", "Working directory")
	cmd.Flags().StringVar(&opts.User, "user", "", "User to run as")
	cmd.Flags().IntVar(&opts.Processes, "processes", 0, "Number of processes")
	cmd.Flags().StringArrayVar(&envPairs, "environment", nil, "Environment variables (KEY=VALUE)")

	return cmd
}

func newDaemonStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <name>",
		Short: "Start a daemon",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := requireRootAndSupported(); err != nil {
				return err
			}
			svc := daemon.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.Start(cmd.Context(), name); err != nil {
				return err
			}
			return printSimpleResult(actions.DaemonStart,
				fmt.Sprintf("Daemon %s started.", name), nil)
		},
	}
}

func newDaemonStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop <name>",
		Short: "Stop a daemon",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := requireRootAndSupported(); err != nil {
				return err
			}
			svc := daemon.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.Stop(cmd.Context(), name); err != nil {
				return err
			}
			return printSimpleResult(actions.DaemonStop,
				fmt.Sprintf("Daemon %s stopped.", name), nil)
		},
	}
}

func newDaemonRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart <name>",
		Short: "Restart a daemon",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := requireRootAndSupported(); err != nil {
				return err
			}
			svc := daemon.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.Restart(cmd.Context(), name); err != nil {
				return err
			}
			return printSimpleResult(actions.DaemonRestart,
				fmt.Sprintf("Daemon %s restarted.", name), nil)
		},
	}
}

func newDaemonStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <name>",
		Short: "Show daemon status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			svc := daemon.New(false, globals.Flags.Verbose)
			info, err := svc.Status(cmd.Context(), name)
			if err != nil {
				return err
			}

			p := printer()
			if machineOutput() {
				emitResult(output.Success(actions.DaemonStatus, "", info))
				return nil
			}

			p.Line("")
			p.Line("  %-12s %s", "Name:", info.Name)
			p.Line("  %-12s %s", "Status:", info.Status)
			p.Line("  %-12s %s", "Processes:", formatProcessSummary(*info))
			if info.ConfigPath != "" {
				p.Line("  %-12s %s", "Config:", info.ConfigPath)
			}
			for _, proc := range info.Processes {
				detail := proc.Description
				if detail == "" {
					detail = proc.Status
				}
				p.Line("    %s  %s", proc.Name, detail)
			}
			p.Line("")
			return nil
		},
	}
}

func newDaemonListCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List managed daemons",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := daemon.New(false, globals.Flags.Verbose)
			daemons, err := svc.List(cmd.Context(), daemon.ListOptions{All: all})
			if err != nil {
				return err
			}

			if machineOutput() {
				emitResult(output.Success(actions.DaemonList, "", daemons))
				return nil
			}

			if len(daemons) == 0 {
				if all {
					printer().Line("No daemons running.")
				} else {
					printer().Line("No managed daemons.")
				}
				return nil
			}

			t := output.NewTable([]string{"NAME", "STATUS", "PROCESSES"})
			for _, d := range daemons {
				t.Append([]string{d.Name, d.Status, formatProcessSummary(d)})
			}
			t.Render()
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "Include Supervisor programs Abstrax did not create")
	return cmd
}

func newDaemonInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <name>",
		Short: "Show a managed daemon's settings and status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validate.DaemonName(name); err != nil {
				return err
			}
			svc := daemon.New(false, globals.Flags.Verbose)
			info, err := svc.Info(cmd.Context(), name)
			if err != nil {
				return err
			}
			if machineOutput() {
				emitResult(output.Success(actions.DaemonInfo, "", info))
				return nil
			}
			p := printer()
			p.Line("")
			p.Line("  %-12s %s", "Name:", info.Name)
			p.Line("  %-12s %s", "Status:", info.Status)
			p.Line("  %-12s %s", "Command:", info.Command)
			p.Line("  %-12s %s", "Directory:", info.Directory)
			p.Line("  %-12s %s", "User:", info.User)
			p.Line("  %-12s %d", "Processes:", info.Numprocs)
			p.Line("  %-12s %s", "Running:", formatProcessSummary(*info))
			p.Line("  %-12s %t", "Autostart:", info.Autostart)
			p.Line("  %-12s %s", "Autorestart:", info.Autorestart)
			p.Line("  %-12s %s", "Config:", info.ConfigPath)
			p.Line("")
			return nil
		},
	}
}

func formatProcessSummary(info daemon.DaemonInfo) string {
	total := len(info.Processes)
	if total == 0 {
		return info.Status
	}
	running := 0
	fatal := 0
	for _, proc := range info.Processes {
		switch proc.Status {
		case "RUNNING":
			running++
		case "FATAL":
			fatal++
		}
	}
	summary := fmt.Sprintf("%d/%d", running, total)
	if fatal > 0 {
		summary += fmt.Sprintf("  (%d FATAL)", fatal)
	}
	return summary
}

func newDaemonLogsCmd() *cobra.Command {
	opts := daemon.LogOptions{}

	cmd := &cobra.Command{
		Use:   "logs <name>",
		Short: "Show daemon logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Name = args[0]

			svc := daemon.New(false, globals.Flags.Verbose)
			logs, err := svc.Logs(cmd.Context(), opts)
			if err != nil {
				return err
			}

			if machineOutput() {
				emitResult(output.Success(actions.DaemonLogs, "",
					map[string]string{"output": logs}))
				return nil
			}

			if strings.TrimSpace(logs) == "" {
				printer().Line("No log output.")
				return nil
			}
			fmt.Print(logs)
			return nil
		},
	}

	cmd.Flags().IntVar(&opts.Lines, "lines", 50, "Number of lines to show")
	cmd.Flags().IntVar(&opts.Process, "process", 0, "Process number when the daemon runs more than one")
	cmd.Flags().BoolVar(&opts.Follow, "follow", false, "Follow log output")
	cmd.Flags().BoolVar(&opts.Stderr, "stderr", false, "Show stderr log")
	cmd.Flags().BoolVar(&opts.Stdout, "stdout", false, "Show stdout log")

	return cmd
}
