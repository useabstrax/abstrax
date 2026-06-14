// Package session provides helpers for detecting properties of the current
// login session, such as the remote client IP address.
package session

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	executil "abstrax/internal/exec"
)

// ClientIP returns the IP address of the client for the current session.
// It tries SSH environment variables first, then falls back to parsing who(1)
// output which works when sudo strips SSH env vars.
func ClientIP(ctx context.Context, runner *executil.Runner) (string, error) {
	if ip := ipFromSSHEnv(os.Getenv("SSH_CONNECTION"), os.Getenv("SSH_CLIENT")); ip != "" {
		return ip, nil
	}

	if ip, err := ipFromWhoAmI(ctx, runner); err == nil && ip != "" {
		return ip, nil
	}

	if ip, err := ipFromSudoUserWho(ctx, runner); err == nil && ip != "" {
		return ip, nil
	}

	return "", fmt.Errorf("could not determine client IP address")
}

func ipFromSSHEnv(connection, client string) string {
	if connection != "" {
		parts := strings.Fields(connection)
		if len(parts) >= 1 {
			if ip := normalizeIP(parts[0]); ip != "" {
				return ip
			}
		}
	}

	if client != "" {
		parts := strings.Fields(client)
		if len(parts) >= 1 {
			if ip := normalizeIP(parts[0]); ip != "" {
				return ip
			}
		}
	}

	return ""
}

func ipFromWhoLine(line string) string {
	line = strings.TrimSpace(line)
	start := strings.LastIndex(line, "(")
	end := strings.LastIndex(line, ")")
	if start < 0 || end <= start {
		return ""
	}
	return normalizeIP(line[start+1 : end])
}

func ipFromWhoAmI(ctx context.Context, runner *executil.Runner) (string, error) {
	res, err := runner.RunSilent(ctx, "who", "am", "i")
	if err != nil {
		return "", err
	}
	if ip := ipFromWhoLine(res.Stdout); ip != "" {
		return ip, nil
	}
	return "", fmt.Errorf("no IP in who am i output")
}

func ipFromSudoUserWho(ctx context.Context, runner *executil.Runner) (string, error) {
	sudoUser := os.Getenv("SUDO_USER")
	if sudoUser == "" {
		return "", fmt.Errorf("SUDO_USER not set")
	}

	res, err := runner.RunSilent(ctx, "who")
	if err != nil {
		return "", err
	}

	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 1 || parts[0] != sudoUser {
			continue
		}
		if ip := ipFromWhoLine(line); ip != "" {
			return ip, nil
		}
	}

	return "", fmt.Errorf("no IP found for SUDO_USER %q", sudoUser)
}

func normalizeIP(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	// Strip IPv4-mapped IPv6 prefix if present.
	if strings.HasPrefix(raw, "::ffff:") {
		raw = strings.TrimPrefix(raw, "::ffff:")
	}

	if net.ParseIP(raw) == nil {
		return ""
	}
	return raw
}
