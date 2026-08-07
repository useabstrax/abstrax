package cli

import (
	"github.com/spf13/cobra"

	"abstrax/internal/actions"
	"abstrax/internal/globals"
	"abstrax/internal/output"
	"abstrax/internal/version"
)

// NewVersionCmd returns the version command.
func NewVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := printer()

			type versionData struct {
				Version   string `json:"version"`
				Commit    string `json:"commit"`
				BuildDate string `json:"build_date"`
			}

			data := versionData{
				Version:   version.Version,
				Commit:    version.Commit,
				BuildDate: version.BuildDate,
			}

			r := output.Success(actions.VersionShow,
				"abstrax "+version.String(), data)

			if globals.Flags.JSON {
				output.PrintJSON(r)
			} else {
				p.Line("abstrax %s", version.String())
			}
			return nil
		},
	}
}
