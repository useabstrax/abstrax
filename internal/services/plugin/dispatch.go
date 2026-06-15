package plugin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
)

// DispatchOptions configures plugin execution.
type DispatchOptions struct {
	AllowBlocked []string
}

// Dispatcher executes plugin binaries.
type Dispatcher struct {
	discoverer   *Discoverer
	store        *Store
	abstraxBinary string
}

// NewDispatcher creates a Dispatcher.
func NewDispatcher(paths *Paths, store *Store) (*Dispatcher, error) {
	binary, err := currentAbstraxBinary()
	if err != nil {
		return nil, err
	}
	return &Dispatcher{
		discoverer:    NewDiscoverer(paths),
		store:         store,
		abstraxBinary: binary,
	}, nil
}

// Dispatch runs a plugin command with the given arguments.
func (d *Dispatcher) Dispatch(ctx context.Context, command string, args []string, opts DispatchOptions) (int, error) {
	binaryPath, err := d.discoverer.FindBinary(command)
	if err != nil {
		return 1, err
	}

	if err := d.checkBlocked(command, opts.AllowBlocked); err != nil {
		return 1, err
	}

	meta, err := FetchMetadata(ctx, binaryPath)
	if err != nil {
		return 1, err
	}
	if err := ValidateMetadata(meta, command); err != nil {
		return 1, err
	}

	status := d.registryStatus(command)
	if status == StatusDeprecated {
		fmt.Fprintf(os.Stderr, "WARNING: plugin %q is deprecated\n", command)
	}
	if status == StatusBlocked && !slices.Contains(opts.AllowBlocked, command) {
		fmt.Fprintf(os.Stderr, "WARNING: plugin %q is marked as blocked by the registry\n", command)
		return 1, fmt.Errorf("%w: %s (use --allow-blocked-plugin to override)", ErrBlockedPlugin, command)
	}

	cmd := exec.CommandContext(ctx, binaryPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"ABSTRAX_PLUGIN=1",
		fmt.Sprintf("ABSTRAX_PLUGIN_PROTOCOL=%d", ProtocolVersion),
		"ABSTRAX_BINARY="+d.abstraxBinary,
		"ABSTRAX_VERSION="+AbstraxVersionString(),
	)

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code := exitErr.ExitCode()
			return code, &ExitError{Code: code, Err: err}
		}
		return 1, fmt.Errorf("%w: %v", ErrProcessFailure, err)
	}
	return 0, nil
}

func (d *Dispatcher) checkBlocked(command string, allowBlocked []string) error {
	if slices.Contains(allowBlocked, command) {
		return nil
	}
	status := d.registryStatus(command)
	if status == StatusBlocked {
		return fmt.Errorf("%w: %s (use --allow-blocked-plugin to override)", ErrBlockedPlugin, command)
	}
	return nil
}

func (d *Dispatcher) registryStatus(command string) string {
	rec, err := d.store.Load(command)
	if err != nil {
		return ""
	}
	return rec.RegistryStatus
}

func currentAbstraxBinary() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolving abstrax binary: %w", err)
	}
	return path, nil
}
