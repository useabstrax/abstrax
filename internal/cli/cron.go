package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"abstrax/internal/actions"
	"abstrax/internal/globals"
	"abstrax/internal/output"
	"abstrax/internal/platform"
	"abstrax/internal/services/cron"
	"abstrax/internal/validate"
)

// NewCronCmd returns the cron command.
func NewCronCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cron",
		Short: "Manage scheduled cron jobs",
	}

	cmd.AddCommand(newCronAddCmd())
	cmd.AddCommand(newCronRemoveCmd())
	cmd.AddCommand(newCronModifyCmd())
	cmd.AddCommand(newCronListCmd())
	cmd.AddCommand(newCronInfoCmd())
	cmd.AddCommand(newCronEnableCmd())
	cmd.AddCommand(newCronDisableCmd())

	return cmd
}

func newCronAddCmd() *cobra.Command {
	opts := cron.AddOptions{Enabled: true}
	var envPairs []string
	var disabled bool
	var (
		everyMinute         bool
		everyFiveMinutes    bool
		everyTenMinutes     bool
		everyFifteenMinutes bool
		everyThirtyMinutes  bool
		hourly              bool
		daily               bool
		weekly              bool
		monthly             bool
		yearly              bool
	)

	cmd := &cobra.Command{
		Use:   "add <id>",
		Short: "Add a new cron job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.ID = args[0]
			opts.DryRun = globals.Flags.DryRun

			if err := validate.CronID(opts.ID); err != nil {
				return err
			}
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			if opts.Command == "" {
				return fmt.Errorf("--command is required")
			}
			if opts.User == "" {
				opts.User = "root"
			}

			// Build schedule from convenience flags.
			if opts.Schedule == "" {
				opts.Schedule = buildSchedule(everyMinute, everyFiveMinutes,
					everyTenMinutes, everyFifteenMinutes, everyThirtyMinutes,
					hourly, daily, weekly, monthly, yearly)
			}
			if opts.Schedule == "" {
				return fmt.Errorf("--schedule or a frequency flag is required")
			}

			if err := validate.CronExpression(opts.Schedule); err != nil {
				return err
			}

			if disabled {
				opts.Enabled = false
			}

			// Parse env pairs.
			opts.Env = parseEnvPairs(envPairs)

			svc := cron.New()
			job, err := svc.Add(cmd.Context(), opts)
			if err != nil {
				return err
			}

			r := output.Success(actions.CronAdd,
				fmt.Sprintf("Cron job %s created.", opts.ID), job)
			p := printer()
			if globals.Flags.JSON {
				output.PrintJSON(r)
				return nil
			}
			p.Success("Cron job %s created.", opts.ID)
			p.Line("  Schedule: %s", job.Schedule)
			p.Line("  User:     %s", job.User)
			p.Line("  Command:  %s", job.Command)
			p.Line("  File:     %s", job.FilePath)
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.User, "user", "", "User to run the cron job as")
	cmd.Flags().StringVar(&opts.Command, "command", "", "Command to execute")
	cmd.Flags().StringVar(&opts.Schedule, "schedule", "", "Cron expression (5 fields)")
	cmd.Flags().StringVar(&opts.Output, "output", "", "Redirect stdout to this file")
	cmd.Flags().StringVar(&opts.ErrorOutput, "error-output", "", "Redirect stderr to this file")
	cmd.Flags().BoolVar(&opts.AppendOutput, "append-output", false, "Append output instead of overwriting")
	cmd.Flags().BoolVar(&opts.DiscardOutput, "discard-output", false, "Discard all output")
	cmd.Flags().StringVar(&opts.WorkingDir, "working-dir", "", "Working directory")
	cmd.Flags().StringArrayVar(&envPairs, "env", nil, "Environment variables (KEY=VALUE)")
	cmd.Flags().BoolVar(&opts.Enabled, "enabled", true, "Enable the cron job")
	cmd.Flags().BoolVar(&disabled, "disabled", false, "Disable the cron job")

	cmd.Flags().BoolVar(&everyMinute, "every-minute", false, "Run every minute")
	cmd.Flags().BoolVar(&everyFiveMinutes, "every-five-minutes", false, "Run every 5 minutes")
	cmd.Flags().BoolVar(&everyTenMinutes, "every-ten-minutes", false, "Run every 10 minutes")
	cmd.Flags().BoolVar(&everyFifteenMinutes, "every-fifteen-minutes", false, "Run every 15 minutes")
	cmd.Flags().BoolVar(&everyThirtyMinutes, "every-thirty-minutes", false, "Run every 30 minutes")
	cmd.Flags().BoolVar(&hourly, "hourly", false, "Run hourly")
	cmd.Flags().BoolVar(&daily, "daily", false, "Run daily at midnight")
	cmd.Flags().BoolVar(&weekly, "weekly", false, "Run weekly on Sunday")
	cmd.Flags().BoolVar(&monthly, "monthly", false, "Run monthly on the 1st")
	cmd.Flags().BoolVar(&yearly, "yearly", false, "Run yearly on Jan 1st")

	return cmd
}

func newCronRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove a cron job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			if err := validate.CronID(id); err != nil {
				return err
			}
			if err := platform.RequireRoot(); err != nil {
				return err
			}

			svc := cron.New()
			if err := svc.Remove(cmd.Context(), id); err != nil {
				return err
			}
			return printSimpleResult(actions.CronRemove,
				fmt.Sprintf("Cron job %s removed.", id), nil)
		},
	}
}

func newCronModifyCmd() *cobra.Command {
	opts := cron.ModifyOptions{}
	var envPairs []string

	cmd := &cobra.Command{
		Use:   "modify <id>",
		Short: "Modify a cron job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.ID = args[0]
			opts.DryRun = globals.Flags.DryRun

			if err := validate.CronID(opts.ID); err != nil {
				return err
			}
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			if opts.Schedule != "" {
				if err := validate.CronExpression(opts.Schedule); err != nil {
					return err
				}
			}

			opts.Env = parseEnvPairs(envPairs)

			svc := cron.New()
			job, err := svc.Modify(cmd.Context(), opts)
			if err != nil {
				return err
			}

			return printSimpleResult(actions.CronModify,
				fmt.Sprintf("Cron job %s updated.", job.ID), job)
		},
	}

	cmd.Flags().StringVar(&opts.User, "user", "", "User to run the cron job as")
	cmd.Flags().StringVar(&opts.Command, "command", "", "Command to execute")
	cmd.Flags().StringVar(&opts.Schedule, "schedule", "", "Cron expression")
	cmd.Flags().StringVar(&opts.Output, "output", "", "Redirect stdout")
	cmd.Flags().StringVar(&opts.ErrorOutput, "error-output", "", "Redirect stderr")
	cmd.Flags().StringVar(&opts.WorkingDir, "working-dir", "", "Working directory")
	cmd.Flags().StringArrayVar(&envPairs, "env", nil, "Environment variables (KEY=VALUE)")

	return cmd
}

func newCronListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List managed cron jobs",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := cron.New()
			jobs, err := svc.List(cmd.Context())
			if err != nil {
				return err
			}

			if globals.Flags.JSON {
				output.PrintJSON(output.Success(actions.CronList, "", jobs))
				return nil
			}

			if len(jobs) == 0 {
				printer().Line("No managed cron jobs found.")
				return nil
			}

			t := output.NewTable([]string{"ID", "SCHEDULE", "USER", "ENABLED", "COMMAND"})
			for _, j := range jobs {
				enabled := "yes"
				if !j.Enabled {
					enabled = "no"
				}
				cmd := j.Command
				if len(cmd) > 40 {
					cmd = cmd[:37] + "..."
				}
				t.Append([]string{j.ID, j.Schedule, j.User, enabled, cmd})
			}
			t.Render()
			return nil
		},
	}
}

func newCronInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <id>",
		Short: "Show information about a cron job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := cron.New()
			job, err := svc.Info(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			p := printer()
			if globals.Flags.JSON {
				output.PrintJSON(output.Success(actions.CronInfo, "", job))
				return nil
			}

			p.Line("")
			p.Line("  %-10s %s", "ID:", job.ID)
			p.Line("  %-10s %s", "Schedule:", job.Schedule)
			p.Line("  %-10s %s", "User:", job.User)
			p.Line("  %-10s %s", "Command:", job.Command)
			enabled := "yes"
			if !job.Enabled {
				enabled = "no"
			}
			p.Line("  %-10s %s", "Enabled:", enabled)
			p.Line("  %-10s %s", "File:", job.FilePath)
			p.Line("")
			return nil
		},
	}
}

func newCronEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <id>",
		Short: "Enable a cron job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := cron.New()
			if err := svc.Enable(cmd.Context(), args[0]); err != nil {
				return err
			}
			return printSimpleResult(actions.CronEnable,
				fmt.Sprintf("Cron job %s enabled.", args[0]), nil)
		},
	}
}

func newCronDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <id>",
		Short: "Disable a cron job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := cron.New()
			if err := svc.Disable(cmd.Context(), args[0]); err != nil {
				return err
			}
			return printSimpleResult(actions.CronDisable,
				fmt.Sprintf("Cron job %s disabled.", args[0]), nil)
		},
	}
}

func buildSchedule(everyMin, every5, every10, every15, every30, hourly, daily, weekly, monthly, yearly bool) string {
	switch {
	case everyMin:
		return "* * * * *"
	case every5:
		return "*/5 * * * *"
	case every10:
		return "*/10 * * * *"
	case every15:
		return "*/15 * * * *"
	case every30:
		return "*/30 * * * *"
	case hourly:
		return "0 * * * *"
	case daily:
		return "0 0 * * *"
	case weekly:
		return "0 0 * * 0"
	case monthly:
		return "0 0 1 * *"
	case yearly:
		return "0 0 1 1 *"
	default:
		return ""
	}
}

func parseEnvPairs(pairs []string) map[string]string {
	env := make(map[string]string)
	for _, p := range pairs {
		parts := strings.SplitN(p, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}
	return env
}
