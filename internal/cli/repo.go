package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"abstrax/internal/actions"
	executil "abstrax/internal/exec"
	"abstrax/internal/globals"
	"abstrax/internal/platform"
	"abstrax/internal/services/pkgmanager"
)

// NewRepoCmd returns the repository management command.
func NewRepoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo",
		Short: "Enable required package repositories (EPEL, CRB, Remi)",
	}
	cmd.AddCommand(newRepoEnableCmd())
	return cmd
}

func newRepoEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <epel|crb|remi>",
		Short: "Enable a required repository for RHEL-family systems",
		Long: `Enable a package repository required by some Abstrax features on RHEL-family systems.

Supported repositories:
  epel  Extra Packages for Enterprise Linux (Certbot and Remi dependency)
  crb   CodeReady Builder / CRB (Remi dependency on EL9)
  remi  Remi repository (multi-version PHP via Software Collections)

On Rocky Linux and AlmaLinux, EPEL/CRB may be enabled without extra flags.
On RHEL and Oracle Linux, pass --enable-required-repos (or --yes) to confirm
third-party repository setup.

Examples:
  sudo abstrax repo enable epel
  sudo abstrax repo enable remi --enable-required-repos`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireRootAndSupported(); err != nil {
				return err
			}

			name := strings.ToLower(strings.TrimSpace(args[0]))
			var repo platform.RepoName
			switch name {
			case "epel":
				repo = platform.RepoEPEL
			case "crb", "codeready", "codeready-builder":
				repo = platform.RepoCRB
			case "remi":
				repo = platform.RepoRemi
			default:
				return fmt.Errorf("unknown repository %q; supported: epel, crb, remi", args[0])
			}

			provider := platform.Resolve()
			if !provider.Profile().IsRHELFamily() {
				return fmt.Errorf("repository enablement is only needed on RHEL-family systems")
			}

			mgr, _, err := pkgmanager.NewFromPlatform(globals.Flags.DryRun, globals.Flags.Verbose)
			if err != nil {
				return err
			}

			opts := platform.RepoOptions{
				EnableRequiredRepos: true,
				Yes:                 globals.Flags.Yes,
				DryRun:              globals.Flags.DryRun,
				Verbose:             globals.Flags.Verbose,
			}

			id := strings.ToLower(provider.Profile().DistroID)
			if id == "rhel" || id == "ol" || id == "oracle" {
				if !globals.Flags.EnableRequiredRepos && !globals.Flags.Yes {
					return fmt.Errorf("enabling %s on %s requires --enable-required-repos (or --yes) to confirm third-party repository setup",
						repo, provider.Profile().DistroName)
				}
			}

			runner := executil.New(globals.Flags.DryRun, globals.Flags.Verbose)
			enabler := platform.DefaultRepoEnabler(
				func(ctx context.Context, pkg string) error {
					return mgr.Install(ctx, pkgmanager.InstallOptions{Name: pkg, DryRun: globals.Flags.DryRun})
				},
				func(ctx context.Context, bin string, a ...string) error {
					_, err := runner.Run(ctx, bin, a...)
					return err
				},
			)

			if err := platform.EnsureRepository(cmd.Context(), provider, repo, opts, enabler); err != nil {
				return err
			}

			return printSimpleResult(actions.RepoEnable,
				fmt.Sprintf("Repository %s enabled.", repo), nil)
		},
	}
}
