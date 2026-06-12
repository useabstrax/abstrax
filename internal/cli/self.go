package cli

import (
	"github.com/spf13/cobra"

	"abstrax/internal/actions"
	"abstrax/internal/globals"
	"abstrax/internal/output"
	"abstrax/internal/platform"
	"abstrax/internal/services/selfupdate"
)

// NewSelfCmd returns the self command group.
func NewSelfCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "self",
		Short: "Manage the Abstrax CLI itself",
	}

	cmd.AddCommand(newSelfUpdateCmd())
	return cmd
}

func newSelfUpdateCmd() *cobra.Command {
	var allowBreaking bool

	cmd := &cobra.Command{
		Use:   "update [version]",
		Short: "Update the Abstrax CLI to a newer release",
		Long: `Update the Abstrax CLI from GitHub releases.

When no version is given, Abstrax updates to the newest release within the
current major version (non-breaking updates only). Use --allow-breaking to
upgrade across major versions.

Examples:
  abstrax self update
  abstrax self update 1.2.0
  abstrax self update --allow-breaking`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := printer()

			requested := ""
			if len(args) == 1 {
				requested = args[0]
			}

			if !globals.Flags.DryRun {
				if err := platform.RequireRoot(); err != nil {
					return err
				}
			}

			svc := selfupdate.New()
			result, err := svc.Update(cmd.Context(), selfupdate.Options{
				RequestedVersion: requested,
				AllowBreaking:    allowBreaking,
				DryRun:           globals.Flags.DryRun,
				Verbose:          globals.Flags.Verbose,
			})
			if err != nil {
				return err
			}

			if globals.Flags.DryRun && !globals.Flags.JSON {
				p.DryRun("%s", result.Message)
				if result.Notice != "" {
					p.Warn(result.Notice)
				}
				return nil
			}

			r := output.Success(actions.SelfUpdate, result.Message, result)
			if globals.Flags.JSON {
				output.PrintJSON(r)
				return nil
			}

			if result.Updated {
				p.Success(result.Message)
			} else {
				p.Info(result.Message)
			}
			if result.Notice != "" {
				p.Warn(result.Notice)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&allowBreaking, "allow-breaking", false,
		"Upgrade to the latest release even when it includes breaking (major version) changes")

	return cmd
}
