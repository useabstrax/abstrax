package plugin

import (
	"runtime"
	"fmt"
)

// CurrentPlatform returns the registry platform identifier for this host.
func CurrentPlatform() (string, error) {
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("%w: only linux is supported, got %q", ErrUnsupportedPlatform, runtime.GOOS)
	}
	switch runtime.GOARCH {
	case "amd64":
		return "linux-amd64", nil
	case "arm64":
		return "linux-arm64", nil
	default:
		return "", fmt.Errorf("%w: architecture %q is not supported", ErrUnsupportedPlatform, runtime.GOARCH)
	}
}
