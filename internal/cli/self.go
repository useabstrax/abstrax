package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"abstrax/internal/actions"
	"abstrax/internal/globals"
	"abstrax/internal/output"
	"abstrax/internal/services/plugin"
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
	var updatePlugins bool

	cmd := &cobra.Command{
		Use:   "update [version]",
		Short: "Update the Abstrax CLI to a newer release",
		Long: `Update the Abstrax CLI from GitHub releases.

When no version is given, Abstrax updates to the newest release within the
current major version (non-breaking updates only). Use --allow-breaking to
upgrade across major versions.

Use --plugins to also update every installed plugin after the CLI update.
Local plugins are skipped. Plugins that are already current are left in place.

Examples:
  abstrax self update
  abstrax self update 1.2.0
  abstrax self update --allow-breaking
  abstrax self update --plugins`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			requested := ""
			if len(args) == 1 {
				requested = args[0]
			}

			if !globals.Flags.DryRun {
				if err := requireRootAndSupported(); err != nil {
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

			var plugins *plugin.UpdateAllResult
			var pluginErr error
			if updatePlugins {
				psvc, err := pluginService()
				if err != nil {
					pluginErr = err
				} else {
					plugins, pluginErr = psvc.UpdateAll(cmd.Context(), globals.Flags.DryRun)
				}
			}

			return finishSelfUpdate(result, plugins, pluginErr)
		},
	}

	cmd.Flags().BoolVar(&allowBreaking, "allow-breaking", false,
		"Upgrade to the latest release even when it includes breaking (major version) changes")
	cmd.Flags().BoolVar(&updatePlugins, "plugins", false,
		"Also update every installed plugin from the registry")

	return cmd
}

// selfUpdateData is the JSON data object for self update.
// Plugin results are included only when --plugins was passed.
type selfUpdateData struct {
	selfupdate.Result
	Plugins *plugin.UpdateAllResult `json:"plugins,omitempty"`
}

func finishSelfUpdate(result selfupdate.Result, plugins *plugin.UpdateAllResult, pluginErr error) error {
	p := printer()
	failure := pluginUpdateFailure(plugins, pluginErr)

	if machineOutput() {
		data := selfUpdateData{Result: result, Plugins: plugins}
		if failure != nil {
			r := output.Failure(actions.SelfUpdate, "plugin_update_failed", failure.Error())
			r.Summary = result.Message
			r.Data = data
			emitResult(r)
			return &emittedResultError{err: failure}
		}
		emitResult(output.Success(actions.SelfUpdate, result.Message, data))
		return nil
	}

	if globals.Flags.DryRun {
		p.DryRun("%s", result.Message)
	} else if result.Updated {
		p.Success(result.Message)
	} else {
		p.Info(result.Message)
	}
	if result.Notice != "" {
		p.Warn(result.Notice)
	}
	printPluginUpdateLines(p, plugins)
	return failure
}

func pluginUpdateFailure(plugins *plugin.UpdateAllResult, pluginErr error) error {
	if pluginErr != nil {
		return fmt.Errorf("plugin updates failed: %w", pluginErr)
	}
	if plugins == nil || len(plugins.Failed) == 0 {
		return nil
	}
	parts := make([]string, len(plugins.Failed))
	for i, item := range plugins.Failed {
		parts[i] = fmt.Sprintf("%s: %s", item.Name, item.Error)
	}
	if len(parts) == 1 {
		return fmt.Errorf("failed to update plugin %s", parts[0])
	}
	return fmt.Errorf("failed to update %d plugins: %s", len(parts), strings.Join(parts, "; "))
}

func printPluginUpdateLines(p *output.Printer, plugins *plugin.UpdateAllResult) {
	if plugins == nil {
		return
	}
	if len(plugins.Updated) == 0 && len(plugins.Skipped) == 0 && len(plugins.Failed) == 0 {
		p.Info("No plugins installed.")
		return
	}
	for _, item := range plugins.Updated {
		if plugins.DryRun {
			p.DryRun("Would update plugin %s from %s to %s.", item.Name, item.Version, item.TargetVersion)
			continue
		}
		p.Success("Updated plugin %s from %s to %s.", item.Name, item.PreviousVersion, item.Version)
	}
	for _, item := range plugins.Skipped {
		if item.Reason == plugin.SkipReasonLocal {
			p.Warn("Skipped local plugin %s. Reinstall it with --path to change it.", item.Name)
			continue
		}
		p.Info("Plugin %s is already up to date (%s).", item.Name, item.Version)
	}
}
