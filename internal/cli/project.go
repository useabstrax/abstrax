package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"abstrax/internal/actions"
	"abstrax/internal/confirm"
	"abstrax/internal/globals"
	"abstrax/internal/output"
	"abstrax/internal/platform"
	"abstrax/internal/services/project"
	"abstrax/internal/services/ssl"
	"abstrax/internal/validate"
)

// NewProjectCmd returns the project command.
func NewProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage web application projects",
	}

	cmd.AddCommand(newProjectAddCmd())
	cmd.AddCommand(newProjectRemoveCmd())
	cmd.AddCommand(newProjectModifyCmd())
	cmd.AddCommand(newProjectListCmd())
	cmd.AddCommand(newProjectInfoCmd())
	cmd.AddCommand(newProjectInspectCmd())
	cmd.AddCommand(newProjectServiceCmd())
	cmd.AddCommand(newProjectEnableCmd())
	cmd.AddCommand(newProjectDisableCmd())
	cmd.AddCommand(newProjectReloadCmd())

	return cmd
}

func newProjectAddCmd() *cobra.Command {
	opts := project.AddOptions{
		WebServer:    project.WebServerNginx,
		RedirectHTTP: true,
	}
	var domainsStr string
	var nginxFlag, apacheFlag bool

	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Create a new project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Name = args[0]
			opts.DryRun = globals.Flags.DryRun
			opts.Yes = globals.Flags.Yes

			if err := validate.ProjectName(opts.Name); err != nil {
				return err
			}
			if err := platform.RequireRoot(); err != nil {
				return err
			}

			opts.UserExplicit = cmd.Flags().Changed("user")
			if !opts.UserExplicit {
				opts.User = project.SharedWebUser
				if cmd.Flags().Changed("group") {
					// keep explicit group for shared mode
				} else {
					opts.Group = project.SharedWebGroup
				}
			} else if err := validate.Username(opts.User); err != nil {
				return err
			}

			if opts.Path == "" && !opts.UserExplicit {
				opts.Path = project.DefaultSharedBase + "/" + opts.Name
			}

			if apacheFlag {
				opts.WebServer = project.WebServerApache
			} else if nginxFlag {
				opts.WebServer = project.WebServerNginx
			}

			if domainsStr != "" {
				opts.Domains = strings.Split(domainsStr, ",")
				for _, d := range opts.Domains {
					if err := validate.Domain(strings.TrimSpace(d)); err != nil {
						return err
					}
				}
			}

			if opts.SSL {
				if err := validateProjectSSLOptions(opts); err != nil {
					return err
				}
			}

			svc := project.New(opts.DryRun, globals.Flags.Verbose)
			state, err := svc.Add(cmd.Context(), opts)
			if err != nil {
				return err
			}

			if opts.SSL {
				if err := enableProjectSSL(cmd.Context(), opts); err != nil {
					return err
				}
				state, err = svc.Info(cmd.Context(), opts.Name)
				if err != nil {
					return err
				}
			}

			p := printer()
			r := output.Success(actions.ProjectAdd,
				fmt.Sprintf("Project %s created.", opts.Name), state)

			if globals.Flags.JSON {
				output.PrintJSON(r)
				return nil
			}

			p.Success("Project %s created.", opts.Name)
			p.Line("  Path:       %s", state.Path)
			p.Line("  Owner:      %s:%s", state.Owner, state.Group)
			if state.OwnershipMode == project.OwnershipIsolated {
				p.Line("  Mode:       user isolated")
			}
			p.Line("  Web server: %s", state.WebServer)
			if len(state.Domains) > 0 {
				p.Line("  Domains:    %s", strings.Join(state.Domains, ", "))
			}
			if state.VhostPath != "" {
				p.Line("  Vhost:      %s", state.VhostPath)
			}
			if state.SSLEnabled {
				p.Line("  SSL:        enabled")
			}
			if example := project.DaemonAddExampleFor(state); example != nil {
				p.Line("")
				p.Line("  Nginx proxies to 127.0.0.1:%d. Start your app with a managed daemon, for example:", example.Port)
				for _, line := range example.FormatLines() {
					p.Line("    %s", line)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.Path, "path", "", "Project root path")
	cmd.Flags().BoolVar(&nginxFlag, "nginx", true, "Use nginx (default)")
	cmd.Flags().BoolVar(&apacheFlag, "apache", false, "Use Apache (not yet implemented)")
	cmd.Flags().BoolVar(&opts.NoVhost, "no-vhost", false, "Do not create a virtual host")
	cmd.Flags().StringVar(&domainsStr, "domains", "", "Comma-separated domain names")
	cmd.Flags().IntVar(&opts.Port, "port", 80, "HTTP port")
	cmd.Flags().StringVar(&opts.WebRoot, "web-root", "", "Custom web root directory")
	cmd.Flags().StringVar(&opts.User, "user", "", "Linux user for a user-owned project (omit for shared www-data mode)")
	cmd.Flags().StringVar(&opts.Group, "group", "", "Project group for shared mode (default: www-data)")
	cmd.Flags().BoolVar(&opts.SSL, "ssl", false, "Enable SSL (requires certbot)")
	cmd.Flags().StringVar(&opts.Email, "email", "", "Email for SSL certificate")
	cmd.Flags().BoolVar(&opts.RedirectHTTP, "redirect-http", true, "Redirect HTTP to HTTPS")

	// Runtime flags.
	var phpFlag, nodeFlag, rubyFlag, staticFlag bool
	cmd.Flags().BoolVar(&phpFlag, "php", false, "PHP application")
	cmd.Flags().BoolVar(&nodeFlag, "node", false, "Node.js application")
	cmd.Flags().BoolVar(&rubyFlag, "ruby", false, "Ruby application")
	cmd.Flags().BoolVar(&staticFlag, "static", false, "Static site (default)")
	cmd.Flags().StringVar(&opts.PHPVersion, "php-version", project.DefaultPHPVersion, "PHP version")
	cmd.Flags().StringVar(&opts.NodeVersion, "node-version", project.DefaultNodeVersion, "Node.js version")
	cmd.Flags().StringVar(&opts.RubyVersion, "ruby-version", project.DefaultRubyVersion, "Ruby version")
	cmd.Flags().StringVar(&opts.PublicDir, "public-dir", "", "Public directory relative to path")
	cmd.Flags().IntVar(&opts.ProxyPort, "proxy-port", 0, "Proxy to this local port (node/ruby)")

	// Resolve runtime from flags in PreRunE is tricky with cobra, so we use
	// a post-parse step.
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		switch {
		case phpFlag:
			opts.Runtime = project.RuntimePHP
		case nodeFlag:
			opts.Runtime = project.RuntimeNode
		case rubyFlag:
			opts.Runtime = project.RuntimeRuby
		default:
			opts.Runtime = project.RuntimeStatic
		}
		return nil
	}

	return cmd
}

func validateProjectSSLOptions(opts project.AddOptions) error {
	if len(opts.Domains) == 0 {
		return fmt.Errorf("--domains is required when using --ssl")
	}
	if opts.Email == "" {
		return fmt.Errorf("--email is required when using --ssl")
	}
	if opts.NoVhost {
		return fmt.Errorf("--ssl requires a virtual host; do not use --no-vhost")
	}
	if opts.WebServer != project.WebServerNginx {
		return fmt.Errorf("--ssl requires nginx")
	}
	return nil
}

func enableProjectSSL(ctx context.Context, opts project.AddOptions) error {
	sslSvc := ssl.New(opts.DryRun, globals.Flags.Verbose)
	return sslSvc.Add(ctx, ssl.AddOptions{
		ProjectName:  opts.Name,
		Domains:      opts.Domains,
		Email:        opts.Email,
		Staging:      opts.Staging,
		RedirectHTTP: opts.RedirectHTTP,
		DryRun:       opts.DryRun,
	})
}

func newProjectRemoveCmd() *cobra.Command {
	opts := project.RemoveOptions{KeepFiles: true}

	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Name = args[0]
			opts.DryRun = globals.Flags.DryRun

			if err := platform.RequireRoot(); err != nil {
				return err
			}

			ok, err := confirm.Ask(
				fmt.Sprintf("Remove project %q?", opts.Name),
				globals.Flags.Yes,
			)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}

			svc := project.New(opts.DryRun, globals.Flags.Verbose)
			if err := svc.Remove(cmd.Context(), opts); err != nil {
				return err
			}

			return printSimpleResult(actions.ProjectRemove,
				fmt.Sprintf("Project %s removed.", opts.Name), nil)
		},
	}

	cmd.Flags().BoolVar(&opts.RemoveVhost, "remove-vhost", true, "Remove nginx vhost")
	cmd.Flags().BoolVar(&opts.RemoveSSL, "remove-ssl", false, "Remove SSL certificate")
	cmd.Flags().BoolVar(&opts.DeleteFiles, "delete-files", false, "Delete project files")
	cmd.Flags().BoolVar(&opts.KeepFiles, "keep-files", true, "Keep project files (default)")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "Force removal without confirmation")

	return cmd
}

func newProjectModifyCmd() *cobra.Command {
	opts := project.ModifyOptions{}
	var domainsStr string

	cmd := &cobra.Command{
		Use:   "modify <name>",
		Short: "Modify a project's configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Name = args[0]
			opts.DryRun = globals.Flags.DryRun
			opts.Yes = globals.Flags.Yes

			if err := platform.RequireRoot(); err != nil {
				return err
			}

			if domainsStr != "" {
				opts.Domains = strings.Split(domainsStr, ",")
			}

			svc := project.New(opts.DryRun, globals.Flags.Verbose)
			state, err := svc.Modify(cmd.Context(), opts)
			if err != nil {
				return err
			}

			return printSimpleResult(actions.ProjectModify,
				fmt.Sprintf("Project %s updated.", opts.Name), state)
		},
	}

	cmd.Flags().StringVar(&opts.Path, "path", "", "Project root path")
	cmd.Flags().StringVar(&domainsStr, "domains", "", "Comma-separated domain names")
	cmd.Flags().StringVar(&opts.AddDomain, "add-domain", "", "Add a domain")
	cmd.Flags().StringVar(&opts.RemoveDomain, "remove-domain", "", "Remove a domain")
	cmd.Flags().StringVar(&opts.PHPVersion, "php-version", "", "PHP version")
	cmd.Flags().StringVar(&opts.NodeVersion, "node-version", "", "Node.js version")
	cmd.Flags().StringVar(&opts.RubyVersion, "ruby-version", "", "Ruby version")
	cmd.Flags().StringVar(&opts.PublicDir, "public-dir", "", "Public directory")
	cmd.Flags().IntVar(&opts.ProxyPort, "proxy-port", 0, "Proxy port")

	return cmd
}

func newProjectListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List managed projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := project.New(false, globals.Flags.Verbose)
			projects, err := svc.List(cmd.Context())
			if err != nil {
				return err
			}

			if globals.Flags.JSON {
				output.PrintJSON(output.Success(actions.ProjectList, "", projects))
				return nil
			}

			if len(projects) == 0 {
				printer().Line("No projects found.")
				return nil
			}

			t := output.NewTable([]string{"NAME", "PATH", "WEB SERVER", "RUNTIME", "DOMAINS"})
			for _, p := range projects {
				t.Append([]string{
					p.Name,
					p.Path,
					string(p.WebServer),
					string(p.Runtime),
					strings.Join(p.Domains, ", "),
				})
			}
			t.Render()
			return nil
		},
	}
}

func newProjectInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <name>",
		Short: "Show project details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := project.New(false, globals.Flags.Verbose)
			state, err := svc.Info(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			p := printer()
			if globals.Flags.JSON {
				output.PrintJSON(output.Success(actions.ProjectInfo, "", state))
				return nil
			}

			p.Line("")
			p.Line("  %-14s %s", "Name:", state.Name)
			p.Line("  %-14s %s", "Path:", state.Path)
			p.Line("  %-14s %s", "Web server:", state.WebServer)
			p.Line("  %-14s %s", "Runtime:", state.Runtime)
			switch state.Runtime {
			case project.RuntimePHP:
				if state.PHPVersion != "" {
					p.Line("  %-14s %s", "PHP version:", state.PHPVersion)
				}
				if state.PublicDir != "" {
					p.Line("  %-14s %s", "Public dir:", state.PublicDir)
				}
			case project.RuntimeNode:
				if state.NodeVersion != "" {
					p.Line("  %-14s %s", "Node version:", state.NodeVersion)
				}
				if state.ProxyPort != 0 {
					p.Line("  %-14s %s", "Proxy port:", fmt.Sprintf("%d", state.ProxyPort))
				}
			case project.RuntimeRuby:
				if state.RubyVersion != "" {
					p.Line("  %-14s %s", "Ruby version:", state.RubyVersion)
				}
				if state.ProxyPort != 0 {
					p.Line("  %-14s %s", "Proxy port:", fmt.Sprintf("%d", state.ProxyPort))
				}
			}
			p.Line("  %-14s %s", "Domains:", strings.Join(state.Domains, ", "))
			ssl := "no"
			if state.SSLEnabled {
				ssl = "yes"
			}
			p.Line("  %-14s %s", "SSL:", ssl)
			if state.VhostPath != "" {
				p.Line("  %-14s %s", "Vhost:", state.VhostPath)
			}
			p.Line("  %-14s %s", "Owner:", state.Owner)
			if state.Group != "" {
				p.Line("  %-14s %s", "Group:", state.Group)
			}
			if state.OwnershipMode == project.OwnershipIsolated {
				p.Line("  %-14s %s", "Mode:", "user isolated")
				if state.PHPSocketPath != "" {
					p.Line("  %-14s %s", "PHP socket:", state.PHPSocketPath)
				}
			}
			p.Line("  %-14s %s", "Created:", state.CreatedAt.Format("2006-01-02 15:04:05"))
			p.Line("  %-14s %s", "Updated:", state.UpdatedAt.Format("2006-01-02 15:04:05"))
			p.Line("")
			return nil
		},
	}
}

func newProjectInspectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <name>",
		Short: "Inspect a project (machine-readable API for plugins)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := project.New(false, globals.Flags.Verbose)
			resp, err := svc.Inspect(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if globals.Flags.JSON {
				output.PrintJSON(resp)
				return nil
			}
			p := printer()
			p.Line("Project: %s", resp.Project.Name)
			p.Line("  Path:    %s", resp.Project.Path)
			p.Line("  User:    %s", resp.Project.User)
			p.Line("  Runtime: %s %s", resp.Project.Runtime.Type, resp.Project.Runtime.Version)
			p.Line("  Domains: %s", strings.Join(resp.Project.Domains, ", "))
			if len(resp.Project.Services) > 0 {
				p.Line("  Services:")
				for _, s := range resp.Project.Services {
					p.Line("    - %s (%s)", s.Name, s.Type)
				}
			}
			return nil
		},
	}
}

func newProjectServiceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Manage project-owned services",
	}
	cmd.AddCommand(newProjectServiceRestartCmd())
	cmd.AddCommand(newProjectServiceReloadCmd())
	return cmd
}

func newProjectServiceRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart <project> <service>",
		Short: "Restart a project-owned service",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			if err := validate.DaemonName(args[1]); err != nil {
				return err
			}
			svc := project.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.RestartService(cmd.Context(), args[0], args[1]); err != nil {
				return err
			}
			return printSimpleResult(actions.ProjectServiceRestart,
				fmt.Sprintf("Restarted service %s for project %s.", args[1], args[0]), nil)
		},
	}
}

func newProjectServiceReloadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reload <project> <service>",
		Short: "Reload a project-owned service",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			if err := validate.DaemonName(args[1]); err != nil {
				return err
			}
			svc := project.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.ReloadService(cmd.Context(), args[0], args[1]); err != nil {
				return err
			}
			return printSimpleResult(actions.ProjectServiceReload,
				fmt.Sprintf("Reloaded service %s for project %s.", args[1], args[0]), nil)
		},
	}
}

func newProjectEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <name>",
		Short: "Enable a project's nginx vhost",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			svc := project.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.Enable(cmd.Context(), args[0]); err != nil {
				return err
			}
			return printSimpleResult(actions.ProjectEnable,
				fmt.Sprintf("Project %s enabled.", args[0]), nil)
		},
	}
}

func newProjectDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <name>",
		Short: "Disable a project's nginx vhost",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			svc := project.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.Disable(cmd.Context(), args[0]); err != nil {
				return err
			}
			return printSimpleResult(actions.ProjectDisable,
				fmt.Sprintf("Project %s disabled.", args[0]), nil)
		},
	}
}

func newProjectReloadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reload <name>",
		Short: "Reload nginx for a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			svc := project.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.Reload(cmd.Context(), args[0]); err != nil {
				return err
			}
			return printSimpleResult(actions.ProjectReload,
				fmt.Sprintf("Project %s reloaded.", args[0]), nil)
		},
	}
}
