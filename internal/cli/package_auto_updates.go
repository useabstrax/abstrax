package cli

import (
	"github.com/spf13/cobra"

	"abstrax/internal/actions"
	"abstrax/internal/globals"
	"abstrax/internal/output"
	"abstrax/internal/services/pkgmanager"
)

func newPkgAutoUpdatesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auto-updates",
		Short: "Manage automatic security updates",
		Long: `Manage automatic security updates.

Debian and Ubuntu use unattended-upgrades. RHEL-family systems use
dnf-automatic. Both apply security updates only and never reboot the server.
Use "abstrax server reboot-required" to see when a reboot is needed.`,
	}

	cmd.AddCommand(newPkgAutoUpdatesStatusCmd())
	cmd.AddCommand(newPkgAutoUpdatesEnableCmd())
	cmd.AddCommand(newPkgAutoUpdatesDisableCmd())
	return cmd
}

func newPkgAutoUpdatesStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether automatic security updates are enabled",
		Long: `Show whether automatic security updates are enabled, when they last ran, and
how the last run ended. This command changes nothing and does not require root.

last_result is success, failed, unknown (a run was seen but its outcome was
not), or never_run. last_run is null when no run time can be found.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := pkgmanager.NewAutoUpdatesFromPlatform(false, globals.Flags.Verbose)
			if err != nil {
				return err
			}
			st, err := svc.Status(cmd.Context())
			if err != nil {
				return err
			}

			summary := "Automatic security updates are disabled."
			if st.Enabled {
				summary = "Automatic security updates are enabled."
			}
			if machineOutput() {
				emitResult(output.Success(actions.PackageAutoUpdatesStatus, summary, st))
				return nil
			}

			p := printer()
			p.Line("")
			p.Line("  %-14s %s", "Backend:", st.Backend)
			p.Line("  %-14s %s", "Installed:", yesOrNo(st.Installed))
			p.Line("  %-14s %s", "Enabled:", yesOrNo(st.Enabled))
			p.Line("  %-14s %s", "Managed:", yesOrNo(st.Managed))
			lastRun := "never"
			if st.LastRun != nil {
				lastRun = *st.LastRun
			}
			p.Line("  %-14s %s", "Last run:", lastRun)
			p.Line("  %-14s %s", "Last result:", st.LastResult)
			p.Line("")
			return nil
		},
	}
}

func newPkgAutoUpdatesEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable",
		Short: "Enable automatic security updates",
		Long: `Enable automatic security updates. This requires root.

The backend (unattended-upgrades or dnf-automatic) is installed when missing.
Only security updates are applied, and the server is never rebooted
automatically. Abstrax writes its own configuration and leaves distro files
alone.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireRootAndSupported(); err != nil {
				return err
			}
			svc, err := pkgmanager.NewAutoUpdatesFromPlatform(globals.Flags.DryRun, globals.Flags.Verbose)
			if err != nil {
				return err
			}
			change, err := svc.Enable(cmd.Context())
			if err != nil {
				return err
			}
			summary := "Automatic security updates enabled. The server will not reboot automatically."
			if !change.Changed {
				summary = "Automatic security updates were already enabled."
			}
			return printSimpleResult(actions.PackageAutoUpdatesEnable, summary, change)
		},
	}
}

func newPkgAutoUpdatesDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable",
		Short: "Disable automatic security updates",
		Long: `Disable automatic security updates. This requires root.

Abstrax turns off its own configuration. The backend package stays installed.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireRootAndSupported(); err != nil {
				return err
			}
			svc, err := pkgmanager.NewAutoUpdatesFromPlatform(globals.Flags.DryRun, globals.Flags.Verbose)
			if err != nil {
				return err
			}
			change, err := svc.Disable(cmd.Context())
			if err != nil {
				return err
			}
			summary := "Automatic security updates disabled."
			if !change.Changed {
				summary = "Automatic security updates were already disabled."
			}
			return printSimpleResult(actions.PackageAutoUpdatesDisable, summary, change)
		},
	}
}

func yesOrNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
