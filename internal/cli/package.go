package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"abstrax/internal/actions"
	"abstrax/internal/globals"
	"abstrax/internal/output"
	"abstrax/internal/platform"
	"abstrax/internal/services/pkgmanager"
	"abstrax/internal/validate"
)

// NewPackageCmd returns the package command.
func NewPackageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "package",
		Short: "Manage system packages",
	}

	cmd.AddCommand(newPkgInstallCmd())
	cmd.AddCommand(newPkgRemoveCmd())
	cmd.AddCommand(newPkgUpdateCmd())
	cmd.AddCommand(newPkgUpgradeCmd())
	cmd.AddCommand(newPkgSearchCmd())
	cmd.AddCommand(newPkgInfoCmd())
	cmd.AddCommand(newPkgListCmd())

	return cmd
}

func newPkgInstallCmd() *cobra.Command {
	var version string

	cmd := &cobra.Command{
		Use:   "install <name>",
		Short: "Install a package",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validate.PackageName(name); err != nil {
				return err
			}
			if err := platform.RequireRoot(); err != nil {
				return err
			}

			mgr := pkgmanager.NewApt(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := mgr.Install(cmd.Context(), pkgmanager.InstallOptions{
				Name:    name,
				Version: version,
				DryRun:  globals.Flags.DryRun,
			}); err != nil {
				return err
			}

			return printSimpleResult(actions.PackageInstall,
				fmt.Sprintf("Package %s installed.", name), nil)
		},
	}

	cmd.Flags().StringVar(&version, "version", "", "Package version to install")
	return cmd
}

func newPkgRemoveCmd() *cobra.Command {
	var purge bool

	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a package",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validate.PackageName(name); err != nil {
				return err
			}
			if err := platform.RequireRoot(); err != nil {
				return err
			}

			mgr := pkgmanager.NewApt(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := mgr.Remove(cmd.Context(), pkgmanager.RemoveOptions{
				Name:   name,
				Purge:  purge,
				DryRun: globals.Flags.DryRun,
			}); err != nil {
				return err
			}

			return printSimpleResult(actions.PackageRemove,
				fmt.Sprintf("Package %s removed.", name), nil)
		},
	}

	cmd.Flags().BoolVar(&purge, "purge", false, "Purge package configuration files")
	return cmd
}

func newPkgUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update package lists",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			mgr := pkgmanager.NewApt(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := mgr.Update(cmd.Context()); err != nil {
				return err
			}
			return printSimpleResult(actions.PackageUpdate, "Package lists updated.", nil)
		},
	}
}

func newPkgUpgradeCmd() *cobra.Command {
	var securityOnly bool

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade installed packages",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := platform.RequireRoot(); err != nil {
				return err
			}
			mgr := pkgmanager.NewApt(globals.Flags.DryRun, globals.Flags.Verbose)
			if err := mgr.Upgrade(cmd.Context(), securityOnly); err != nil {
				return err
			}
			return printSimpleResult(actions.PackageUpgrade, "Packages upgraded.", nil)
		},
	}

	cmd.Flags().BoolVar(&securityOnly, "security-only", false, "Apply security updates only")
	return cmd
}

func newPkgSearchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search for packages",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := pkgmanager.NewApt(false, globals.Flags.Verbose)
			pkgs, err := mgr.Search(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			if globals.Flags.JSON {
				output.PrintJSON(output.Success(actions.PackageSearch, "", pkgs))
				return nil
			}

			if len(pkgs) == 0 {
				printer().Line("No packages found.")
				return nil
			}

			t := output.NewTable([]string{"NAME", "DESCRIPTION"})
			for _, p := range pkgs {
				t.Append([]string{p.Name, p.Description})
			}
			t.Render()
			return nil
		},
	}
}

func newPkgInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <name>",
		Short: "Show information about a package",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := pkgmanager.NewApt(false, globals.Flags.Verbose)
			info, err := mgr.Info(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			p := printer()
			if globals.Flags.JSON {
				output.PrintJSON(output.Success(actions.PackageInfo, "", info))
				return nil
			}

			p.Line("")
			p.Line("  %-14s %s", "Name:", info.Name)
			p.Line("  %-14s %s", "Version:", info.Version)
			p.Line("  %-14s %s", "Architecture:", info.Architecture)
			p.Line("  %-14s %s", "Status:", info.Status)
			p.Line("  %-14s %s", "Description:", info.Description)
			p.Line("")
			return nil
		},
	}
}

func newPkgListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed packages",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := pkgmanager.NewApt(false, globals.Flags.Verbose)
			pkgs, err := mgr.List(cmd.Context())
			if err != nil {
				return err
			}

			if globals.Flags.JSON {
				output.PrintJSON(output.Success(actions.PackageList, "", pkgs))
				return nil
			}

			t := output.NewTable([]string{"NAME", "VERSION", "ARCHITECTURE"})
			for _, p := range pkgs {
				t.Append([]string{p.Name, p.Version, p.Architecture})
			}
			t.Render()
			return nil
		},
	}
}
