package cli

import (
	"fmt"

	"abstrax/internal/globals"
	"abstrax/internal/output"
	"abstrax/internal/platform"
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

// requireRootAndSupported ensures mutating commands run as root on a supported platform.
func requireRootAndSupported() error {
	if err := platform.RequireRoot(); err != nil {
		return err
	}
	info, _, err := platform.Detect()
	if err != nil {
		return fmt.Errorf("platform detection failed: %w", err)
	}
	return platform.RequireSupported(info)
}
