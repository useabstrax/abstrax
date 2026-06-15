package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"abstrax/internal/actions"
	"abstrax/internal/confirm"
	"abstrax/internal/globals"
	"abstrax/internal/output"
	"abstrax/internal/platform"
	"abstrax/internal/services/config"
	"abstrax/internal/services/plugin"
	"abstrax/internal/validate"
)

// NewPluginCmd returns the plugin management command.
func NewPluginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage Abstrax CLI plugins",
	}

	cmd.AddCommand(newPluginListCmd())
	cmd.AddCommand(newPluginInfoCmd())
	cmd.AddCommand(newPluginSearchCmd())
	cmd.AddCommand(newPluginInstallCmd())
	cmd.AddCommand(newPluginUpdateCmd())
	cmd.AddCommand(newPluginRemoveCmd())

	return cmd
}

func pluginService() (*plugin.Service, error) {
	cfg, err := config.New().Effective()
	if err != nil {
		return nil, err
	}
	registryURL := config.DefaultPluginRegistryURL
	if cfg.Plugins != nil && cfg.Plugins.RegistryURL != "" {
		registryURL = cfg.Plugins.RegistryURL
	}
	if env := os.Getenv("ABSTRAX_PLUGIN_REGISTRY"); env != "" {
		registryURL = env
	}
	paths, err := plugin.EffectivePaths()
	if err != nil {
		return nil, err
	}
	return plugin.NewWithPaths(paths, registryURL), nil
}

func newPluginListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed plugins",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := pluginService()
			if err != nil {
				return err
			}
			entries, err := svc.ListInstalled(cmd.Context())
			if err != nil {
				return err
			}
			if globals.Flags.JSON {
				output.PrintJSON(output.Success(actions.PluginList, "", entries))
				return nil
			}
			if len(entries) == 0 {
				fmt.Println("No plugins installed.")
				return nil
			}
			table := output.NewTable([]string{"NAME", "VERSION", "PUBLISHER", "TRUST", "STATUS", "UPDATE"})
			for _, e := range entries {
				trust := formatTrustLevel(e.TrustLevel)
				update := e.UpdateAvailable
				if update == "" {
					update = "-"
				}
				table.Append([]string{e.Name, e.Version, e.Publisher, trust, e.Status, update})
			}
			table.Render()
			return nil
		},
	}
}

func newPluginInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <name>",
		Short: "Show detailed plugin information",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validate.PluginName(args[0]); err != nil {
				return err
			}
			svc, err := pluginService()
			if err != nil {
				return err
			}
			info, err := svc.Info(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if globals.Flags.JSON {
				output.PrintJSON(output.Success(actions.PluginInfo, "", info))
				return nil
			}
			p := printer()
			p.Info("Name:         %s", info.Name)
			if info.DisplayName != "" {
				p.Info("Display name: %s", info.DisplayName)
			}
			p.Info("Version:      %s", info.Version)
			if info.Description != "" {
				p.Info("Description:  %s", info.Description)
			}
			p.Info("Publisher:    %s", info.Publisher)
			p.Info("Trust level:  %s", formatTrustLevel(info.TrustLevel))
			p.Info("Source:       %s", info.Source)
			if info.Homepage != "" {
				p.Info("Homepage:     %s", info.Homepage)
			}
			if info.RequiresAbstrax != "" {
				p.Info("Requires:     %s", info.RequiresAbstrax)
			}
			p.Info("Installed at: %s", info.InstalledPath)
			p.Info("Status:       %s", info.RegistryStatus)
			if info.UpdateAvailable != "" {
				p.Info("Update:       %s available", info.UpdateAvailable)
			}
			if info.RegistryStatus == plugin.StatusDeprecated {
				p.Warn("This plugin is deprecated.")
			}
			if info.RegistryStatus == plugin.StatusBlocked {
				p.Warn("This plugin is marked as blocked by the registry.")
			}
			if len(info.Commands) > 0 {
				fmt.Println()
				fmt.Println("Commands:")
				for _, c := range info.Commands {
					fmt.Printf("  %-12s %s\n", c.Name, c.Description)
				}
			}
			return nil
		},
	}
}

func newPluginSearchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search the plugin registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := pluginService()
			if err != nil {
				return err
			}
			results, err := svc.Search(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if globals.Flags.JSON {
				output.PrintJSON(output.Success(actions.PluginSearch, "", results))
				return nil
			}
			if len(results) == 0 {
				fmt.Printf("No plugins found matching %q.\n", args[0])
				return nil
			}
			table := output.NewTable([]string{"NAME", "DESCRIPTION", "PUBLISHER", "TRUST", "VERSION"})
			for _, r := range results {
				table.Append([]string{
					r.Name,
					truncate(r.Description, 50),
					r.Publisher,
					formatTrustLevel(r.TrustLevel),
					r.LatestVersion,
				})
			}
			table.Render()
			return nil
		},
	}
}

func newPluginInstallCmd() *cobra.Command {
	var manifestURL string

	cmd := &cobra.Command{
		Use:   "install <name>",
		Short: "Install a plugin from the registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validate.PluginName(args[0]); err != nil {
				return err
			}
			if manifestURL == "" {
				if err := platform.RequireRoot(); err != nil {
					return err
				}
			}
			if manifestURL != "" {
				fmt.Fprintf(os.Stderr, "WARNING: Installing plugin from a direct manifest URL, not the official Abstrax registry.\n")
				fmt.Fprintf(os.Stderr, "WARNING: Manifest source: %s\n", manifestURL)
				ok, err := confirm.Ask("Continue with manifest installation?", globals.Flags.Yes)
				if err != nil {
					return err
				}
				if !ok {
					return nil
				}
			}

			svc, err := pluginService()
			if err != nil {
				return err
			}
			result, err := svc.Install(cmd.Context(), plugin.InstallOptions{
				Name:        args[0],
				ManifestURL: manifestURL,
			})
			if err != nil {
				return err
			}
			if globals.Flags.JSON {
				output.PrintJSON(output.Success(actions.PluginInstall,
					fmt.Sprintf("Installed plugin %s %s from %s.", result.Name, result.Version, result.Source),
					result))
				return nil
			}
			p := printer()
			p.Success("Installed plugin %s %s (%s, trust: %s) from %s.",
				result.Name, result.Version, result.Publisher, formatTrustLevel(result.TrustLevel), result.Source)
			return nil
		},
	}
	cmd.Flags().StringVar(&manifestURL, "manifest", "", "Install from a direct manifest JSON URL")
	return cmd
}

func newPluginUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update <name>",
		Short: "Update an installed plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validate.PluginName(args[0]); err != nil {
				return err
			}
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			svc, err := pluginService()
			if err != nil {
				return err
			}
			result, err := svc.Update(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			summary := fmt.Sprintf("Updated plugin %s to %s.", result.Name, result.Version)
			return printSimpleResult(actions.PluginUpdate, summary, result)
		},
	}
}

func newPluginRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an installed plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validate.PluginName(args[0]); err != nil {
				return err
			}
			ok, err := confirm.Ask(fmt.Sprintf("Remove plugin %q?", args[0]), globals.Flags.Yes)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			svc, err := pluginService()
			if err != nil {
				return err
			}
			if err := svc.Remove(args[0]); err != nil {
				return err
			}
			return printSimpleResult(actions.PluginRemove, fmt.Sprintf("Removed plugin %s.", args[0]), nil)
		},
	}
}

func formatTrustLevel(level string) string {
	switch level {
	case plugin.TrustOfficial:
		return "official *"
	case plugin.TrustVerified:
		return "verified"
	case plugin.TrustCommunity:
		return "community"
	default:
		return level
	}
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
