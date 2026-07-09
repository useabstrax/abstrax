package platform

import (
	"fmt"
	"strings"
)

// UnsupportedError is returned when a command requires a supported platform.
type UnsupportedError struct {
	Profile Profile
}

func (e *UnsupportedError) Error() string {
	var b strings.Builder
	b.WriteString("Abstrax does not support this operating system.\n\n")

	name := e.Profile.DistroName
	if name == "" {
		name = e.Profile.DistroID
	}
	if name == "" {
		name = "unknown"
	}

	b.WriteString(fmt.Sprintf("Detected: %s", name))
	if e.Profile.DistroID != "" {
		b.WriteString(fmt.Sprintf(" (%s)", e.Profile.DistroID))
	}
	b.WriteString("\n")

	if e.Profile.VersionID != "" {
		b.WriteString(fmt.Sprintf("Version: %s\n", e.Profile.VersionID))
	}
	b.WriteString(fmt.Sprintf("Family: %s\n", e.Profile.Family))
	if e.Profile.PackageManager != "" && e.Profile.PackageManager != "unknown" {
		b.WriteString(fmt.Sprintf("Package manager: %s\n", e.Profile.PackageManager))
	}

	if e.Profile.SupportNote != "" {
		b.WriteString("\n")
		b.WriteString(e.Profile.SupportNote)
		b.WriteString("\n")
	}

	b.WriteString("\nAbstrax currently supports Debian/Ubuntu-based and RHEL-compatible distributions.\n")
	b.WriteString("\nFully supported (Debian/Ubuntu family):\n")
	b.WriteString("  - Ubuntu 20.04+\n")
	b.WriteString("  - Debian 11+\n")
	b.WriteString("  - Linux Mint\n")
	b.WriteString("  - Pop!_OS\n")
	b.WriteString("  - Raspbian / Raspberry Pi OS\n")
	b.WriteString("\nFully supported (RHEL-compatible family):\n")
	b.WriteString("  - Rocky Linux 9+\n")
	b.WriteString("  - AlmaLinux 9+\n")
	b.WriteString("\nExperimental / compatible:\n")
	b.WriteString("  - Red Hat Enterprise Linux 9+\n")
	b.WriteString("  - CentOS Stream 9+\n")
	b.WriteString("  - Oracle Linux 9+\n")
	b.WriteString("\nRun `abstrax doctor` for a full platform report.")

	return b.String()
}
