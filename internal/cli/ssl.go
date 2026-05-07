package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"abstrax/internal/actions"
	"abstrax/internal/globals"
	"abstrax/internal/output"
	"abstrax/internal/platform"
	"abstrax/internal/services/ssl"
)

// NewSSLCmd returns the ssl command.
func NewSSLCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ssl",
		Short: "Manage SSL certificates (Certbot)",
	}

	cmd.AddCommand(newSSLAddCmd())
	cmd.AddCommand(newSSLRemoveCmd())
	cmd.AddCommand(newSSLRenewCmd())
	cmd.AddCommand(newSSLStatusCmd())

	return cmd
}

func newSSLAddCmd() *cobra.Command {
	opts := ssl.AddOptions{RedirectHTTP: true}
	var domainsStr string

	cmd := &cobra.Command{
		Use:   "add <project>",
		Short: "Obtain an SSL certificate for a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.ProjectName = args[0]
			opts.DryRun = globals.Flags.DryRun

			if err := platform.RequireRoot(); err != nil {
				return err
			}

			if domainsStr != "" {
				opts.Domains = strings.Split(domainsStr, ",")
			}

			svc := ssl.New(opts.DryRun, globals.Flags.Verbose)
			if err := svc.Add(cmd.Context(), opts); err != nil {
				return err
			}

			return printSimpleResult(actions.SSLAdd,
				"SSL certificate obtained.", nil)
		},
	}

	cmd.Flags().StringVar(&domainsStr, "domains", "", "Comma-separated domain names")
	cmd.Flags().StringVar(&opts.Email, "email", "", "Email for certificate registration")
	cmd.Flags().BoolVar(&opts.Staging, "staging", false, "Use Let's Encrypt staging server")
	cmd.Flags().BoolVar(&opts.RedirectHTTP, "redirect-http", true, "Redirect HTTP to HTTPS")

	return cmd
}

func newSSLRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <project>",
		Short: "Remove SSL certificate for a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := platform.RequireRoot(); err != nil {
				return err
			}

			svc := ssl.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.Remove(cmd.Context(), args[0]); err != nil {
				return err
			}

			return printSimpleResult(actions.SSLRemove,
				"SSL certificate removed.", nil)
		},
	}
}

func newSSLRenewCmd() *cobra.Command {
	opts := ssl.RenewOptions{}

	cmd := &cobra.Command{
		Use:   "renew",
		Short: "Renew SSL certificates",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.DryRun = globals.Flags.DryRun
			if err := platform.RequireRoot(); err != nil {
				return err
			}

			svc := ssl.New(opts.DryRun, globals.Flags.Verbose)
			if err := svc.Renew(cmd.Context(), opts); err != nil {
				return err
			}

			return printSimpleResult(actions.SSLRenew, "Certificates renewed.", nil)
		},
	}

	cmd.Flags().StringVar(&opts.Project, "project", "", "Renew only this project's certificate")

	return cmd
}

func newSSLStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [project]",
		Short: "Show SSL certificate status",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectName := ""
			if len(args) > 0 {
				projectName = args[0]
			}

			svc := ssl.New(false, globals.Flags.Verbose)
			statuses, err := svc.Status(cmd.Context(), projectName)
			if err != nil {
				return err
			}

			if globals.Flags.JSON {
				output.PrintJSON(output.Success(actions.SSLStatus, "", statuses))
				return nil
			}

			if len(statuses) == 0 {
				printer().Line("No SSL certificates found.")
				return nil
			}

			t := output.NewTable([]string{"PROJECT", "DOMAINS", "EXPIRY"})
			for _, s := range statuses {
				t.Append([]string{
					s.ProjectName,
					strings.Join(s.Domains, ", "),
					s.Expiry,
				})
			}
			t.Render()
			return nil
		},
	}
}
