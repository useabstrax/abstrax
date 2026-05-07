package cli

import (
	"abstrax/internal/globals"
	"abstrax/internal/output"
)

// printer returns an output.Printer configured from the current global flags.
func printer() *output.Printer {
	return output.NewPrinter(
		globals.Flags.JSON,
		globals.Flags.Quiet,
		globals.Flags.Verbose,
		globals.Flags.NoColor,
	)
}
