package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"abstrax/internal/actions"
	"abstrax/internal/globals"
	"abstrax/internal/output"
	"abstrax/internal/services/project"
)

func newProjectRedirectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "redirect",
		Short: "Manage a project's path redirects",
		Long: `Manage path redirects in a project's managed nginx vhost.

Redirects are exact-path matches (location = /path) rendered before the
project's runtime locations, and are kept when "project modify" rewrites
the vhost. The project must have an nginx vhost.`,
	}
	cmd.AddCommand(newProjectRedirectListCmd())
	cmd.AddCommand(newProjectRedirectAddCmd())
	cmd.AddCommand(newProjectRedirectModifyCmd())
	cmd.AddCommand(newProjectRedirectRemoveCmd())
	return cmd
}

func newProjectRedirectListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <project>",
		Short: "List a project's redirects",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := project.New(false, globals.Flags.Verbose)
			redirects, err := svc.ListRedirects(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if redirects == nil {
				redirects = []project.Redirect{}
			}
			if machineOutput() {
				emitResult(output.Success(actions.ProjectRedirectList, "", redirects))
				return nil
			}
			if len(redirects) == 0 {
				printer().Line("No redirects found.")
				return nil
			}
			t := output.NewTable([]string{"ID", "SOURCE", "DESTINATION", "CODE"})
			for _, r := range redirects {
				t.Append([]string{r.ID, r.Source, r.Destination, strconv.Itoa(r.Code)})
			}
			t.Render()
			return nil
		},
	}
}

func newProjectRedirectAddCmd() *cobra.Command {
	var code int
	cmd := &cobra.Command{
		Use:   "add <project> <source> <destination>",
		Short: "Add a redirect from a path to a path or URL",
		Long: `Add a redirect. <source> is an exact path starting with /. <destination> is
a path starting with / or an absolute http(s) URL. --code is one of 301
(default), 302, 307, or 308.`,
		Example: `  abstrax project redirect add mysite /old-page /new-page
  abstrax project redirect add mysite /blog https://blog.example.com/ --code 302`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireRootAndSupported(); err != nil {
				return err
			}
			svc := project.New(globals.Flags.DryRun, globals.Flags.Verbose)
			r, err := svc.AddRedirect(cmd.Context(), project.AddRedirectOptions{
				Project:     args[0],
				Source:      args[1],
				Destination: args[2],
				Code:        code,
				DryRun:      globals.Flags.DryRun,
			})
			if err != nil {
				return err
			}
			return printSimpleResult(actions.ProjectRedirectAdd,
				fmt.Sprintf("Redirect %s added: %s -> %s (%d).", r.ID, r.Source, r.Destination, r.Code), r)
		},
	}
	cmd.Flags().IntVar(&code, "code", project.DefaultRedirectCode, "HTTP status code (301, 302, 307, 308)")
	return cmd
}

func newProjectRedirectModifyCmd() *cobra.Command {
	var source, destination string
	var code int
	cmd := &cobra.Command{
		Use:   "modify <project> <id>",
		Short: "Change a redirect",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireRootAndSupported(); err != nil {
				return err
			}
			svc := project.New(globals.Flags.DryRun, globals.Flags.Verbose)
			r, err := svc.ModifyRedirect(cmd.Context(), project.ModifyRedirectOptions{
				Project:     args[0],
				ID:          args[1],
				Source:      source,
				Destination: destination,
				Code:        code,
				DryRun:      globals.Flags.DryRun,
			})
			if err != nil {
				return err
			}
			return printSimpleResult(actions.ProjectRedirectModify,
				fmt.Sprintf("Redirect %s updated: %s -> %s (%d).", r.ID, r.Source, r.Destination, r.Code), r)
		},
	}
	cmd.Flags().StringVar(&source, "source", "", "New source path")
	cmd.Flags().StringVar(&destination, "destination", "", "New destination path or URL")
	cmd.Flags().IntVar(&code, "code", 0, "New HTTP status code (301, 302, 307, 308)")
	return cmd
}

func newProjectRedirectRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <project> <id>",
		Short: "Remove a redirect",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireRootAndSupported(); err != nil {
				return err
			}
			svc := project.New(globals.Flags.DryRun, globals.Flags.Verbose)
			r, err := svc.RemoveRedirect(cmd.Context(), args[0], args[1], globals.Flags.DryRun)
			if err != nil {
				return err
			}
			return printSimpleResult(actions.ProjectRedirectRemove,
				fmt.Sprintf("Redirect %s removed (%s).", r.ID, r.Source), r)
		},
	}
}
