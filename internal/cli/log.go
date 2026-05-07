package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"abstrax/internal/globals"
	"abstrax/internal/platform/debian"
)

// NewLogCmd returns the log command.
func NewLogCmd() *cobra.Command {
	var follow bool
	var lines int

	cmd := &cobra.Command{
		Use:   "log",
		Short: "View Abstrax log output",
		RunE: func(cmd *cobra.Command, args []string) error {
			logFile := debian.AbstraxLogDir + "/abstrax.log"

			if _, err := os.Stat(logFile); os.IsNotExist(err) {
				printer().Line("No log file found at %s.", logFile)
				return nil
			}

			if follow {
				c := exec.CommandContext(cmd.Context(), "tail", "-f", "-n",
					fmt.Sprintf("%d", lines), logFile)
				c.Stdout = os.Stdout
				c.Stderr = os.Stderr
				return c.Run()
			}

			c := exec.CommandContext(cmd.Context(), "tail", "-n",
				fmt.Sprintf("%d", lines), logFile)
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			_ = globals.Flags.Verbose // suppress unused warning
			return c.Run()
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	cmd.Flags().IntVar(&lines, "lines", 50, "Number of lines to show")

	return cmd
}
