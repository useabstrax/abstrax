package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"abstrax/internal/actions"
	"abstrax/internal/globals"
	"abstrax/internal/output"
	"abstrax/internal/platform"
)

// NewDoctorCmd returns the doctor command.
func NewDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Inspect the current system and report platform capabilities",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := printer()

			info, tools, err := platform.Detect()
			if err != nil {
				return fmt.Errorf("platform detection failed: %w", err)
			}

			type doctorData struct {
				OS              string      `json:"os"`
				Version         string      `json:"version"`
				PrettyName      string      `json:"pretty_name"`
				KernelVersion   string      `json:"kernel_version"`
				Architecture    string      `json:"architecture"`
				PackageManager  string      `json:"package_manager"`
				ServiceManager  string      `json:"service_manager"`
				FirewallBackend string      `json:"firewall_backend"`
				IsRoot          bool        `json:"is_root"`
				Supported       bool        `json:"supported"`
				SupportNote     string      `json:"support_note,omitempty"`
				Tools           interface{} `json:"tools"`
			}

			data := doctorData{
				OS:              info.OSName,
				Version:         info.OSVersion,
				PrettyName:      info.OSPrettyName,
				KernelVersion:   info.KernelVersion,
				Architecture:    info.Architecture,
				PackageManager:  info.PackageManager,
				ServiceManager:  info.ServiceManager,
				FirewallBackend: info.FirewallBackend,
				IsRoot:          info.IsRoot,
				Supported:       info.Supported,
				SupportNote:     info.SupportNote,
				Tools:           tools,
			}

			r := output.Success(actions.DoctorCheck, "System inspection complete.", data)

			if globals.Flags.JSON {
				output.PrintJSON(r)
				return nil
			}

			p.Line("")
			p.Line("  %-20s %s", "OS:", info.OSPrettyName)
			p.Line("  %-20s %s", "Version:", info.OSVersion)
			p.Line("  %-20s %s", "Kernel:", info.KernelVersion)
			p.Line("  %-20s %s", "Architecture:", info.Architecture)
			p.Line("")
			p.Line("  %-20s %s", "Package manager:", info.PackageManager)
			p.Line("  %-20s %s", "Service manager:", info.ServiceManager)
			p.Line("  %-20s %s", "Firewall backend:", info.FirewallBackend)
			p.Line("")

			rootStr := "no"
			if info.IsRoot {
				rootStr = "yes"
			}
			p.Line("  %-20s %s", "Running as root:", rootStr)

			if !info.Supported {
				p.Warn(info.SupportNote)
			} else {
				p.Line("  %-20s %s", "Platform support:", "full")
			}

			p.Line("")
			p.Line("  Tools:")
			printTool(p, "nginx", tools.Nginx)
			printTool(p, "apache2", tools.Apache2)
			printTool(p, "certbot", tools.Certbot)
			printTool(p, "mysql", tools.MySQL)
			printTool(p, "mariadb", tools.MariaDB)
			printTool(p, "supervisor", tools.Supervisor)
			printTool(p, "redis", tools.Redis)
			printTool(p, "memcached", tools.Memcached)
			printTool(p, "ufw", tools.UFW)
			printTool(p, "curl", tools.Curl)
			printTool(p, "git", tools.Git)
			p.Line("")

			return nil
		},
	}
}

func printTool(p *output.Printer, name string, available bool) {
	status := "not found"
	if available {
		status = "available"
	}
	p.Line("    %-16s %s", name+":", status)
}
