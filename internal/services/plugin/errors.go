package plugin

import (
	"errors"
	"fmt"
)

var (
	// ErrNotFound indicates no plugin binary or record was found.
	ErrNotFound = errors.New("plugin not installed")

	// ErrUnsupportedProtocol indicates an unsupported metadata protocol version.
	ErrUnsupportedProtocol = errors.New("unsupported plugin protocol")

	// ErrMalformedMetadata indicates invalid plugin metadata.
	ErrMalformedMetadata = errors.New("malformed plugin metadata")

	// ErrIncompatibleAbstrax indicates the plugin requires a different Abstrax version.
	ErrIncompatibleAbstrax = errors.New("incompatible Abstrax version")

	// ErrUnsupportedPlatform indicates no binary for the current platform.
	ErrUnsupportedPlatform = errors.New("unsupported platform")

	// ErrChecksumMismatch indicates a downloaded binary failed checksum verification.
	ErrChecksumMismatch = errors.New("checksum mismatch")

	// ErrRegistryUnavailable indicates the plugin registry could not be reached.
	ErrRegistryUnavailable = errors.New("registry unavailable")

	// ErrBlockedPlugin indicates the plugin is blocked by registry policy.
	ErrBlockedPlugin = errors.New("plugin is blocked")

	// ErrProcessFailure indicates the plugin process failed to start.
	ErrProcessFailure = errors.New("plugin process failure")

	// ErrRegistryPluginNotFound indicates the registry has no matching plugin.
	ErrRegistryPluginNotFound = errors.New("registry plugin not found")

	// ErrRegistryVersionNotFound indicates the registry has no matching version.
	ErrRegistryVersionNotFound = errors.New("registry version not found")

	// ErrNoCompatibleVersion indicates no version matches the requested constraints.
	ErrNoCompatibleVersion = errors.New("no compatible plugin version")
)

// ExitError wraps a plugin process exit with its exit code.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("plugin exited with code %d: %v", e.Code, e.Err)
	}
	return fmt.Sprintf("plugin exited with code %d", e.Code)
}

func (e *ExitError) Unwrap() error {
	return e.Err
}

// ExitCode extracts an exit code from an error if present.
func ExitCode(err error) (int, bool) {
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code, true
	}
	return 0, false
}
