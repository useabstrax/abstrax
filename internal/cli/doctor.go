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

			profile := info.Profile

			type doctorData struct {
				OS              string           `json:"os"`
				Version         string           `json:"version"`
				PrettyName      string           `json:"pretty_name"`
				KernelVersion   string           `json:"kernel_version"`
				Architecture    string           `json:"architecture"`
				PackageManager  string           `json:"package_manager"`
				ServiceManager  string           `json:"service_manager"`
				FirewallBackend string           `json:"firewall_backend"`
				IsRoot          bool             `json:"is_root"`
				Supported       bool             `json:"supported"`
				SupportNote     string           `json:"support_note,omitempty"`
				Profile         platform.Profile `json:"profile"`
				Tools           interface{}      `json:"tools"`
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
				Profile:         profile,
				Tools:           tools,
			}

			r := output.Success(actions.DoctorCheck, "System inspection complete.", data)

			if globals.Flags.JSON {
				output.PrintJSON(r)
				return nil
			}

			p.Line("")
			p.Line("  %-20s %s", "OS:", info.OSPrettyName)
			p.Line("  %-20s %s", "Distro ID:", profile.DistroID)
			p.Line("  %-20s %s", "Version:", info.OSVersion)
			p.Line("  %-20s %s", "Family:", profile.Family)
			p.Line("  %-20s %s", "Kernel:", info.KernelVersion)
			p.Line("  %-20s %s", "Architecture:", info.Architecture)
			p.Line("")
			p.Line("  %-20s %s", "Package manager:", info.PackageManager)
			p.Line("  %-20s %s", "Service manager:", info.ServiceManager)
			p.Line("  %-20s %s", "Firewall strategy:", profile.FirewallStrategy)
			p.Line("  %-20s %s", "Nginx layout:", string(profile.NginxLayout))
			if profile.NginxConfigDir != "" {
				p.Line("  %-20s %s", "Nginx config dir:", profile.NginxConfigDir)
			}
			webIdentity := profile.WebUser
			if profile.WebGroup != "" {
				webIdentity = profile.WebUser + "/" + profile.WebGroup
			}
			p.Line("  %-20s %s", "Web user/group:", webIdentity)
			p.Line("  %-20s %s", "Project root:", profile.DefaultProjectRoot)
			p.Line("  %-20s %s", "PHP-FPM strategy:", profile.PHPFPMStrategy)
			if profile.Family == "rhel" || (profile.SELinuxStatus != "" && profile.SELinuxStatus != platform.SELinuxUnknown) {
				p.Line("  %-20s %s", "SELinux:", profile.SELinuxStatus)
			}
			p.Line("")

			rootStr := "no"
			if info.IsRoot {
				rootStr = "yes"
			}
			p.Line("  %-20s %s", "Running as root:", rootStr)
			p.Line("  %-20s %s", "Support level:", profile.SupportLevel)

			if profile.SupportNote != "" {
				p.Warn(profile.SupportNote)
			}
			if note := platform.SELinuxWarning(profile.SELinuxStatus, ""); note != "" {
				p.Warn(note)
			}

			p.Line("")
			p.Line("  Tools:")
			printTool(p, "nginx", tools.Nginx)
			printTool(p, "apache2/httpd", tools.Apache2)
			printTool(p, "certbot", tools.Certbot)
			printTool(p, "mysql", tools.MySQL)
			printTool(p, "mariadb", tools.MariaDB)
			printTool(p, "supervisor", tools.Supervisor)
			printTool(p, "redis", tools.Redis)
			printTool(p, "memcached", tools.Memcached)
			printTool(p, "ufw", tools.UFW)
			printTool(p, "firewalld", tools.Firewalld)
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
