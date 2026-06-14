package firewall

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"abstrax/internal/services/sshcfg"
	"abstrax/internal/session"
)

const sshProtectComment = "abstrax: ssh lockout protection"

func (s *Service) ensureClientSSHAllow(ctx context.Context) (SSHProtectResult, error) {
	result := SSHProtectResult{}

	clientIP, err := session.ClientIP(ctx, s.runner)
	if err != nil {
		return result, nil
	}

	sshPort, err := sshcfg.SSHPort()
	if err != nil {
		return result, nil
	}

	result.Applied = true
	result.ClientIP = clientIP
	result.SSHPort = sshPort

	status, err := s.GetStatus(ctx)
	if err != nil {
		return result, fmt.Errorf("checking firewall rules: %w", err)
	}

	if hasSSHAllowRule(status.Rules, clientIP, sshPort) {
		return result, nil
	}

	args := []string{
		"allow", "from", clientIP, "to", "any",
		"port", strconv.Itoa(sshPort), "proto", "tcp",
		"comment", sshProtectComment,
	}
	if _, err := s.runner.Run(ctx, "ufw", args...); err != nil {
		return result, fmt.Errorf("allowing SSH from %s: %w", clientIP, err)
	}

	result.Added = true
	return result, nil
}

func hasSSHAllowRule(rules []Rule, clientIP string, sshPort int) bool {
	portTCP := fmt.Sprintf("%d/tcp", sshPort)
	portStr := strconv.Itoa(sshPort)

	for _, r := range rules {
		if !strings.EqualFold(r.Action, "ALLOW") {
			continue
		}
		if r.From != clientIP {
			continue
		}
		port := strings.TrimSuffix(strings.TrimSuffix(r.Port, "/tcp"), "/udp")
		if r.Port == portTCP || port == portStr {
			return true
		}
	}
	return false
}
