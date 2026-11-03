package firewall

import (
	"context"
	"fmt"
	"strings"
)

// Defaults returns the default incoming and outgoing policy and the ping state.
func (s *Service) Defaults(ctx context.Context) (*Defaults, error) {
	if err := s.requireBackend(); err != nil {
		return nil, err
	}
	if s.backend == "firewalld" {
		return s.firewalldDefaults(ctx)
	}
	return s.ufwDefaults(ctx)
}

// DefaultsInfo returns Defaults wrapped with the backend name.
func (s *Service) DefaultsInfo(ctx context.Context) (*DefaultsResult, error) {
	defaults, err := s.Defaults(ctx)
	if err != nil {
		return nil, err
	}
	return &DefaultsResult{Backend: s.backend, Defaults: defaults}, nil
}

// SetDefaults changes the default incoming (and, on UFW, outgoing) policy.
//
// Switching incoming to deny or reject from a policy that allows traffic is
// refused when it would leave an active firewall with no rule for the SSH port,
// unless opts.Force is set. If the current policy already blocks incoming
// traffic, nothing new is being locked out and the check is skipped.
func (s *Service) SetDefaults(ctx context.Context, opts DefaultsSetOptions) (*DefaultsResult, error) {
	if err := s.requireBackend(); err != nil {
		return nil, err
	}
	incoming, err := normalizePolicy("incoming", opts.Incoming)
	if err != nil {
		return nil, err
	}
	outgoing, err := normalizePolicy("outgoing", opts.Outgoing)
	if err != nil {
		return nil, err
	}
	if incoming == "" && outgoing == "" {
		return nil, fmt.Errorf("specify --incoming deny|reject|allow")
	}
	if outgoing != "" && s.backend == "firewalld" {
		return nil, fmt.Errorf("setting the outgoing default is not supported on firewalld; it does not filter outgoing traffic by default")
	}

	var check *SSHCheck
	if incoming == PolicyDeny || incoming == PolicyReject {
		current, err := s.Defaults(ctx)
		alreadyBlocking := err == nil && (current.Incoming == PolicyDeny || current.Incoming == PolicyReject)
		if !opts.Force && !alreadyBlocking {
			status, err := s.GetStatus(ctx)
			if err != nil {
				return nil, err
			}
			check, err = s.guardSSH(false, status.Active, status.Rules)
			if err != nil {
				return nil, err
			}
		}
	}

	if s.backend == "firewalld" {
		if err := s.firewalldSetIncoming(ctx, incoming); err != nil {
			return nil, err
		}
	} else {
		if incoming != "" {
			if _, err := s.runner.Run(ctx, "ufw", "default", incoming, "incoming"); err != nil {
				return nil, fmt.Errorf("ufw default %s incoming: %w", incoming, err)
			}
		}
		if outgoing != "" {
			if _, err := s.runner.Run(ctx, "ufw", "default", outgoing, "outgoing"); err != nil {
				return nil, fmt.Errorf("ufw default %s outgoing: %w", outgoing, err)
			}
		}
	}

	defaults, err := s.Defaults(ctx)
	if err != nil {
		return nil, err
	}
	if s.dryRun {
		// Nothing changed on disk; show what the command would have set.
		if incoming != "" {
			defaults.Incoming = incoming
		}
		if outgoing != "" {
			defaults.Outgoing = outgoing
		}
	}
	return &DefaultsResult{Backend: s.backend, Defaults: defaults, SSHCheck: check}, nil
}

// SetPing allows or blocks inbound ICMP echo requests. mode is "allow" or "block".
func (s *Service) SetPing(ctx context.Context, mode string) (*PingResult, error) {
	if err := s.requireBackend(); err != nil {
		return nil, err
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != PingAllow && mode != PingBlock {
		return nil, fmt.Errorf("invalid ping setting %q: use allow or block", mode)
	}
	if s.backend == "firewalld" {
		if err := s.firewalldSetPing(ctx, mode); err != nil {
			return nil, err
		}
	} else if err := s.ufwSetPing(ctx, mode); err != nil {
		return nil, err
	}
	return &PingResult{Backend: s.backend, Ping: mode}, nil
}

func normalizePolicy(kind, value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", PolicyAllow, PolicyDeny, PolicyReject:
		return value, nil
	default:
		return "", fmt.Errorf("invalid %s policy %q: use deny, reject, or allow", kind, value)
	}
}

// firewalld

func (s *Service) firewalldDefaultZone(ctx context.Context) (string, error) {
	res, err := s.runner.RunQuery(ctx, "firewall-cmd", "--get-default-zone")
	if err != nil {
		return "", fmt.Errorf("firewall-cmd --get-default-zone: %w", err)
	}
	zone := strings.TrimSpace(res.Stdout)
	if zone == "" {
		return "", fmt.Errorf("firewalld did not report a default zone")
	}
	return zone, nil
}

func (s *Service) firewalldDefaults(ctx context.Context) (*Defaults, error) {
	zone, err := s.firewalldDefaultZone(ctx)
	if err != nil {
		return nil, err
	}
	res, err := s.runner.RunQuery(ctx, "firewall-cmd", "--permanent", "--zone="+zone, "--get-target")
	if err != nil {
		return nil, fmt.Errorf("firewall-cmd --get-target: %w", err)
	}
	incoming, err := firewalldTargetPolicy(res.Stdout)
	if err != nil {
		return nil, err
	}
	return &Defaults{
		Incoming: incoming,
		// firewalld does not filter traffic the host sends.
		Outgoing: PolicyAllow,
		Ping:     s.firewalldPing(ctx, zone),
	}, nil
}

// firewalldTargetPolicy maps a zone target to an Abstrax policy.
// "default" behaves like REJECT. DROP is the silent deny.
func firewalldTargetPolicy(target string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(target)) {
	case "DEFAULT", "REJECT", "%%REJECT%%":
		return PolicyReject, nil
	case "DROP":
		return PolicyDeny, nil
	case "ACCEPT":
		return PolicyAllow, nil
	default:
		return "", fmt.Errorf("unrecognized firewalld zone target %q", strings.TrimSpace(target))
	}
}

func firewalldPolicyTarget(policy string) (string, error) {
	switch policy {
	case PolicyDeny:
		return "DROP", nil
	case PolicyReject:
		return "%%REJECT%%", nil
	case PolicyAllow:
		return "ACCEPT", nil
	default:
		return "", fmt.Errorf("invalid incoming policy %q", policy)
	}
}

func (s *Service) firewalldSetIncoming(ctx context.Context, policy string) error {
	if policy == "" {
		return nil
	}
	target, err := firewalldPolicyTarget(policy)
	if err != nil {
		return err
	}
	zone, err := s.firewalldDefaultZone(ctx)
	if err != nil {
		return err
	}
	if _, err := s.runner.Run(ctx, "firewall-cmd", "--permanent", "--zone="+zone, "--set-target="+target); err != nil {
		return fmt.Errorf("setting zone target: %w", err)
	}
	if _, err := s.runner.Run(ctx, "firewall-cmd", "--reload"); err != nil {
		return fmt.Errorf("reloading firewalld: %w", err)
	}
	return nil
}

// firewalldPing reports whether echo-request is blocked in the zone.
func (s *Service) firewalldPing(ctx context.Context, zone string) string {
	res, _ := s.runner.RunQuery(ctx, "firewall-cmd", "--zone="+zone, "--query-icmp-block=echo-request")
	// firewall-cmd exits 1 for "no", so read the answer before the error.
	switch strings.ToLower(strings.TrimSpace(res.Stdout)) {
	case "yes":
		return PingBlock
	case "no":
		return PingAllow
	}
	return PingUnknown
}

// firewalldSetPing blocks or allows echo-request in the default zone. The
// firewalld echo-request ICMP type covers both IPv4 and IPv6, so one entry is enough.
func (s *Service) firewalldSetPing(ctx context.Context, mode string) error {
	zone, err := s.firewalldDefaultZone(ctx)
	if err != nil {
		return err
	}
	flag := "--add-icmp-block=echo-request"
	if mode == PingAllow {
		flag = "--remove-icmp-block=echo-request"
	}
	if _, err := s.runner.Run(ctx, "firewall-cmd", "--permanent", "--zone="+zone, flag); err != nil {
		return fmt.Errorf("setting icmp block: %w", err)
	}
	if _, err := s.runner.Run(ctx, "firewall-cmd", "--reload"); err != nil {
		return fmt.Errorf("reloading firewalld: %w", err)
	}
	return nil
}
