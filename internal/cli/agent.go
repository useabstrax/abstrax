package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

const agentNotImplementedMsg = `Agent mode is not yet implemented.

The Abstrax hosted agent will connect to a hosted API, fetch structured jobs,
execute them locally through the same action layer as the CLI, and report
structured results. The hosted platform will not require inbound SSH access
to the server.

See the README for more information about the future agent architecture.`

// NewAgentCmd returns placeholder agent subcommands.
func NewAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage the Abstrax hosted agent (not yet implemented)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println(agentNotImplementedMsg)
			return nil
		},
	}

	for _, name := range []string{"connect", "status", "run", "update"} {
		n := name
		cmd.AddCommand(&cobra.Command{
			Use:   n,
			Short: fmt.Sprintf("Agent %s (not yet implemented)", n),
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Println(agentNotImplementedMsg)
				return nil
			},
		})
	}

	return cmd
}
