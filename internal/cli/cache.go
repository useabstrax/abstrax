package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"abstrax/internal/actions"
	"abstrax/internal/confirm"
	"abstrax/internal/globals"
	"abstrax/internal/output"
	"abstrax/internal/platform"
	"abstrax/internal/services/cache"
)

// NewCacheCmd returns the cache command.
func NewCacheCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Manage cache services (Redis, Memcached)",
	}

	cmd.AddCommand(newCacheInstallCmd())
	cmd.AddCommand(newCacheRemoveCmd())
	cmd.AddCommand(newCacheStartCmd())
	cmd.AddCommand(newCacheStopCmd())
	cmd.AddCommand(newCacheRestartCmd())
	cmd.AddCommand(newCacheStatusCmd())
	cmd.AddCommand(newCacheConfigCmd())

	return cmd
}

func newCacheInstallCmd() *cobra.Command {
	opts := cache.InstallOptions{Enable: true, Start: true}

	cmd := &cobra.Command{
		Use:   "install <driver>",
		Short: "Install a cache driver (redis|memcached)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Driver = cache.Driver(args[0])
			opts.DryRun = globals.Flags.DryRun

			if err := platform.RequireRoot(); err != nil {
				return err
			}

			if opts.Driver == cache.DriverRedis && opts.Bind == "0.0.0.0" {
				ok, err := confirm.Ask(
					"WARNING: Binding Redis to 0.0.0.0 exposes it to all interfaces. Continue?",
					globals.Flags.Yes,
				)
				if err != nil {
					return err
				}
				if !ok {
					return nil
				}
			}

			svc := cache.New(opts.DryRun, globals.Flags.Verbose)
			if err := svc.Install(cmd.Context(), opts); err != nil {
				return err
			}

			return printSimpleResult(actions.CacheInstall,
				fmt.Sprintf("%s installed.", opts.Driver), nil)
		},
	}

	cmd.Flags().StringVar(&opts.Version, "version", "", "Package version")
	cmd.Flags().IntVar(&opts.Port, "port", 0, "Port to listen on")
	cmd.Flags().StringVar(&opts.Bind, "bind", "127.0.0.1", "IP to bind to")
	cmd.Flags().StringVar(&opts.Memory, "memory", "", "Memory limit")
	cmd.Flags().BoolVar(&opts.Enable, "enable", true, "Enable service at boot")
	cmd.Flags().BoolVar(&opts.Start, "start", true, "Start service after install")

	return cmd
}

func newCacheRemoveCmd() *cobra.Command {
	opts := cache.RemoveOptions{}

	cmd := &cobra.Command{
		Use:   "remove <driver>",
		Short: "Remove a cache driver",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Driver = cache.Driver(args[0])
			opts.DryRun = globals.Flags.DryRun

			if err := platform.RequireRoot(); err != nil {
				return err
			}

			svc := cache.New(opts.DryRun, globals.Flags.Verbose)
			if err := svc.Remove(cmd.Context(), opts); err != nil {
				return err
			}

			return printSimpleResult(actions.CacheRemove,
				fmt.Sprintf("%s removed.", opts.Driver), nil)
		},
	}

	cmd.Flags().BoolVar(&opts.Purge, "purge", false, "Purge configuration files")
	return cmd
}

func newCacheStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <driver>",
		Short: "Start a cache driver",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d := cache.Driver(args[0])
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			svc := cache.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.Start(cmd.Context(), d); err != nil {
				return err
			}
			return printSimpleResult(actions.CacheStart,
				fmt.Sprintf("%s started.", d), nil)
		},
	}
}

func newCacheStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop <driver>",
		Short: "Stop a cache driver",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d := cache.Driver(args[0])
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			svc := cache.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.Stop(cmd.Context(), d); err != nil {
				return err
			}
			return printSimpleResult(actions.CacheStop,
				fmt.Sprintf("%s stopped.", d), nil)
		},
	}
}

func newCacheRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart <driver>",
		Short: "Restart a cache driver",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d := cache.Driver(args[0])
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			svc := cache.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.Restart(cmd.Context(), d); err != nil {
				return err
			}
			return printSimpleResult(actions.CacheRestart,
				fmt.Sprintf("%s restarted.", d), nil)
		},
	}
}

func newCacheStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [driver]",
		Short: "Show cache driver status",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var d cache.Driver
			if len(args) > 0 {
				d = cache.Driver(args[0])
			}

			svc := cache.New(false, globals.Flags.Verbose)
			statuses, err := svc.Status(cmd.Context(), d)
			if err != nil {
				return err
			}

			if globals.Flags.JSON {
				output.PrintJSON(output.Success(actions.CacheStatus, "", statuses))
				return nil
			}

			t := output.NewTable([]string{"DRIVER", "RUNNING", "ENABLED"})
			for _, s := range statuses {
				running := "no"
				if s.Running {
					running = "yes"
				}
				enabled := "no"
				if s.Enabled {
					enabled = "yes"
				}
				t.Append([]string{string(s.Driver), running, enabled})
			}
			t.Render()
			return nil
		},
	}
}

func newCacheConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config <driver>",
		Short: "Show configuration for a cache driver",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d := cache.Driver(args[0])
			svc := cache.New(false, globals.Flags.Verbose)
			cfg, err := svc.Config(cmd.Context(), d)
			if err != nil {
				return err
			}

			if globals.Flags.JSON {
				output.PrintJSON(output.Success(actions.CacheConfig, "", map[string]string{"config": cfg}))
				return nil
			}

			fmt.Println(cfg)
			return nil
		},
	}
}
