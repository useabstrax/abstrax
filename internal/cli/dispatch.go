package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"abstrax/internal/globals"
	"abstrax/internal/output"
	"abstrax/internal/services/plugin"
)

func isUnknownCommand(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "unknown command")
}

func tryPluginDispatch(ctx context.Context, args []string) (int, bool, error) {
	if len(args) == 0 {
		return 0, false, nil
	}

	command := args[0]
	if isBuiltinCommand(command) {
		return 0, false, nil
	}

	remaining := args[1:]
	svc, err := plugin.New()
	if err != nil {
		return 0, false, err
	}
	dispatcher, err := svc.NewDispatcher()
	if err != nil {
		return 0, false, err
	}

	exitCode, err := dispatcher.Dispatch(ctx, command, remaining, plugin.DispatchOptions{
		AllowBlocked: effectiveAllowBlocked(),
	})
	return exitCode, true, err
}

func isBuiltinCommand(name string) bool {
	root := NewRootCmd()
	for _, cmd := range root.Commands() {
		if cmd.Name() == name || cmd.HasAlias(name) {
			return true
		}
	}
	return false
}

func printCommandError(err error) {
	p := output.NewPrinter(globals.Flags.JSON, globals.Flags.Quiet, globals.Flags.Verbose, globals.Flags.NoColor)
	if globals.Flags.JSON {
		output.PrintJSON(output.Failure("", errorCode(err), err.Error()))
	} else {
		p.Error("%v", err)
	}
	fmt.Fprintln(os.Stderr)
}

func errorCode(err error) string {
	switch {
	case err == nil:
		return ""
	case isError(err, plugin.ErrNotFound):
		return "plugin_not_installed"
	case isError(err, plugin.ErrRegistryUnavailable):
		return "registry_unavailable"
	case isError(err, plugin.ErrIncompatibleAbstrax):
		return "incompatible_abstrax_version"
	case isError(err, plugin.ErrUnsupportedPlatform):
		return "unsupported_platform"
	case isError(err, plugin.ErrChecksumMismatch):
		return "checksum_mismatch"
	case isError(err, plugin.ErrMalformedMetadata):
		return "malformed_plugin_metadata"
	case isError(err, plugin.ErrUnsupportedProtocol):
		return "unsupported_plugin_protocol"
	case isError(err, plugin.ErrBlockedPlugin):
		return "blocked_plugin"
	case isError(err, plugin.ErrProcessFailure):
		return "plugin_process_failure"
	default:
		return "command_error"
	}
}

func isError(err, target error) bool {
	return err != nil && (err == target || strings.Contains(err.Error(), target.Error()))
}
