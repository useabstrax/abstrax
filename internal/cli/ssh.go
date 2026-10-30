package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"abstrax/internal/actions"
	"abstrax/internal/globals"
	"abstrax/internal/output"
	"abstrax/internal/services/firewall"
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
		Short: "Show the SSH settings sshd is using",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := sshcfg.New(false, globals.Flags.Verbose)
			cfg, err := svc.Show(cmd.Context())
			if err != nil {
				return err
			}

			p := printer()
			r := output.Success(actions.SSHConfigShow, "", cfg)

			if machineOutput() {
				emitResult(r)
				return nil
			}

			p.Line("")
			if cfg.Note != "" {
				p.Line("  %s", cfg.Note)
			}
			p.Line("  %-28s %s", "Port:", formatSSHSetting(cfg.Port, cfg.PortSource))
			p.Line("  %-28s %s", "PermitRootLogin:", formatSSHSetting(cfg.PermitRootLogin, cfg.PermitRootLoginSource))
			p.Line("  %-28s %s", "PasswordAuthentication:", formatSSHSetting(cfg.PasswordAuth, cfg.PasswordAuthSource))
			p.Line("  %-28s %s", "ClientAliveInterval:", formatSSHSetting(cfg.ClientAliveInterval, cfg.ClientAliveIntervalSource))
			p.Line("")
			return nil
		},
	}
}

func formatSSHSetting(value, source string) string {
	if source == "" {
		return value
	}
	return value + " (" + source + ")"
}

func newSSHConfigSetPortCmd() *cobra.Command {
	var allowFirewall bool
	var closeOldPort bool

	cmd := &cobra.Command{
		Use:   "set-port <port>",
		Short: "Change the SSH listening port",
		Long: `Change the SSH listening port.

With --allow-firewall the new port is opened before sshd is reloaded. The previous port stays open until --close-old-port, so you can test a new connection first. Without --allow-firewall the firewall is left unchanged.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validate.PortString(args[0]); err != nil {
				return err
			}
			if err := requireRootAndSupported(); err != nil {
				return err
			}

			var port int
			fmt.Sscanf(args[0], "%d", &port)

			p := printer()
			if !allowFirewall {
				p.Warn("The firewall was not changed. Open the new port before you disconnect.")
			}

			svc := sshcfg.New(globals.Flags.DryRun, globals.Flags.Verbose)
			fw := firewall.New(globals.Flags.DryRun, globals.Flags.Verbose)
			svc.UseFirewall(
				func(ctx context.Context, port int) error {
					_, err := fw.Allow(ctx, firewall.AllowOptions{
						Port:     fmt.Sprintf("%d", port),
						Protocol: "tcp",
						Comment:  "abstrax: ssh",
						DryRun:   globals.Flags.DryRun,
					})
					return err
				},
				func(ctx context.Context, port int) error {
					return fw.RemovePort(ctx, fmt.Sprintf("%d", port), "tcp")
				},
			)
			result, err := svc.SetPort(cmd.Context(), sshcfg.SetPortOptions{
				Port:          port,
				AllowFirewall: allowFirewall,
				CloseOldPort:  closeOldPort,
				DryRun:        globals.Flags.DryRun,
			})
			if err != nil {
				return err
			}

			return printSimpleResult(actions.SSHConfigSetPort, result.Message, result)
		},
	}

	cmd.Flags().BoolVar(&allowFirewall, "allow-firewall", false, "Open the new port in the firewall before reloading sshd")
	cmd.Flags().BoolVar(&closeOldPort, "close-old-port", false, "Close the previous SSH port after the new one is accepting connections")
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
			if err := requireRootAndSupported(); err != nil {
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
			if err := requireRootAndSupported(); err != nil {
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
			if err := requireRootAndSupported(); err != nil {
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
			if err := requireRootAndSupported(); err != nil {
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
			if err := requireRootAndSupported(); err != nil {
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
			if err := requireRootAndSupported(); err != nil {
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
			if err := requireRootAndSupported(); err != nil {
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
