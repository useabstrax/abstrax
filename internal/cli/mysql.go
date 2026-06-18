package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"abstrax/internal/actions"
	"abstrax/internal/confirm"
	"abstrax/internal/globals"
	"abstrax/internal/output"
	"abstrax/internal/platform"
	"abstrax/internal/services/mysql"
	"abstrax/internal/validate"
)

// NewMySQLCmd returns the mysql command.
func NewMySQLCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mysql",
		Short: "Manage MySQL / MariaDB",
	}

	cfgCmd := &cobra.Command{Use: "config", Short: "Manage MySQL connection config"}
	cfgCmd.AddCommand(newMySQLConfigSetCmd())
	cfgCmd.AddCommand(newMySQLConfigShowCmd())

	dbCmd := &cobra.Command{Use: "database", Short: "Manage databases"}
	dbCmd.AddCommand(newMySQLDBAddCmd())
	dbCmd.AddCommand(newMySQLDBRemoveCmd())
	dbCmd.AddCommand(newMySQLDBListCmd())

	userCmd := &cobra.Command{Use: "user", Short: "Manage MySQL users"}
	userCmd.AddCommand(newMySQLUserAddCmd())
	userCmd.AddCommand(newMySQLUserRemoveCmd())
	userCmd.AddCommand(newMySQLUserListCmd())
	userCmd.AddCommand(newMySQLUserInfoCmd())

	cmd.AddCommand(cfgCmd)
	cmd.AddCommand(dbCmd)
	cmd.AddCommand(userCmd)
	cmd.AddCommand(newMySQLTestCmd())
	cmd.AddCommand(newMySQLInstallCmd())
	cmd.AddCommand(newMySQLResetRootPasswordCmd())
	cmd.AddCommand(newMySQLGrantCmd())
	cmd.AddCommand(newMySQLRevokeCmd())

	return cmd
}

func newMySQLConfigSetCmd() *cobra.Command {
	cfg := mysql.Config{Host: "127.0.0.1", Port: 3306, User: "root"}
	var passwordFlag bool

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set MySQL connection configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := platform.RequireRoot(); err != nil {
				return err
			}

			if passwordFlag {
				pw, err := promptPassword("MySQL password: ")
				if err != nil {
					return err
				}
				cfg.Password = pw
			}

			svc := mysql.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.SetConfig(cmd.Context(), cfg); err != nil {
				return err
			}

			return printSimpleResult(actions.MySQLConfigSet, "MySQL config saved.", nil)
		},
	}

	cmd.Flags().StringVar(&cfg.Host, "host", "127.0.0.1", "MySQL host")
	cmd.Flags().IntVar(&cfg.Port, "port", 3306, "MySQL port")
	cmd.Flags().StringVar(&cfg.User, "user", "root", "MySQL user")
	cmd.Flags().BoolVar(&passwordFlag, "password", false, "Prompt for password")
	cmd.Flags().StringVar(&cfg.Socket, "socket", "", "MySQL socket path")

	return cmd
}

func newMySQLConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show MySQL connection configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := mysql.New(false, globals.Flags.Verbose)
			cfg, err := svc.ShowConfig(cmd.Context())
			if err != nil {
				return err
			}

			p := printer()
			if globals.Flags.JSON {
				output.PrintJSON(output.Success(actions.MySQLConfigShow, "", cfg))
				return nil
			}

			p.Line("")
			p.Line("  %-10s %s", "Host:", cfg.Host)
			p.Line("  %-10s %d", "Port:", cfg.Port)
			p.Line("  %-10s %s", "User:", cfg.User)
			p.Line("  %-10s %s", "Socket:", cfg.Socket)
			p.Line("")
			return nil
		},
	}
}

func newMySQLTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test",
		Short: "Test MySQL connection",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := mysql.New(false, globals.Flags.Verbose)
			if err := svc.Test(cmd.Context()); err != nil {
				return err
			}
			return printSimpleResult(actions.MySQLTest, "MySQL connection successful.", nil)
		},
	}
}

func newMySQLInstallCmd() *cobra.Command {
	opts := mysql.InstallOptions{}

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install MySQL / MariaDB",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := platform.RequireRoot(); err != nil {
				return err
			}

			opts.DryRun = globals.Flags.DryRun
			opts.RootPassword = resolveMySQLRootPassword(opts.RootPassword)

			svc := mysql.New(globals.Flags.DryRun, globals.Flags.Verbose)
			result, err := svc.Install(cmd.Context(), opts)
			if err != nil {
				return err
			}

			return printRootPasswordResult(
				actions.MySQLInstall,
				"MySQL installed successfully.",
				result,
				false,
			)
		},
	}

	cmd.Flags().StringVar(&opts.Version, "version", "", "MySQL version to install")
	cmd.Flags().StringVar(&opts.RootPassword, "root-password", "", "Root password (generated if omitted)")

	return cmd
}

func newMySQLResetRootPasswordCmd() *cobra.Command {
	opts := mysql.ResetRootPasswordOptions{}

	cmd := &cobra.Command{
		Use:   "reset-root-password",
		Short: "Reset the MySQL root password",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := platform.RequireRoot(); err != nil {
				return err
			}

			ok, err := confirm.Ask(
				"This will reset the MySQL root password and invalidate the current one. Continue?",
				globals.Flags.Yes,
			)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}

			opts.DryRun = globals.Flags.DryRun
			opts.RootPassword = resolveMySQLRootPassword(opts.RootPassword)

			svc := mysql.New(globals.Flags.DryRun, globals.Flags.Verbose)
			result, err := svc.ResetRootPassword(cmd.Context(), opts)
			if err != nil {
				return err
			}

			staleConfig := svc.HasSavedPassword()
			return printRootPasswordResult(
				actions.MySQLResetRootPassword,
				"MySQL root password reset.",
				result,
				staleConfig,
			)
		},
	}

	cmd.Flags().StringVar(&opts.RootPassword, "root-password", "", "New root password (generated if omitted)")

	return cmd
}

func resolveMySQLRootPassword(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return os.Getenv("ABSTRAX_MYSQL_ROOT_PASSWORD")
}

func printRootPasswordResult(action, summary string, result *mysql.RootPasswordResult, staleConfig bool) error {
	r := output.Success(action, summary, result)

	if globals.Flags.JSON {
		output.PrintJSON(r)
		return nil
	}

	p := printer()
	if !globals.Flags.Quiet {
		p.Line("")
		p.Line("============================================================")
		p.Line("  %s", summary)
		p.Line("")
		p.Line("  ROOT PASSWORD (save this now — shown only once):")
		p.Line("")
		p.Line("  %s", result.RootPassword)
		p.Line("")
		p.Line("  Connect with: mysql -u root -p")
		p.Line("  Abstrax does not store this password. Use `mysql config set")
		p.Line("  --password` separately if you want Abstrax commands to connect.")
		if staleConfig {
			p.Line("")
			p.Warn("Saved MySQL config at /etc/abstrax/mysql.json may be stale. Run `mysql config set --password` to update it.")
		}
		p.Line("============================================================")
		p.Line("")
	} else {
		p.Success(summary)
	}

	return nil
}

func newMySQLDBAddCmd() *cobra.Command {
	opts := mysql.DBAddOptions{}

	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Create a database",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Name = args[0]
			opts.DryRun = globals.Flags.DryRun

			if err := validate.DatabaseName(opts.Name); err != nil {
				return err
			}

			svc := mysql.New(opts.DryRun, globals.Flags.Verbose)
			if err := svc.DBAdd(cmd.Context(), opts); err != nil {
				return err
			}

			return printSimpleResult(actions.MySQLDBAdd,
				fmt.Sprintf("Database %s created.", opts.Name), nil)
		},
	}

	cmd.Flags().StringVar(&opts.Charset, "charset", "utf8mb4", "Character set")
	cmd.Flags().StringVar(&opts.Collation, "collation", "utf8mb4_unicode_ci", "Collation")
	cmd.Flags().BoolVar(&opts.IfNotExists, "if-not-exists", false, "Do not error if database exists")

	return cmd
}

func newMySQLDBRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Drop a database",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if err := validate.DatabaseName(name); err != nil {
				return err
			}

			ok, err := confirm.Ask(
				fmt.Sprintf("Drop database %q? This action is irreversible.", name),
				globals.Flags.Yes,
			)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}

			svc := mysql.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.DBRemove(cmd.Context(), name); err != nil {
				return err
			}

			return printSimpleResult(actions.MySQLDBRemove,
				fmt.Sprintf("Database %s dropped.", name), nil)
		},
	}
}

func newMySQLDBListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List databases",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := mysql.New(false, globals.Flags.Verbose)
			dbs, err := svc.DBList(cmd.Context())
			if err != nil {
				return err
			}

			if globals.Flags.JSON {
				output.PrintJSON(output.Success(actions.MySQLDBList, "", dbs))
				return nil
			}

			t := output.NewTable([]string{"DATABASE"})
			for _, db := range dbs {
				t.Append([]string{db.Name})
			}
			t.Render()
			return nil
		},
	}
}

func newMySQLUserAddCmd() *cobra.Command {
	opts := mysql.UserAddOptions{Host: "localhost"}
	var passwordFlag bool

	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Create a MySQL user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Name = args[0]
			opts.DryRun = globals.Flags.DryRun

			if err := validate.MySQLUsername(opts.Name); err != nil {
				return err
			}

			if passwordFlag {
				pw, err := promptPassword(fmt.Sprintf("Password for %s: ", opts.Name))
				if err != nil {
					return err
				}
				opts.Password = pw
			}

			svc := mysql.New(opts.DryRun, globals.Flags.Verbose)
			if err := svc.UserAdd(cmd.Context(), opts); err != nil {
				return err
			}

			return printSimpleResult(actions.MySQLUserAdd,
				fmt.Sprintf("MySQL user %s created.", opts.Name), nil)
		},
	}

	cmd.Flags().StringVar(&opts.Host, "host", "localhost", "User host")
	cmd.Flags().BoolVar(&passwordFlag, "password", false, "Prompt for password")
	cmd.Flags().StringVar(&opts.GrantDB, "grant-db", "", "Grant access to this database")
	cmd.Flags().StringVar(&opts.Privileges, "privileges", "", "Specific privileges to grant")
	cmd.Flags().StringVar(&opts.Preset, "preset", "app",
		"Privilege preset (readonly|app|admin)")

	return cmd
}

func newMySQLUserRemoveCmd() *cobra.Command {
	var host string

	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Drop a MySQL user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			ok, err := confirm.Ask(
				fmt.Sprintf("Drop MySQL user %q?", name),
				globals.Flags.Yes,
			)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}

			svc := mysql.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.UserRemove(cmd.Context(), name, host); err != nil {
				return err
			}

			return printSimpleResult(actions.MySQLUserRemove,
				fmt.Sprintf("MySQL user %s dropped.", name), nil)
		},
	}
	cmd.Flags().StringVar(&host, "host", "localhost", "User host restriction")
	return cmd
}

func newMySQLUserListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List MySQL users",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := mysql.New(false, globals.Flags.Verbose)
			users, err := svc.UserList(cmd.Context())
			if err != nil {
				return err
			}

			if globals.Flags.JSON {
				output.PrintJSON(output.Success(actions.MySQLUserList, "", users))
				return nil
			}

			t := output.NewTable([]string{"USER", "HOST"})
			for _, u := range users {
				t.Append([]string{u.Name, u.Host})
			}
			t.Render()
			return nil
		},
	}
}

func newMySQLUserInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <name>",
		Short: "Show MySQL user grants",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := mysql.New(false, globals.Flags.Verbose)
			info, err := svc.UserInfo(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			p := printer()
			if globals.Flags.JSON {
				output.PrintJSON(output.Success(actions.MySQLUserInfo, "", info))
				return nil
			}

			p.Line("")
			p.Line("  User: %s", info.Name)
			p.Line("  Grants:")
			for _, g := range info.Grants {
				p.Line("    %s", g)
			}
			p.Line("")
			return nil
		},
	}
}

func newMySQLGrantCmd() *cobra.Command {
	var privileges string
	var preset string

	cmd := &cobra.Command{
		Use:   "grant <user> <database>",
		Short: "Grant database access to a user",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			user, db := args[0], args[1]

			if preset != "" {
				if p, ok := mysql.PresetPrivileges[preset]; ok {
					privileges = p
				} else {
					return fmt.Errorf("unknown privilege preset %q", preset)
				}
			}

			svc := mysql.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.Grant(cmd.Context(), user, db, privileges); err != nil {
				return err
			}

			return printSimpleResult(actions.MySQLGrant,
				fmt.Sprintf("Granted access on %s to %s.", db, user), nil)
		},
	}
	cmd.Flags().StringVar(&privileges, "privileges", "", "Specific privileges to grant")
	cmd.Flags().StringVar(&preset, "preset", "app", "Privilege preset (readonly|app|admin)")
	return cmd
}

func newMySQLRevokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <user> <database>",
		Short: "Revoke database access from a user",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			user, db := args[0], args[1]

			ok, err := confirm.Ask(
				fmt.Sprintf("Revoke all privileges on %q from %q?", db, user),
				globals.Flags.Yes,
			)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}

			svc := mysql.New(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := svc.Revoke(cmd.Context(), user, db); err != nil {
				return err
			}

			return printSimpleResult(actions.MySQLRevoke,
				fmt.Sprintf("Revoked access on %s from %s.", db, user), nil)
		},
	}
}
