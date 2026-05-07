package sshcfg

// ConfigEntry represents a single sshd_config directive.
type ConfigEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// SetPortOptions holds options for changing the SSH port.
type SetPortOptions struct {
	Port          int
	AllowFirewall bool
	DryRun        bool
}

// SetTimeoutOptions holds options for changing SSH idle timeout.
type SetTimeoutOptions struct {
	Seconds int
	DryRun  bool
}

// SSHConfig holds the current SSH configuration values managed by Abstrax.
type SSHConfig struct {
	Port                string `json:"port"`
	PermitRootLogin     string `json:"permit_root_login"`
	PasswordAuth        string `json:"password_authentication"`
	ClientAliveInterval string `json:"client_alive_interval"`
}

// ReloadOptions holds options for SSH service reload/restart.
type ReloadOptions struct {
	DryRun bool
}
