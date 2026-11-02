package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"abstrax/internal/actions"
	"abstrax/internal/confirm"
	executil "abstrax/internal/exec"
	"abstrax/internal/globals"
	"abstrax/internal/output"
	"abstrax/internal/platform"
	"abstrax/internal/services/serverinfo"
)

// NewServerCmd returns the server command.
func NewServerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Show server status and reboot the machine",
	}

	cmd.AddCommand(newServerStatusCmd())
	cmd.AddCommand(newServerCPUCmd())
	cmd.AddCommand(newServerMemoryCmd())
	cmd.AddCommand(newServerDiskCmd())
	cmd.AddCommand(newServerLoadCmd())
	cmd.AddCommand(newServerServicesCmd())
	cmd.AddCommand(newServerRebootCmd())
	cmd.AddCommand(newServerRebootRequiredCmd())

	return cmd
}

func newServerStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show comprehensive server status",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := serverinfo.New(globals.Flags.Verbose)
			status, err := svc.Status(cmd.Context())
			if err != nil {
				return err
			}

			p := printer()
			r := output.Success(actions.ServerStatus, "", status)

			if machineOutput() {
				emitResult(r)
				return nil
			}

			p.Line("")
			p.Line("  %-20s %s", "Hostname:", status.Hostname)
			p.Line("  %-20s %s", "Uptime:", status.Uptime)
			p.Line("  %-20s %s (%s, %s)", "Timezone:", status.Timezone.Name, status.Timezone.Abbreviation, status.Timezone.UTCOffset)
			p.Line("  %-20s %.2f  %.2f  %.2f",
				"Load average:", status.LoadAverage[0], status.LoadAverage[1], status.LoadAverage[2])
			p.Line("")

			p.Line("  %-20s %d cores", "CPU:", status.CPU.Cores)
			p.Line("  %-20s %d MB total / %d MB used (%.1f%%)",
				"Memory:",
				status.Memory.TotalMB,
				status.Memory.UsedMB,
				status.Memory.UsagePct,
			)
			if status.Swap.TotalMB > 0 {
				p.Line("  %-20s %d MB total / %d MB used (%.1f%%)",
					"Swap:",
					status.Swap.TotalMB,
					status.Swap.UsedMB,
					status.Swap.UsagePct,
				)
			}
			p.Line("")

			p.Line("  OS:    %s", status.OS.Pretty)
			p.Line("  Kernel: %s", status.KernelVersion)
			p.Line("")

			if len(status.PrivateIPs) > 0 {
				p.Line("  Private IPs:")
				for _, ip := range status.PrivateIPs {
					p.Line("    %s", ip)
				}
				p.Line("")
			}

			if len(status.Disks) > 0 {
				p.Line("  Disk usage:")
				for _, d := range status.Disks {
					p.Line("    %-20s %.1f GB / %.1f GB (%.0f%%)",
						d.MountPoint, d.UsedGB, d.TotalGB, d.UsagePct)
				}
				p.Line("")
			}

			return nil
		},
	}
}

func newServerCPUCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cpu",
		Short: "Show CPU information",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := serverinfo.New(globals.Flags.Verbose)
			cpu := svc.CPU(cmd.Context())

			p := printer()
			if machineOutput() {
				emitResult(output.Success(actions.ServerCPU, "", cpu))
				return nil
			}

			p.Line("  CPU cores: %d", cpu.Cores)
			return nil
		},
	}
}

func newServerMemoryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "memory",
		Short: "Show memory usage",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := serverinfo.New(globals.Flags.Verbose)
			mem := svc.Memory()

			p := printer()
			if machineOutput() {
				emitResult(output.Success(actions.ServerMemory, "", mem))
				return nil
			}

			p.Line("  %-12s %d MB", "Total:", mem.TotalMB)
			p.Line("  %-12s %d MB", "Used:", mem.UsedMB)
			p.Line("  %-12s %d MB", "Free:", mem.FreeMB)
			p.Line("  %-12s %.1f%%", "Usage:", mem.UsagePct)
			return nil
		},
	}
}

func newServerDiskCmd() *cobra.Command {
	var path string

	cmd := &cobra.Command{
		Use:   "disk",
		Short: "Show disk usage",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := serverinfo.New(globals.Flags.Verbose)
			disks := svc.Disk(cmd.Context(), path)

			if machineOutput() {
				emitResult(output.Success(actions.ServerDisk, "", disks))
				return nil
			}

			t := output.NewTable([]string{"MOUNT", "TOTAL", "USED", "FREE", "USE%"})
			for _, d := range disks {
				t.Append([]string{
					d.MountPoint,
					fmt.Sprintf("%.1f GB", d.TotalGB),
					fmt.Sprintf("%.1f GB", d.UsedGB),
					fmt.Sprintf("%.1f GB", d.FreeGB),
					fmt.Sprintf("%.0f%%", d.UsagePct),
				})
			}
			t.Render()
			return nil
		},
	}

	cmd.Flags().StringVar(&path, "path", "", "Show disk usage for specific path")
	return cmd
}

func newServerLoadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "load",
		Short: "Show load average",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := serverinfo.New(globals.Flags.Verbose)
			load := svc.Load()

			p := printer()
			if machineOutput() {
				emitResult(output.Success(actions.ServerLoad, "", load))
				return nil
			}

			p.Line("  Load average: %.2f  %.2f  %.2f", load[0], load[1], load[2])
			return nil
		},
	}
}

func newServerServicesCmd() *cobra.Command {
	var failed bool

	cmd := &cobra.Command{
		Use:   "services",
		Short: "List running or failed services",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := serverinfo.New(globals.Flags.Verbose)
			services, err := svc.Services(cmd.Context(), failed)
			if err != nil {
				return err
			}

			p := printer()
			if machineOutput() {
				emitResult(output.Success(actions.ServerServices, "", services))
				return nil
			}

			if len(services) == 0 {
				p.Line("No services found.")
				return nil
			}

			for _, s := range services {
				p.Line("  %s", s)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&failed, "failed", false, "Show only failed services")
	return cmd
}

func newServerRebootCmd() *cobra.Command {
	var delay int

	cmd := &cobra.Command{
		Use:   "reboot",
		Short: "Reboot the server",
		Long: `Reboot the server. This requires root and asks for confirmation unless --yes is set.

With no delay, or with --delay 0, Abstrax runs "systemctl reboot".
A positive --delay uses "systemd-run --on-active=<seconds>s -- systemctl reboot"
when systemd-run is available. Otherwise the delay is rounded up to whole
minutes and Abstrax runs "shutdown -r +N".

The result is printed before the reboot command runs. --dry-run records the
journal and reboot commands and does not reboot.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireRootAndSupported(); err != nil {
				return err
			}
			if delay < 0 {
				return fmt.Errorf("delay must be zero or greater")
			}

			prompt := "Reboot the server?"
			if delay > 0 {
				prompt = fmt.Sprintf("Reboot the server in %d seconds?", delay)
			}
			ok, err := confirm.Ask(prompt, globals.Flags.Yes)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}

			svc := serverinfo.NewReboot(globals.Flags.DryRun, globals.Flags.Verbose)
			summary := "Server reboot requested."
			if delay > 0 {
				summary = fmt.Sprintf("Server reboot scheduled in %d seconds.", delay)
			}
			_, err = svc.Reboot(cmd.Context(), serverinfo.RebootOptions{DelaySeconds: delay}, func(res *serverinfo.RebootResult) error {
				return printSimpleResult(actions.ServerReboot, summary, res)
			})
			return err
		},
	}

	cmd.Flags().IntVar(&delay, "delay", 0, "Seconds to wait before rebooting (0 reboots immediately)")
	return cmd
}

func newServerRebootRequiredCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reboot-required",
		Short: "Show whether a reboot is required",
		RunE: func(cmd *cobra.Command, args []string) error {
			info, _, err := platform.Detect()
			if err != nil {
				return fmt.Errorf("platform detection failed: %w", err)
			}

			var status serverinfo.RebootStatus
			switch info.Family {
			case "debian":
				status, err = serverinfo.ReadDebianReboot(serverinfo.DefaultRebootFiles())
			case "rhel":
				status, err = serverinfo.ReadRHELReboot(cmd.Context(), executil.New(false, globals.Flags.Verbose))
			default:
				return fmt.Errorf("checking reboot status is not supported on this platform")
			}
			if err != nil {
				return err
			}

			summary := "No reboot required."
			if status.Required {
				summary = "Reboot required."
			}

			if machineOutput() {
				emitResult(output.Success(actions.ServerRebootRequired, summary, status))
				return nil
			}

			p := printer()
			p.Line("%s", summary)
			if status.Note != "" {
				p.Line("%s", status.Note)
			}
			if len(status.Packages) > 0 {
				p.Line("")
				p.Line("Packages:")
				for _, pkg := range status.Packages {
					p.Line("  %s", pkg)
				}
			}
			return nil
		},
	}
}
