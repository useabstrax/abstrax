package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"abstrax/internal/actions"
	"abstrax/internal/confirm"
	"abstrax/internal/globals"
	"abstrax/internal/output"
	"abstrax/internal/platform"
	"abstrax/internal/services/firewall"
	"abstrax/internal/validate"
)

// NewFirewallCmd returns the firewall command.
func NewFirewallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "firewall",
		Short: "Manage the system firewall (UFW)",
	}

	ruleCmd := &cobra.Command{
		Use:   "rule",
		Short: "Manage firewall rules",
	}
	ruleCmd.AddCommand(newFirewallRuleListCmd())
	ruleCmd.AddCommand(newFirewallRuleRemoveCmd())

	cmd.AddCommand(newFirewallStatusCmd())
	cmd.AddCommand(newFirewallEnableCmd())
	cmd.AddCommand(newFirewallDisableCmd())
	cmd.AddCommand(newFirewallAllowCmd())
	cmd.AddCommand(newFirewallDenyCmd())
	cmd.AddCommand(newFirewallAllowIPCmd())
	cmd.AddCommand(newFirewallDenyIPCmd())
	cmd.AddCommand(ruleCmd)

	return cmd
}

func newFirewallStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show firewall status",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := firewall.New(false, globals.Flags.Verbose)
			status, err := svc.GetStatus(cmd.Context())
			if err != nil {
				return err
			}

			p := printer()
			if globals.Flags.JSON {
				output.PrintJSON(output.Success(actions.FirewallStatus, "", status))
				return nil
			}

			active := "inactive"
			if status.Active {
				active = "active"
			}
			p.Line("")
			p.Line("  %-12s %s", "Firewall:", status.Backend)
			p.Line("  %-12s %s", "Status:", active)
			if len(status.Rules) > 0 {
				p.Line("")
				p.Line("  Rules:")
				for _, r := range status.Rules {
					p.Line("    [%s] %-20s %s", r.ID, r.Port, r.Action)
				}
			}
			p.Line("")
			return nil
		},
	}
}

func newFirewallEnableCmd() *cobra.Command {
	opts := firewall.EnableOptions{}

	cmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable the firewall",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := platform.RequireRoot(); err != nil {
				return err
			}

			opts.DryRun = globals.Flags.DryRun

			if !opts.AllowSSH {
				p := printer()
				p.Warn("Enabling the firewall without opening SSH may lock you out.")
				p.Warn("Use --allow-ssh to ensure SSH access is preserved.")
			}

			ok, err := confirm.Ask(
				"Enable firewall? Ensure SSH access is allowed first.",
				globals.Flags.Yes,
			)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}

			svc := firewall.New(opts.DryRun, globals.Flags.Verbose)
			if err := svc.Enable(cmd.Context(), opts); err != nil {
				return err
			}

			return printSimpleResult(actions.FirewallEnable, "Firewall enabled.", nil)
		},
	}

	cmd.Flags().BoolVar(&opts.AllowSSH, "allow-ssh", false, "Open SSH port before enabling")
	cmd.Flags().IntVar(&opts.SSHPort, "ssh-port", 22, "SSH port to allow")

	return cmd
}

func newFirewallDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable",
		Short: "Disable the firewall",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := platform.RequireRoot(); err != nil {
				return err
			}

			ok, err := confirm.Ask("Disable firewall?", globals.Flags.Yes)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}

			svc := firewall.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.Disable(cmd.Context()); err != nil {
				return err
			}

			return printSimpleResult(actions.FirewallDisable, "Firewall disabled.", nil)
		},
	}
}

func newFirewallAllowCmd() *cobra.Command {
	opts := firewall.AllowOptions{}

	cmd := &cobra.Command{
		Use:   "allow <port>",
		Short: "Allow traffic on a port",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Port = args[0]
			opts.DryRun = globals.Flags.DryRun

			if err := validate.PortString(opts.Port); err != nil {
				return err
			}
			if err := platform.RequireRoot(); err != nil {
				return err
			}

			svc := firewall.New(opts.DryRun, globals.Flags.Verbose)
			if err := svc.Allow(cmd.Context(), opts); err != nil {
				return err
			}

			return printSimpleResult(actions.FirewallAllow,
				fmt.Sprintf("Port %s allowed.", opts.Port), nil)
		},
	}

	cmd.Flags().StringVar(&opts.Protocol, "protocol", "", "Protocol (tcp|udp)")
	cmd.Flags().StringVar(&opts.From, "from", "", "Allow only from this IP or CIDR")
	cmd.Flags().StringVar(&opts.Comment, "comment", "", "Rule comment")

	return cmd
}

func newFirewallDenyCmd() *cobra.Command {
	opts := firewall.AllowOptions{}

	cmd := &cobra.Command{
		Use:   "deny <port>",
		Short: "Deny traffic on a port",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Port = args[0]
			opts.DryRun = globals.Flags.DryRun

			if err := platform.RequireRoot(); err != nil {
				return err
			}

			svc := firewall.New(opts.DryRun, globals.Flags.Verbose)
			if err := svc.Deny(cmd.Context(), opts); err != nil {
				return err
			}

			return printSimpleResult(actions.FirewallDeny,
				fmt.Sprintf("Port %s denied.", opts.Port), nil)
		},
	}

	cmd.Flags().StringVar(&opts.Protocol, "protocol", "", "Protocol (tcp|udp)")
	return cmd
}

func newFirewallAllowIPCmd() *cobra.Command {
	opts := firewall.AllowOptions{}

	cmd := &cobra.Command{
		Use:   "allow-ip <ip-or-cidr>",
		Short: "Allow all traffic from an IP or CIDR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.From = args[0]
			opts.DryRun = globals.Flags.DryRun

			if err := validate.CIDRRange(opts.From); err != nil {
				return err
			}
			if err := platform.RequireRoot(); err != nil {
				return err
			}

			svc := firewall.New(opts.DryRun, globals.Flags.Verbose)
			if err := svc.AllowIP(cmd.Context(), opts); err != nil {
				return err
			}

			return printSimpleResult(actions.FirewallAllowIP,
				fmt.Sprintf("Traffic from %s allowed.", opts.From), nil)
		},
	}

	cmd.Flags().StringVar(&opts.To, "to", "", "Destination IP")
	cmd.Flags().StringVar(&opts.Port, "port", "", "Specific port")

	return cmd
}

func newFirewallDenyIPCmd() *cobra.Command {
	opts := firewall.AllowOptions{}

	return &cobra.Command{
		Use:   "deny-ip <ip-or-cidr>",
		Short: "Deny all traffic from an IP or CIDR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.From = args[0]
			opts.DryRun = globals.Flags.DryRun

			if err := validate.CIDRRange(opts.From); err != nil {
				return err
			}
			if err := platform.RequireRoot(); err != nil {
				return err
			}

			svc := firewall.New(opts.DryRun, globals.Flags.Verbose)
			if err := svc.DenyIP(cmd.Context(), opts); err != nil {
				return err
			}

			return printSimpleResult(actions.FirewallDenyIP,
				fmt.Sprintf("Traffic from %s denied.", opts.From), nil)
		},
	}
}

func newFirewallRuleListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List firewall rules",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := firewall.New(false, globals.Flags.Verbose)
			rules, err := svc.RuleList(cmd.Context())
			if err != nil {
				return err
			}

			if globals.Flags.JSON {
				output.PrintJSON(output.Success(actions.FirewallRuleList, "", rules))
				return nil
			}

			if len(rules) == 0 {
				printer().Line("No firewall rules.")
				return nil
			}

			t := output.NewTable([]string{"ID", "PORT/IP", "PROTOCOL", "ACTION"})
			for _, r := range rules {
				t.Append([]string{r.ID, r.Port, r.Protocol, r.Action})
			}
			t.Render()
			return nil
		},
	}
}

func newFirewallRuleRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove a firewall rule by number",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			if err := platform.RequireRoot(); err != nil {
				return err
			}

			svc := firewall.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.RuleRemove(cmd.Context(), id); err != nil {
				return err
			}

			return printSimpleResult(actions.FirewallRuleRm,
				fmt.Sprintf("Rule %s removed.", id), nil)
		},
	}
}
