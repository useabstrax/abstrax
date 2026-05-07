package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"abstrax/internal/actions"
	"abstrax/internal/confirm"
	"abstrax/internal/globals"
	"abstrax/internal/output"
	"abstrax/internal/platform"
	"abstrax/internal/services/user"
	"abstrax/internal/validate"
)

// NewUserCmd returns the user command with all subcommands.
func NewUserCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage Linux users",
	}

	cmd.AddCommand(newUserAddCmd())
	cmd.AddCommand(newUserRemoveCmd())
	cmd.AddCommand(newUserGrantSudoCmd())
	cmd.AddCommand(newUserRevokeSudoCmd())
	cmd.AddCommand(newUserSetGroupsCmd())
	cmd.AddCommand(newUserAddGroupsCmd())
	cmd.AddCommand(newUserRemoveGroupsCmd())
	cmd.AddCommand(newUserSetShellCmd())
	cmd.AddCommand(newUserLockCmd())
	cmd.AddCommand(newUserUnlockCmd())
	cmd.AddCommand(newUserInfoCmd())
	cmd.AddCommand(newUserListCmd())

	return cmd
}

func newUserAddCmd() *cobra.Command {
	opts := user.AddOptions{
		CreateHome: true,
	}
	var groupsStr string
	var password bool

	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Create a new user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Username = args[0]
			opts.DryRun = globals.Flags.DryRun

			if err := validate.Username(opts.Username); err != nil {
				return err
			}
			if err := platform.RequireRoot(); err != nil {
				return err
			}

			if groupsStr != "" {
				opts.Groups = strings.Split(groupsStr, ",")
			}
			if opts.Shell != "" {
				if err := validate.Shell(opts.Shell); err != nil {
					return err
				}
			}
			if password {
				pw, err := promptPassword("Enter password for " + opts.Username + ": ")
				if err != nil {
					return err
				}
				opts.Password = pw
			}

			svc := user.New(opts.DryRun, globals.Flags.Verbose)
			result, err := svc.Add(cmd.Context(), opts)
			if err != nil {
				return err
			}

			p := printer()
			r := output.Success(actions.UserAdd, fmt.Sprintf("User %s created.", opts.Username), result)

			if result.AlreadyExisted {
				r.Summary = fmt.Sprintf("User %s already exists.", opts.Username)
			}

			if globals.Flags.JSON {
				output.PrintJSON(r)
				return nil
			}

			if result.AlreadyExisted {
				p.Warn("User %s already exists.", opts.Username)
			} else {
				p.Success("User %s created.", opts.Username)
			}
			p.Line("  Home:   %s", result.Home)
			p.Line("  Shell:  %s", result.Shell)
			p.Line("  UID:    %s", result.UID)
			p.Line("  Groups: %s", strings.Join(result.Groups, ", "))
			if result.Sudo {
				p.Line("  Sudo:   granted")
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&opts.CreateHome, "create-home", true, "Create home directory")
	cmd.Flags().BoolVar(&opts.NoCreateHome, "no-create-home", false, "Do not create home directory")
	cmd.Flags().BoolVar(&opts.GrantSudo, "grant-sudo", false, "Add user to sudo group")
	cmd.Flags().StringVar(&groupsStr, "groups", "", "Additional groups (comma-separated)")
	cmd.Flags().StringVar(&opts.Shell, "shell", "", "Login shell")
	cmd.Flags().StringVar(&opts.UID, "uid", "", "Custom UID")
	cmd.Flags().BoolVar(&opts.System, "system", false, "Create a system user")
	cmd.Flags().BoolVar(&password, "password", false, "Prompt for password")
	cmd.Flags().BoolVar(&opts.DisabledPassword, "disabled-password", false, "Create user without a password")
	cmd.Flags().StringVar(&opts.Comment, "comment", "", "GECOS comment field")

	return cmd
}

func newUserRemoveCmd() *cobra.Command {
	opts := user.RemoveOptions{}

	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Username = args[0]
			opts.DryRun = globals.Flags.DryRun

			if err := validate.Username(opts.Username); err != nil {
				return err
			}
			if err := platform.RequireRoot(); err != nil {
				return err
			}

			ok, err := confirm.Ask(
				fmt.Sprintf("Remove user %q?", opts.Username),
				globals.Flags.Yes,
			)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}

			svc := user.New(opts.DryRun, globals.Flags.Verbose)
			if err := svc.Remove(cmd.Context(), opts); err != nil {
				return err
			}

			p := printer()
			r := output.Success(actions.UserRemove, fmt.Sprintf("User %s removed.", opts.Username), nil)
			if globals.Flags.JSON {
				output.PrintJSON(r)
				return nil
			}
			p.Success("User %s removed.", opts.Username)
			return nil
		},
	}

	cmd.Flags().BoolVar(&opts.DeleteHome, "delete-home", false, "Delete home directory")
	cmd.Flags().BoolVar(&opts.KeepHome, "keep-home", false, "Keep home directory")
	cmd.Flags().BoolVar(&opts.RemoveCron, "remove-cron", false, "Remove user's crontab")
	cmd.Flags().BoolVar(&opts.KillProcesses, "kill-processes", false, "Kill user's processes before removal")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "Force removal")

	return cmd
}

func newUserGrantSudoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "grant-sudo <name>",
		Short: "Grant sudo privileges to a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			username := args[0]
			if err := validate.Username(username); err != nil {
				return err
			}
			if err := platform.RequireRoot(); err != nil {
				return err
			}

			svc := user.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.GrantSudo(cmd.Context(), username, globals.Flags.DryRun); err != nil {
				return err
			}

			return printSimpleResult(actions.UserGrantSudo,
				fmt.Sprintf("Sudo granted to %s.", username), nil)
		},
	}
}

func newUserRevokeSudoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke-sudo <name>",
		Short: "Revoke sudo privileges from a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			username := args[0]
			if err := validate.Username(username); err != nil {
				return err
			}
			if err := platform.RequireRoot(); err != nil {
				return err
			}

			ok, err := confirm.Ask(
				fmt.Sprintf("Revoke sudo from %q?", username),
				globals.Flags.Yes,
			)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}

			svc := user.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.RevokeSudo(cmd.Context(), username, globals.Flags.DryRun); err != nil {
				return err
			}

			return printSimpleResult(actions.UserRevokeSudo,
				fmt.Sprintf("Sudo revoked from %s.", username), nil)
		},
	}
}

func newUserSetGroupsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-groups <name> <groups>",
		Short: "Set a user's supplementary groups",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			username := args[0]
			groups := strings.Split(args[1], ",")
			if err := validate.Username(username); err != nil {
				return err
			}
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			svc := user.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.SetGroups(cmd.Context(), user.ModifyGroupsOptions{
				Username: username,
				Groups:   groups,
				DryRun:   globals.Flags.DryRun,
			}); err != nil {
				return err
			}
			return printSimpleResult(actions.UserSetGroups,
				fmt.Sprintf("Groups updated for %s.", username), nil)
		},
	}
}

func newUserAddGroupsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add-groups <name> <groups>",
		Short: "Add groups to a user",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			username := args[0]
			groups := strings.Split(args[1], ",")
			if err := validate.Username(username); err != nil {
				return err
			}
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			svc := user.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.AddGroups(cmd.Context(), user.ModifyGroupsOptions{
				Username: username,
				Groups:   groups,
				DryRun:   globals.Flags.DryRun,
			}); err != nil {
				return err
			}
			return printSimpleResult(actions.UserAddGroups,
				fmt.Sprintf("Groups added to %s.", username), nil)
		},
	}
}

func newUserRemoveGroupsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove-groups <name> <groups>",
		Short: "Remove groups from a user",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			username := args[0]
			groups := strings.Split(args[1], ",")
			if err := validate.Username(username); err != nil {
				return err
			}
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			svc := user.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.RemoveGroups(cmd.Context(), user.ModifyGroupsOptions{
				Username: username,
				Groups:   groups,
				DryRun:   globals.Flags.DryRun,
			}); err != nil {
				return err
			}
			return printSimpleResult(actions.UserRemoveGroups,
				fmt.Sprintf("Groups removed from %s.", username), nil)
		},
	}
}

func newUserSetShellCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-shell <name> <shell>",
		Short: "Set a user's login shell",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			username, shell := args[0], args[1]
			if err := validate.Username(username); err != nil {
				return err
			}
			if err := validate.Shell(shell); err != nil {
				return err
			}
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			svc := user.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.SetShell(cmd.Context(), user.SetShellOptions{
				Username: username,
				Shell:    shell,
				DryRun:   globals.Flags.DryRun,
			}); err != nil {
				return err
			}
			return printSimpleResult(actions.UserSetShell,
				fmt.Sprintf("Shell for %s set to %s.", username, shell), nil)
		},
	}
}

func newUserLockCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "lock <name>",
		Short: "Lock a user account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			username := args[0]
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			svc := user.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.Lock(cmd.Context(), user.LockOptions{
				Username: username,
				DryRun:   globals.Flags.DryRun,
			}); err != nil {
				return err
			}
			return printSimpleResult(actions.UserLock,
				fmt.Sprintf("User %s locked.", username), nil)
		},
	}
}

func newUserUnlockCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unlock <name>",
		Short: "Unlock a user account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			username := args[0]
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			svc := user.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.Unlock(cmd.Context(), user.LockOptions{
				Username: username,
				DryRun:   globals.Flags.DryRun,
			}); err != nil {
				return err
			}
			return printSimpleResult(actions.UserUnlock,
				fmt.Sprintf("User %s unlocked.", username), nil)
		},
	}
}

func newUserInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <name>",
		Short: "Show information about a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			username := args[0]
			svc := user.New(false, globals.Flags.Verbose)
			info, err := svc.Info(cmd.Context(), username)
			if err != nil {
				return err
			}

			p := printer()
			r := output.Success(actions.UserInfo, "", info)

			if globals.Flags.JSON {
				output.PrintJSON(r)
				return nil
			}

			p.Line("")
			p.Line("  %-12s %s", "Username:", info.Username)
			p.Line("  %-12s %s", "UID:", info.UID)
			p.Line("  %-12s %s", "GID:", info.GID)
			p.Line("  %-12s %s", "Home:", info.Home)
			p.Line("  %-12s %s", "Shell:", info.Shell)
			p.Line("  %-12s %s", "Groups:", strings.Join(info.Groups, ", "))
			if info.Comment != "" {
				p.Line("  %-12s %s", "Comment:", info.Comment)
			}

			sudoStr := "no"
			if info.IsSudo {
				sudoStr = "yes"
			}
			p.Line("  %-12s %s", "Sudo:", sudoStr)

			lockedStr := "no"
			if info.Locked {
				lockedStr = "yes"
			}
			p.Line("  %-12s %s", "Locked:", lockedStr)
			p.Line("")

			return nil
		},
	}
}

func newUserListCmd() *cobra.Command {
	opts := user.ListOptions{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List users",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := user.New(false, globals.Flags.Verbose)
			users, err := svc.List(cmd.Context(), opts)
			if err != nil {
				return err
			}

			if globals.Flags.JSON {
				r := output.Success(actions.UserList, "", users)
				output.PrintJSON(r)
				return nil
			}

			if len(users) == 0 {
				printer().Line("No users found.")
				return nil
			}

			t := output.NewTable([]string{"USERNAME", "UID", "HOME", "SHELL", "SUDO"})
			for _, u := range users {
				sudo := ""
				if u.IsSudo {
					sudo = "yes"
				}
				t.Append([]string{u.Username, u.UID, u.Home, u.Shell, sudo})
			}
			t.Render()
			return nil
		},
	}

	cmd.Flags().BoolVar(&opts.System, "system", false, "Show system users only")
	cmd.Flags().BoolVar(&opts.Regular, "regular", false, "Show regular users only")
	cmd.Flags().BoolVar(&opts.Sudo, "sudo", false, "Show sudo users only")

	return cmd
}

// printSimpleResult is a convenience helper for commands that return a simple
// success or JSON result.
func printSimpleResult(action, summary string, data interface{}) error {
	p := printer()
	r := output.Success(action, summary, data)
	if globals.Flags.JSON {
		output.PrintJSON(r)
		return nil
	}
	p.Success(summary)
	return nil
}

// promptPassword reads a password securely from the terminal.
func promptPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	pw, err := term.ReadPassword(0)
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("reading password: %w", err)
	}
	return string(pw), nil
}
