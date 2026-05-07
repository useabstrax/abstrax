package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"abstrax/internal/actions"
	"abstrax/internal/globals"
	"abstrax/internal/output"
	"abstrax/internal/platform"
	"abstrax/internal/services/svcmanager"
	"abstrax/internal/validate"
)

// NewServiceCmd returns the service command.
func NewServiceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Manage system services (systemd)",
	}

	cmd.AddCommand(newSvcStartCmd())
	cmd.AddCommand(newSvcStopCmd())
	cmd.AddCommand(newSvcRestartCmd())
	cmd.AddCommand(newSvcReloadCmd())
	cmd.AddCommand(newSvcEnableCmd())
	cmd.AddCommand(newSvcDisableCmd())
	cmd.AddCommand(newSvcStatusCmd())

	return cmd
}

func newSvcStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <name>",
		Short: "Start a service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSvcAction(cmd, args[0], actions.ServiceStart, "started",
				func(svc *svcmanager.Service, name string) error { return svc.Start(cmd.Context(), name) })
		},
	}
}

func newSvcStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop <name>",
		Short: "Stop a service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSvcAction(cmd, args[0], actions.ServiceStop, "stopped",
				func(svc *svcmanager.Service, name string) error { return svc.Stop(cmd.Context(), name) })
		},
	}
}

func newSvcRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart <name>",
		Short: "Restart a service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSvcAction(cmd, args[0], actions.ServiceRestart, "restarted",
				func(svc *svcmanager.Service, name string) error { return svc.Restart(cmd.Context(), name) })
		},
	}
}

func newSvcReloadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reload <name>",
		Short: "Reload a service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSvcAction(cmd, args[0], actions.ServiceReload, "reloaded",
				func(svc *svcmanager.Service, name string) error { return svc.Reload(cmd.Context(), name) })
		},
	}
}

func newSvcEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <name>",
		Short: "Enable a service to start at boot",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSvcAction(cmd, args[0], actions.ServiceEnable, "enabled",
				func(svc *svcmanager.Service, name string) error { return svc.Enable(cmd.Context(), name) })
		},
	}
}

func newSvcDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <name>",
		Short: "Disable a service from starting at boot",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSvcAction(cmd, args[0], actions.ServiceDisable, "disabled",
				func(svc *svcmanager.Service, name string) error { return svc.Disable(cmd.Context(), name) })
		},
	}
}

func newSvcStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <name>",
		Short: "Show service status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validate.ServiceName(name); err != nil {
				return err
			}

			svc := svcmanager.New(false, globals.Flags.Verbose)
			st, err := svc.Status(cmd.Context(), name)
			if err != nil {
				return err
			}

			p := printer()
			if globals.Flags.JSON {
				output.PrintJSON(output.Success(actions.ServiceStatus, "", st))
				return nil
			}

			p.Line("")
			p.Line("  %-14s %s", "Service:", st.Name)
			p.Line("  %-14s %s", "Active:", st.Active)
			p.Line("  %-14s %s", "Sub:", st.Sub)
			p.Line("  %-14s %s", "Enabled:", st.Enabled)
			if st.PID != "" && st.PID != "0" {
				p.Line("  %-14s %s", "PID:", st.PID)
			}
			if st.Description != "" {
				p.Line("  %-14s %s", "Description:", st.Description)
			}
			p.Line("")
			return nil
		},
	}
}

// runSvcAction executes a service lifecycle command.
func runSvcAction(
	cmd *cobra.Command,
	name, action, verb string,
	fn func(*svcmanager.Service, string) error,
) error {
	if err := validate.ServiceName(name); err != nil {
		return err
	}
	if err := platform.RequireRoot(); err != nil {
		return err
	}

	svc := svcmanager.New(globals.Flags.DryRun, globals.Flags.Verbose)
	if err := fn(svc, name); err != nil {
		return err
	}

	return printSimpleResult(action,
		fmt.Sprintf("Service %s %s.", name, verb), nil)
}
