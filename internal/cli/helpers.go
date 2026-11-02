package cli

import (
	"fmt"

	"abstrax/internal/globals"
	"abstrax/internal/output"
	"abstrax/internal/platform"
)

// emittedResultError means the command already wrote its JSON result.
// printCommandError skips it so the result is not written twice.
type emittedResultError struct {
	err error
}

func (e *emittedResultError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *emittedResultError) Unwrap() error { return e.err }

// skipConfirm returns true when a destructive command should skip its prompt.
func skipConfirm(force bool) bool {
	return force || globals.Flags.Yes
}

// printer returns an output.Printer configured from the current global flags.
func printer() *output.Printer {
	return output.NewPrinter(
		globals.Flags.JSON,
		globals.Flags.JSONStream,
		globals.Flags.Quiet,
		globals.Flags.Verbose,
		globals.Flags.NoColor,
	)
}

// machineOutput reports whether --json or --json-stream is set.
func machineOutput() bool {
	return globals.MachineOutput()
}

// emitResult writes a final Result for --json or --json-stream.
func emitResult(r output.Result) {
	output.WriteResult(r, globals.Flags.JSONStream)
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
