package cli

import (
	"abstrax/internal/globals"
	"abstrax/internal/output"
)

// skipConfirm returns true when a destructive command should skip its prompt.
func skipConfirm(force bool) bool {
	return force || globals.Flags.Yes
}

// printer returns an output.Printer configured from the current global flags.
func printer() *output.Printer {
	return output.NewPrinter(
		globals.Flags.JSON,
		globals.Flags.Quiet,
		globals.Flags.Verbose,
		globals.Flags.NoColor,
	)
}
