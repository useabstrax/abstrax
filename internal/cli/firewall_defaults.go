package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"abstrax/internal/actions"
	"abstrax/internal/confirm"
	"abstrax/internal/globals"
	"abstrax/internal/output"
	"abstrax/internal/services/firewall"
)

const defaultsLockoutHelp = "Setting --incoming deny or reject while the firewall is active is refused when no rule allows TCP to the SSH port, unless the policy already blocks incoming traffic. Pass --force to do it anyway. --yes does not skip the check."

const incomingAllowHelp = "Setting --incoming allow opens every port that has no rule against it, so it asks for confirmation. Pass --yes to skip the prompt in scripts."

func newFirewallDefaultsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "defaults",
		Short: "Show or change the default firewall policy",
		Long: `Show the default incoming and outgoing policy and whether ping is allowed.

  UFW:       read from "ufw status verbose" (or /etc/default/ufw when inactive)
  firewalld: the target of the default zone (DROP is deny, %%REJECT%% is reject)

Use "abstrax firewall defaults set" to change the policy and
"abstrax firewall ping allow|block" to change ping.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := firewall.New(false, globals.Flags.Verbose)
			result, err := svc.DefaultsInfo(cmd.Context())
			if err != nil {
				return err
			}

			if machineOutput() {
				emitResult(output.Success(actions.FirewallDefaults, "", result))
				return nil
			}

			p := printer()
			p.Line("")
			p.Line("  %-12s %s", "Firewall:", result.Backend)
			printFirewallDefaults(p, result.Defaults)
			p.Line("")
			return nil
		},
	}
	cmd.AddCommand(newFirewallDefaultsSetCmd())
	return cmd
}

func newFirewallDefaultsSetCmd() *cobra.Command {
	opts := firewall.DefaultsSetOptions{}

	cmd := &cobra.Command{
		Use:   "set --incoming deny|reject|allow [--outgoing allow|deny|reject]",
		Short: "Set the default incoming (and outgoing) policy",
		Long: `Set the default policy for traffic that matches no rule.

  UFW:       ufw default <policy> incoming|outgoing
  firewalld: sets the target of the default zone (deny is DROP, reject is
             REJECT, allow is ACCEPT) and reloads. --outgoing is not supported.

` + incomingAllowHelp + "\n\n" + defaultsLockoutHelp,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(opts.Incoming) == "" {
				return fmt.Errorf("--incoming is required: deny, reject, or allow")
			}
			if err := requireRootAndSupported(); err != nil {
				return err
			}

			if strings.EqualFold(strings.TrimSpace(opts.Incoming), firewall.PolicyAllow) {
				ok, err := confirmIncomingAllow(globals.Flags.Yes)
				if err != nil {
					return err
				}
				if !ok {
					return nil
				}
			}

			svc := firewall.New(globals.Flags.DryRun, globals.Flags.Verbose)
			result, err := svc.SetDefaults(cmd.Context(), opts)
			if err != nil {
				return err
			}

			msg := fmt.Sprintf("Default incoming policy is %s.", result.Defaults.Incoming)
			if strings.TrimSpace(opts.Outgoing) != "" {
				msg = fmt.Sprintf("Default incoming policy is %s, outgoing is %s.",
					result.Defaults.Incoming, result.Defaults.Outgoing)
			}
			return printSimpleResult(actions.FirewallDefaultsSet, msg, result)
		},
	}

	cmd.Flags().StringVar(&opts.Incoming, "incoming", "", "Default for incoming traffic (deny|reject|allow)")
	cmd.Flags().StringVar(&opts.Outgoing, "outgoing", "", "Default for outgoing traffic (allow|deny|reject). UFW only")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "Apply the policy even if it would block SSH")
	return cmd
}

// confirmIncomingAllow warns that an allow-all incoming default exposes every
// port, then asks for confirmation unless yes is set. Without a terminal the
// prompt fails, so scripts must pass --yes.
func confirmIncomingAllow(yes bool) (bool, error) {
	p := printer()
	p.Warn("Allowing all incoming traffic by default exposes every port that has no rule against it.")
	return confirm.Ask("Set the default incoming policy to allow?", yes)
}

func newFirewallPingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ping allow|block",
		Short: "Allow or block inbound ping (ICMP echo requests)",
		Long: `Allow or block inbound ICMP echo requests (ping).

  UFW:       edits the echo-request rules in /etc/ufw/before.rules and
             before6.rules (ACCEPT or DROP), keeps a .bak of each changed
             file, then runs "ufw reload".
  firewalld: adds or removes the echo-request ICMP block in the default zone
             and reloads.

Run "abstrax firewall defaults" to see the current setting.`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{firewall.PingAllow, firewall.PingBlock},
		RunE: func(cmd *cobra.Command, args []string) error {
			mode := strings.ToLower(strings.TrimSpace(args[0]))
			if mode != firewall.PingAllow && mode != firewall.PingBlock {
				return fmt.Errorf("invalid ping setting %q: use allow or block", args[0])
			}
			if err := requireRootAndSupported(); err != nil {
				return err
			}

			svc := firewall.New(globals.Flags.DryRun, globals.Flags.Verbose)
			result, err := svc.SetPing(cmd.Context(), mode)
			if err != nil {
				return err
			}

			msg := "Ping is allowed."
			if result.Ping == firewall.PingBlock {
				msg = "Ping is blocked."
			}
			return printSimpleResult(actions.FirewallPing, msg, result)
		},
	}
	return cmd
}
