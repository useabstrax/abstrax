package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"abstrax/internal/actions"
	"abstrax/internal/globals"
	"abstrax/internal/output"
	"abstrax/internal/platform"
	"abstrax/internal/services/sshcfg"
	"abstrax/internal/validate"
)

// NewSSHCmd returns the ssh command.
func NewSSHCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ssh",
		Short: "Manage SSH server configuration",
	}

	cfgCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage sshd configuration",
	}

	cfgCmd.AddCommand(newSSHConfigShowCmd())
	cfgCmd.AddCommand(newSSHConfigSetPortCmd())
	cfgCmd.AddCommand(newSSHConfigSetTimeoutCmd())
	cfgCmd.AddCommand(newSSHConfigDisableRootLoginCmd())
	cfgCmd.AddCommand(newSSHConfigEnableRootLoginCmd())
	cfgCmd.AddCommand(newSSHConfigDisablePasswordAuthCmd())
	cfgCmd.AddCommand(newSSHConfigEnablePasswordAuthCmd())

	cmd.AddCommand(cfgCmd)
	cmd.AddCommand(newSSHReloadCmd())
	cmd.AddCommand(newSSHRestartCmd())

	return cmd
}

func newSSHConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show current SSH configuration managed by Abstrax",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := sshcfg.New(false, globals.Flags.Verbose)
			cfg, err := svc.Show(cmd.Context())
			if err != nil {
				return err
			}

			p := printer()
			r := output.Success(actions.SSHConfigShow, "", cfg)

			if globals.Flags.JSON {
				output.PrintJSON(r)
				return nil
			}

			p.Line("")
			p.Line("  %-28s %s", "Port:", cfg.Port)
			p.Line("  %-28s %s", "PermitRootLogin:", cfg.PermitRootLogin)
			p.Line("  %-28s %s", "PasswordAuthentication:", cfg.PasswordAuth)
			p.Line("  %-28s %s", "ClientAliveInterval:", cfg.ClientAliveInterval)
			p.Line("")
			return nil
		},
	}
}

func newSSHConfigSetPortCmd() *cobra.Command {
	var allowFirewall bool

	cmd := &cobra.Command{
		Use:   "set-port <port>",
		Short: "Change the SSH listening port",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validate.PortString(args[0]); err != nil {
				return err
			}
			if err := platform.RequireRoot(); err != nil {
				return err
			}

			var port int
			fmt.Sscanf(args[0], "%d", &port)

			p := printer()
			p.Warn("Changing the SSH port can lock you out if the firewall is not updated.")

			svc := sshcfg.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.SetPort(cmd.Context(), sshcfg.SetPortOptions{
				Port:          port,
				AllowFirewall: allowFirewall,
				DryRun:        globals.Flags.DryRun,
			}); err != nil {
				return err
			}

			return printSimpleResult(actions.SSHConfigSetPort,
				fmt.Sprintf("SSH port set to %d.", port), nil)
		},
	}

	cmd.Flags().BoolVar(&allowFirewall, "allow-firewall", false, "Open new port in firewall if available")
	return cmd
}

func newSSHConfigSetTimeoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-timeout <seconds>",
		Short: "Set SSH client alive interval (idle timeout)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var seconds int
			if _, err := fmt.Sscanf(args[0], "%d", &seconds); err != nil {
				return fmt.Errorf("invalid timeout value %q", args[0])
			}
			if err := platform.RequireRoot(); err != nil {
				return err
			}

			svc := sshcfg.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.SetTimeout(cmd.Context(), sshcfg.SetTimeoutOptions{
				Seconds: seconds,
				DryRun:  globals.Flags.DryRun,
			}); err != nil {
				return err
			}

			return printSimpleResult(actions.SSHConfigSetTimeout,
				fmt.Sprintf("SSH timeout set to %d seconds.", seconds), nil)
		},
	}
}

func newSSHConfigDisableRootLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable-root-login",
		Short: "Disable root SSH login",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			svc := sshcfg.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.DisableRootLogin(cmd.Context(), globals.Flags.DryRun); err != nil {
				return err
			}
			return printSimpleResult(actions.SSHConfigDisableRootLogin, "Root SSH login disabled.", nil)
		},
	}
}

func newSSHConfigEnableRootLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable-root-login",
		Short: "Enable root SSH login",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			svc := sshcfg.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.EnableRootLogin(cmd.Context(), globals.Flags.DryRun); err != nil {
				return err
			}
			return printSimpleResult(actions.SSHConfigEnableRootLogin, "Root SSH login enabled.", nil)
		},
	}
}

func newSSHConfigDisablePasswordAuthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable-password-auth",
		Short: "Disable SSH password authentication",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			printer().Warn("Ensure you have a working SSH key before disabling password auth.")
			svc := sshcfg.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.DisablePasswordAuth(cmd.Context(), globals.Flags.DryRun); err != nil {
				return err
			}
			return printSimpleResult(actions.SSHConfigDisablePasswdAuth, "SSH password authentication disabled.", nil)
		},
	}
}

func newSSHConfigEnablePasswordAuthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable-password-auth",
		Short: "Enable SSH password authentication",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			svc := sshcfg.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.EnablePasswordAuth(cmd.Context(), globals.Flags.DryRun); err != nil {
				return err
			}
			return printSimpleResult(actions.SSHConfigEnablePasswdAuth, "SSH password authentication enabled.", nil)
		},
	}
}

func newSSHReloadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reload",
		Short: "Reload the SSH server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			svc := sshcfg.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.Reload(cmd.Context(), sshcfg.ReloadOptions{DryRun: globals.Flags.DryRun}); err != nil {
				return err
			}
			return printSimpleResult(actions.SSHReload, "SSH server reloaded.", nil)
		},
	}
}

func newSSHRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "Restart the SSH server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			printer().Warn("Restarting SSH will briefly disconnect active sessions.")
			svc := sshcfg.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.Restart(cmd.Context(), sshcfg.ReloadOptions{DryRun: globals.Flags.DryRun}); err != nil {
				return err
			}
			return printSimpleResult(actions.SSHRestart, "SSH server restarted.", nil)
		},
	}
}
