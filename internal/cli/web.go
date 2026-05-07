package cli

import (
	"github.com/spf13/cobra"

	"abstrax/internal/actions"
	"abstrax/internal/globals"
	"abstrax/internal/output"
	"abstrax/internal/platform"
	"abstrax/internal/services/web"
)

// NewWebCmd returns the web command.
func NewWebCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "web",
		Short: "Manage web servers",
	}

	var backend string

	cmd.PersistentFlags().StringVar(&backend, "nginx", "", "Use nginx backend")

	testCmd := &cobra.Command{
		Use:   "test",
		Short: "Test web server configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := web.New(globals.Flags.DryRun, globals.Flags.Verbose)
			result, err := svc.Test(cmd.Context(), resolveBackend(backend))
			if err != nil {
				return err
			}

			p := printer()
			r := output.Success(actions.WebTest, "Web server config test passed.", result)
			if !result.OK {
				r = output.Failure(actions.WebTest, "config_invalid", result.Output)
			}

			if globals.Flags.JSON {
				output.PrintJSON(r)
				return nil
			}

			if result.OK {
				p.Success("nginx configuration test passed.")
			} else {
				p.Error("nginx configuration test failed:\n%s", result.Output)
			}
			return nil
		},
	}

	reloadCmd := &cobra.Command{
		Use:   "reload",
		Short: "Reload web server gracefully",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			svc := web.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.Reload(cmd.Context(), resolveBackend(backend)); err != nil {
				return err
			}
			return printSimpleResult(actions.WebReload, "Web server reloaded.", nil)
		},
	}

	restartCmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart web server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			svc := web.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.Restart(cmd.Context(), resolveBackend(backend)); err != nil {
				return err
			}
			return printSimpleResult(actions.WebRestart, "Web server restarted.", nil)
		},
	}

	testCmd.Flags().StringVar(&backend, "nginx", "nginx", "Use nginx")
	reloadCmd.Flags().StringVar(&backend, "nginx", "nginx", "Use nginx")
	restartCmd.Flags().StringVar(&backend, "nginx", "nginx", "Use nginx")

	cmd.AddCommand(testCmd, reloadCmd, restartCmd)
	return cmd
}

func resolveBackend(flag string) string {
	if flag == "" {
		return "nginx"
	}
	return flag
}
