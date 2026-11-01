package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"abstrax/internal/actions"
	"abstrax/internal/confirm"
	"abstrax/internal/globals"
	"abstrax/internal/output"
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
	cmd.AddCommand(newUserSessionsCmd())
	cmd.AddCommand(newUserLogoutCmd())
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
			if err := requireRootAndSupported(); err != nil {
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

			if machineOutput() {
				emitResult(r)
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
	cmd.Flags().BoolVar(&opts.GrantSudo, "grant-sudo", false, "Grant passwordless sudo (sudo group + sudoers drop-in)")
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
			if err := requireRootAndSupported(); err != nil {
				return err
			}

			if !skipConfirm(opts.Force) {
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
			}

			svc := user.New(opts.DryRun, globals.Flags.Verbose)
			if err := svc.Remove(cmd.Context(), opts); err != nil {
				return err
			}

			p := printer()
			r := output.Success(actions.UserRemove, fmt.Sprintf("User %s removed.", opts.Username), nil)
			if machineOutput() {
				emitResult(r)
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
		Short: "Grant passwordless sudo privileges to a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			username := args[0]
			if err := validate.Username(username); err != nil {
				return err
			}
			if err := requireRootAndSupported(); err != nil {
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
			if err := requireRootAndSupported(); err != nil {
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
			if err := requireRootAndSupported(); err != nil {
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
			if err := requireRootAndSupported(); err != nil {
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
			if err := requireRootAndSupported(); err != nil {
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
			if err := requireRootAndSupported(); err != nil {
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
	var passwordOnly bool
	cmd := &cobra.Command{
		Use:   "lock <name>",
		Short: "Lock a user account",
		Long: `Lock a user account so password and SSH key logins fail.

Processes already running as that user, including daemons and cron jobs, keep running.
--password-only locks the password and leaves key logins working.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			username := args[0]
			if err := requireRootAndSupported(); err != nil {
				return err
			}
			svc := user.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.Lock(cmd.Context(), user.LockOptions{
				Username:     username,
				PasswordOnly: passwordOnly,
				DryRun:       globals.Flags.DryRun,
			}); err != nil {
				return err
			}
			return printSimpleResult(actions.UserLock,
				fmt.Sprintf("User %s locked.", username), nil)
		},
	}
	cmd.Flags().BoolVar(&passwordOnly, "password-only", false, "Lock the password only; SSH keys can still log in")
	return cmd
}

func newUserUnlockCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unlock <name>",
		Short: "Unlock a user account",
		Long:  "Unlock a user account and restore the account expiry from before it was locked.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			username := args[0]
			if err := requireRootAndSupported(); err != nil {
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

			if machineOutput() {
				emitResult(r)
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
			expiredStr := "no"
			if info.Expired {
				expiredStr = "yes"
			}
			p.Line("  %-12s %s", "Expired:", expiredStr)
			blockedStr := "no"
			if info.LoginBlocked {
				blockedStr = "yes"
			}
			p.Line("  %-12s %s", "Login blocked:", blockedStr)
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

			if machineOutput() {
				r := output.Success(actions.UserList, "", users)
				emitResult(r)
				return nil
			}

			if len(users) == 0 {
				printer().Line("No users found.")
				return nil
			}

			t := output.NewTable([]string{"USERNAME", "UID", "HOME", "SHELL", "SUDO", "LOCKED", "EXPIRED", "BLOCKED"})
			for _, u := range users {
				sudo := ""
				if u.IsSudo {
					sudo = "yes"
				}
				t.Append([]string{u.Username, u.UID, u.Home, u.Shell, sudo, yesNo(u.Locked), yesNo(u.Expired), yesNo(u.LoginBlocked)})
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

// askConfirm is replaced in tests. Production prompts unless --yes is set.
var askConfirm = confirm.Ask

// ensureRoot is replaced in tests. Logout requires root; sessions does not.
var ensureRoot = requireRootAndSupported

// executeUserLogout runs logout after the CLI has confirmed.
// Tests replace it so a declined confirmation records no terminate command.
var executeUserLogout = func(ctx context.Context, opts user.LogoutOptions) (*user.LogoutResult, error) {
	svc := user.New(opts.DryRun, globals.Flags.Verbose)
	return svc.Logout(ctx, opts)
}

func newUserSessionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sessions <name>",
		Short: "List a user's interactive login sessions",
		Long: `List interactive login sessions for a user.

Each session includes the session id, TTY, source IP, login time, and idle time. Idle is empty when it is not reported. Daemons and cron jobs are not listed.

Reads loginctl list-sessions when it returns JSON, and who -u otherwise. If the user has no login sessions, the command succeeds and prints an empty list.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			username := args[0]
			if err := validate.Username(username); err != nil {
				return err
			}

			svc := user.New(false, globals.Flags.Verbose)
			sessions, err := svc.Sessions(cmd.Context(), username)
			if err != nil {
				return err
			}

			if machineOutput() {
				emitResult(output.Success(actions.UserSessions, "", sessions))
				return nil
			}
			if len(sessions) == 0 {
				printer().Line("No sessions.")
				return nil
			}

			t := output.NewTable([]string{"SESSION", "TTY", "SOURCE", "LOGIN", "IDLE"})
			for _, sess := range sessions {
				t.Append([]string{sess.ID, sess.TTY, sess.SourceIP, sess.LoginTime, idleString(sess.Idle)})
			}
			t.Render()
			return nil
		},
	}
}

func newUserLogoutCmd() *cobra.Command {
	var sessionID string
	var all bool

	cmd := &cobra.Command{
		Use:   "logout <name>",
		Short: "End a user's interactive login sessions",
		Long: `End one interactive login session, or every login session for a user.

Only interactive login sessions are ended. Daemons and cron jobs running as that user keep running. This does not kill every process the user owns, and it does not run loginctl terminate-user.

With logind, each selected session is ended with loginctl terminate-session. Without logind, Abstrax sends SIGHUP and then SIGKILL only to that session's sshd process (sshd: <user>@pts/N).

Pass exactly one of --session or --all. The command asks for confirmation unless --yes is set.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			username := args[0]
			if err := validate.Username(username); err != nil {
				return err
			}
			if sessionID != "" {
				if err := user.ValidateSessionID(sessionID); err != nil {
					return err
				}
			}
			if err := ensureRoot(); err != nil {
				return err
			}

			ok, err := askConfirm(logoutPrompt(username, sessionID, all), globals.Flags.Yes)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}

			result, err := executeUserLogout(cmd.Context(), user.LogoutOptions{
				Username:  username,
				SessionID: sessionID,
				All:       all,
				DryRun:    globals.Flags.DryRun,
			})
			if err != nil {
				return err
			}

			summary := logoutSummary(username, result.Ended)
			r := output.Success(actions.UserLogout, summary, result)
			if machineOutput() {
				emitResult(r)
				return nil
			}
			if len(result.Ended) == 0 {
				printer().Line(summary)
				return nil
			}
			p := printer()
			p.Success(summary)
			for _, sess := range result.Ended {
				p.Line("  %s  %s  %s", sess.ID, sess.TTY, sess.SourceIP)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&sessionID, "session", "", "Session id to end")
	cmd.Flags().BoolVar(&all, "all", false, "End every interactive login session for this user. Does not stop daemons or cron jobs")
	cmd.MarkFlagsOneRequired("session", "all")
	cmd.MarkFlagsMutuallyExclusive("session", "all")
	return cmd
}

func logoutPrompt(username, sessionID string, all bool) string {
	if all {
		return fmt.Sprintf("End all interactive login sessions for %q? Daemons and cron jobs will keep running.", username)
	}
	return fmt.Sprintf("End login session %s for %q? Daemons and cron jobs will keep running.", sessionID, username)
}

func logoutSummary(username string, ended []user.Session) string {
	switch len(ended) {
	case 0:
		return fmt.Sprintf("No login sessions to end for %s.", username)
	case 1:
		return fmt.Sprintf("Ended 1 login session for %s.", username)
	default:
		return fmt.Sprintf("Ended %d login sessions for %s.", len(ended), username)
	}
}

func idleString(idle *string) string {
	if idle == nil {
		return ""
	}
	return *idle
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return ""
}

// printSimpleResult is a convenience helper for commands that return a simple
// success or JSON result.
func printSimpleResult(action, summary string, data interface{}) error {
	p := printer()
	r := output.Success(action, summary, data)
	if machineOutput() {
		emitResult(r)
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
