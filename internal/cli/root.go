// Package cli defines the Cobra command tree for the Abstrax CLI.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"abstrax/internal/globals"
	"abstrax/internal/output"
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
			// Nothing to do here – flags are read directly from globals.Flags.
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

	// Subcommands.
	root.AddCommand(NewVersionCmd())
	root.AddCommand(NewDoctorCmd())
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

	return root
}

// Execute runs the root command.
func Execute() {
	root := NewRootCmd()
	if err := root.Execute(); err != nil {
		p := output.NewPrinter(globals.Flags.JSON, globals.Flags.Quiet, globals.Flags.Verbose, globals.Flags.NoColor)
		if globals.Flags.JSON {
			output.PrintJSON(output.Failure("", "command_error", err.Error()))
		} else {
			p.Error("%v", err)
		}
		fmt.Fprintln(os.Stderr)
		os.Exit(1)
	}
}
