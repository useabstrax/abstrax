package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"abstrax/internal/actions"
	"abstrax/internal/confirm"
	"abstrax/internal/globals"
	"abstrax/internal/output"
	"abstrax/internal/platform"
	"abstrax/internal/services/config"
)

// NewConfigCmd returns the config command.
func NewConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage Abstrax configuration",
	}

	cmd.AddCommand(newConfigShowCmd())
	cmd.AddCommand(newConfigGetCmd())
	cmd.AddCommand(newConfigSetCmd())
	cmd.AddCommand(newConfigAddCmd())
	cmd.AddCommand(newConfigRemoveCmd())
	cmd.AddCommand(newConfigResetCmd())

	return cmd
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "show",
		Aliases: []string{"list"},
		Short:   "Show effective Abstrax configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := config.New()
			settings, err := svc.Effective()
			if err != nil {
				return err
			}

			if globals.Flags.JSON {
				output.PrintJSON(output.Success(actions.ConfigShow, "", settings))
				return nil
			}

			p := printer()
			p.Line("")
			p.Line("  php.extensions:")
			for _, ext := range settings.PHP.Extensions {
				p.Line("    - %s", ext)
			}
			p.Line("")
			return nil
		},
	}
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Get a configuration value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := config.New()
			value, err := svc.Get(args[0])
			if err != nil {
				return err
			}

			if globals.Flags.JSON {
				output.PrintJSON(output.Success(actions.ConfigGet, "", map[string]any{
					"key":   args[0],
					"value": value,
				}))
				return nil
			}

			p := printer()
			switch v := value.(type) {
			case []string:
				p.Line(strings.Join(v, " "))
			default:
				p.Line("%v", value)
			}
			return nil
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <values...>",
		Short: "Replace a list configuration value",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := platform.RequireRoot(); err != nil {
				return err
			}

			svc := config.New()
			if err := svc.Set(args[0], args[1:]); err != nil {
				return err
			}

			return printSimpleResult(actions.ConfigSet,
				fmt.Sprintf("Config %s updated.", args[0]), nil)
		},
	}
}

func newConfigAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <key> <value>",
		Short: "Add a value to a list configuration key",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := platform.RequireRoot(); err != nil {
				return err
			}

			svc := config.New()
			if err := svc.Add(args[0], args[1]); err != nil {
				return err
			}

			return printSimpleResult(actions.ConfigAdd,
				fmt.Sprintf("Added %q to %s.", args[1], args[0]), nil)
		},
	}
}

func newConfigRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <key> <value>",
		Short: "Remove a value from a list configuration key",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := platform.RequireRoot(); err != nil {
				return err
			}

			svc := config.New()
			if err := svc.Remove(args[0], args[1]); err != nil {
				return err
			}

			return printSimpleResult(actions.ConfigRemove,
				fmt.Sprintf("Removed %q from %s.", args[1], args[0]), nil)
		},
	}
}

func newConfigResetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset [key]",
		Short: "Reset configuration to defaults",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := platform.RequireRoot(); err != nil {
				return err
			}

			key := ""
			if len(args) > 0 {
				key = args[0]
			}

			prompt := "Reset all Abstrax configuration to defaults?"
			if key != "" {
				prompt = fmt.Sprintf("Reset %s to defaults?", key)
			}

			ok, err := confirm.Ask(prompt, globals.Flags.Yes)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}

			svc := config.New()
			if err := svc.Reset(key); err != nil {
				return err
			}

			msg := "Configuration reset to defaults."
			if key != "" {
				msg = fmt.Sprintf("Config %s reset to defaults.", key)
			}
			return printSimpleResult(actions.ConfigReset, msg, nil)
		},
	}
}
