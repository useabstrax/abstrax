package platform

// Resolve returns the provider for the current host, falling back to Debian
// defaults when detection fails (for example in unit tests on non-Linux hosts).
func Resolve() Provider {
	provider, info, err := Current()
	if err == nil && provider != nil {
		return provider
	}
	if info != nil && info.Family == "rhel" && info.Profile.Supported() {
		return newRHELProvider(info.Profile)
	}
	return DebianDefaults()
}

// ResolveOrError returns the provider for the current host, or an error when
// the platform is unsupported.
func ResolveOrError() (Provider, *Info, error) {
	return Current()
}

// SELinuxWarning returns a user-facing warning when SELinux is enforcing.
// Abstrax never disables SELinux automatically.
func SELinuxWarning(status SELinuxStatus, context string) string {
	if status != SELinuxEnforcing {
		return ""
	}
	msg := "SELinux is enforcing"
	if context != "" {
		msg += " (" + context + ")"
	}
	msg += "; additional file context rules may be required. Abstrax will not disable SELinux automatically."
	return msg
}
