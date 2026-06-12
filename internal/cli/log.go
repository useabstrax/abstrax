package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"abstrax/internal/platform/debian"
)

// NewLogCmd returns the log command.
func NewLogCmd() *cobra.Command {
	var follow bool
	var lines int

	cmd := &cobra.Command{
		Use:   "log [path]",
		Short: "View log output",
		Long:  "Tail a log file. Defaults to the Abstrax log at /var/log/abstrax/abstrax.log.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			logFile := debian.AbstraxLogDir + "/abstrax.log"
			if len(args) == 1 {
				logFile = args[0]
			}

			if _, err := os.Stat(logFile); os.IsNotExist(err) {
				printer().Line("No log file found at %s.", logFile)
				return nil
			}

			tailArgs := []string{"-n", fmt.Sprintf("%d", lines)}
			if follow {
				tailArgs = append([]string{"-f"}, tailArgs...)
			}
			tailArgs = append(tailArgs, logFile)

			c := exec.CommandContext(cmd.Context(), "tail", tailArgs...)
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", true, "Follow log output")
	cmd.Flags().IntVar(&lines, "lines", 50, "Number of lines to show")

	return cmd
}
