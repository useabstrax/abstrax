// Package cli defines the Cobra command tree for the Abstrax CLI.
package cli

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"abstrax/internal/globals"
	"abstrax/internal/services/plugin"
	"abstrax/internal/version"
)

// NewRootCmd creates the root cobra command.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "abstrax",
		Short: "Server management CLI",
		Long: `Abstrax is a server management CLI that abstracts common Linux server
administration tasks behind a consistent, friendly command interface.

Run 'abstrax doctor' to inspect the current system.
Run 'abstrax --help' for a list of commands.`,
		Version: version.String(),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Nothing to do here - flags are read directly from globals.Flags.
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Global flags.
	root.PersistentFlags().BoolVar(&globals.Flags.JSON, "json", false, "Output machine-readable JSON")
	root.PersistentFlags().BoolVar(&globals.Flags.DryRun, "dry-run", false, "Show what would happen without making changes")
	root.PersistentFlags().BoolVar(&globals.Flags.Yes, "yes", false, "Skip confirmation prompts")
	root.PersistentFlags().BoolVar(&globals.Flags.Quiet, "quiet", false, "Reduce output")
	root.PersistentFlags().BoolVar(&globals.Flags.Verbose, "verbose", false, "Increase output verbosity")
	root.PersistentFlags().BoolVar(&globals.Flags.NoColor, "no-color", false, "Disable colour output")
	root.PersistentFlags().StringSliceVar(&globals.Flags.AllowBlockedPlugin, "allow-blocked-plugin", nil, "Allow execution of blocked plugins (repeatable)")

	// Subcommands.
	root.AddCommand(NewVersionCmd())
	root.AddCommand(NewSelfCmd())
	root.AddCommand(NewDoctorCmd())
	root.AddCommand(NewConfigCmd())
	root.AddCommand(NewUserCmd())
	root.AddCommand(NewSSHKeyCmd())
	root.AddCommand(NewSSHCmd())
	root.AddCommand(NewPackageCmd())
	root.AddCommand(NewServiceCmd())
	root.AddCommand(NewCronCmd())
	root.AddCommand(NewDaemonCmd())
	root.AddCommand(NewProjectCmd())
	root.AddCommand(NewWebCmd())
	root.AddCommand(NewSSLCmd())
	root.AddCommand(NewMySQLCmd())
	root.AddCommand(NewCacheCmd())
	root.AddCommand(NewFirewallCmd())
	root.AddCommand(NewServerCmd())
	root.AddCommand(NewLogCmd())
	root.AddCommand(NewAgentCmd())
	root.AddCommand(NewPluginCmd())

	defaultHelp := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		defaultHelp(cmd, args)
		appendPluginHelp()
	})

	return root
}

// Execute runs the root command.
func Execute() {
	root := NewRootCmd()
	_, err := root.ExecuteC()
	if err != nil {
		if isUnknownCommand(err) {
			exitCode, handled, dispatchErr := tryPluginDispatch(context.Background(), os.Args[1:])
			if handled {
				if dispatchErr != nil {
					printCommandError(dispatchErr)
					if code, ok := plugin.ExitCode(dispatchErr); ok {
						os.Exit(code)
					}
					os.Exit(1)
				}
				os.Exit(exitCode)
			}
		}
		printCommandError(err)
		os.Exit(1)
	}
}

func appendPluginHelp() {
	if globals.Flags.Quiet {
		return
	}
	svc, err := plugin.New()
	if err != nil {
		return
	}
	entries, err := svc.MetadataCache().ListEntries()
	if err != nil || len(entries) == 0 {
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	fmt.Println()
	fmt.Println("Plugin commands:")
	for _, e := range entries {
		desc := e.Description
		if desc == "" && len(e.Commands) > 0 {
			desc = e.Commands[0].Description
		}
		if desc == "" {
			desc = e.DisplayName
		}
		fmt.Printf("  %-10s %s\n", e.Name, desc)
	}
}
