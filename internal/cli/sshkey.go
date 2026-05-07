package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"abstrax/internal/actions"
	"abstrax/internal/globals"
	"abstrax/internal/output"
	"abstrax/internal/services/sshkey"
	"abstrax/internal/validate"
)

// NewSSHKeyCmd returns the ssh-key command.
func NewSSHKeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ssh-key",
		Short: "Manage SSH authorized keys for users",
	}

	cmd.AddCommand(newSSHKeyAddCmd())
	cmd.AddCommand(newSSHKeyRemoveCmd())
	cmd.AddCommand(newSSHKeyListCmd())
	cmd.AddCommand(newSSHKeyInfoCmd())

	return cmd
}

func newSSHKeyAddCmd() *cobra.Command {
	opts := sshkey.AddOptions{}

	cmd := &cobra.Command{
		Use:   "add <user> <key>",
		Short: "Add an SSH public key for a user",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Username = args[0]
			opts.Key = args[1]
			opts.DryRun = globals.Flags.DryRun

			if err := validate.Username(opts.Username); err != nil {
				return err
			}

			svc := sshkey.New(opts.DryRun, globals.Flags.Verbose)
			info, err := svc.Add(cmd.Context(), opts)
			if err != nil {
				return err
			}

			p := printer()
			r := output.Success(actions.SSHKeyAdd,
				fmt.Sprintf("SSH key %s added for %s.", info.ID, opts.Username), info)

			if globals.Flags.JSON {
				output.PrintJSON(r)
				return nil
			}

			p.Success("SSH key added for %s.", opts.Username)
			p.Line("  ID:          %s", info.ID)
			p.Line("  Name:        %s", info.Name)
			p.Line("  Fingerprint: %s", info.Fingerprint)
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.Name, "name", "", "Key name / ID")
	cmd.Flags().StringVar(&opts.Comment, "comment", "", "Comment stored in the managed marker")
	cmd.Flags().BoolVar(&opts.FromFile, "from-file", false, "Treat <key> argument as a file path")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "Overwrite if key already exists")

	return cmd
}

func newSSHKeyRemoveCmd() *cobra.Command {
	opts := sshkey.RemoveOptions{}

	cmd := &cobra.Command{
		Use:   "remove <user> <key-id>",
		Short: "Remove a managed SSH key for a user",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Username = args[0]
			opts.KeyID = args[1]
			opts.DryRun = globals.Flags.DryRun

			if err := validate.Username(opts.Username); err != nil {
				return err
			}

			svc := sshkey.New(opts.DryRun, globals.Flags.Verbose)
			if err := svc.Remove(cmd.Context(), opts); err != nil {
				return err
			}

			return printSimpleResult(actions.SSHKeyRemove,
				fmt.Sprintf("SSH key %s removed for %s.", opts.KeyID, opts.Username), nil)
		},
	}

	cmd.Flags().StringVar(&opts.Fingerprint, "fingerprint", "", "Match key by fingerprint")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "Remove even unmanaged keys")

	return cmd
}

func newSSHKeyListCmd() *cobra.Command {
	opts := sshkey.ListOptions{}

	cmd := &cobra.Command{
		Use:   "list <user>",
		Short: "List SSH keys for a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Username = args[0]

			if err := validate.Username(opts.Username); err != nil {
				return err
			}

			svc := sshkey.New(false, globals.Flags.Verbose)
			keys, err := svc.List(cmd.Context(), opts)
			if err != nil {
				return err
			}

			if globals.Flags.JSON {
				output.PrintJSON(output.Success(actions.SSHKeyList, "", keys))
				return nil
			}

			if len(keys) == 0 {
				printer().Line("No SSH keys found for %s.", opts.Username)
				return nil
			}

			t := output.NewTable([]string{"ID", "TYPE", "FINGERPRINT", "MANAGED"})
			for _, k := range keys {
				managed := ""
				if k.Managed {
					managed = "yes"
				}
				t.Append([]string{k.ID, k.Type, k.Fingerprint, managed})
			}
			t.Render()
			return nil
		},
	}

	cmd.Flags().BoolVar(&opts.ManagedOnly, "managed-only", false, "Show only Abstrax-managed keys")

	return cmd
}

func newSSHKeyInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <user> <key-id>",
		Short: "Show information about a specific SSH key",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			username, keyID := args[0], args[1]

			svc := sshkey.New(false, globals.Flags.Verbose)
			info, err := svc.Info(cmd.Context(), username, keyID)
			if err != nil {
				return err
			}

			p := printer()
			r := output.Success(actions.SSHKeyInfo, "", info)

			if globals.Flags.JSON {
				output.PrintJSON(r)
				return nil
			}

			p.Line("")
			p.Line("  %-14s %s", "ID:", info.ID)
			p.Line("  %-14s %s", "Name:", info.Name)
			p.Line("  %-14s %s", "Type:", info.Type)
			p.Line("  %-14s %s", "Fingerprint:", info.Fingerprint)
			p.Line("  %-14s %s", "Comment:", info.Comment)
			managed := "no"
			if info.Managed {
				managed = "yes"
			}
			p.Line("  %-14s %s", "Managed:", managed)
			p.Line("")
			return nil
		},
	}
}
