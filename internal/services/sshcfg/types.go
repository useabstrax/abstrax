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
	CloseOldPort  bool
	DryRun        bool
}

// SetPortResult describes an SSH port change.
type SetPortResult struct {
	Port            int    `json:"port"`
	PreviousPorts   []int  `json:"previous_ports"`
	OldPortKept     bool   `json:"old_port_kept"`
	FirewallChanged bool   `json:"firewall_changed"`
	Message         string `json:"message"`
}

// SetTimeoutOptions holds options for changing SSH idle timeout.
type SetTimeoutOptions struct {
	Seconds int
	DryRun  bool
}

// SSHConfig holds the SSH settings sshd is using.
type SSHConfig struct {
	Port                      string `json:"port"`
	PermitRootLogin           string `json:"permit_root_login"`
	PasswordAuth              string `json:"password_authentication"`
	ClientAliveInterval       string `json:"client_alive_interval"`
	PortSource                string `json:"port_source,omitempty"`
	PermitRootLoginSource     string `json:"permit_root_login_source,omitempty"`
	PasswordAuthSource        string `json:"password_authentication_source,omitempty"`
	ClientAliveIntervalSource string `json:"client_alive_interval_source,omitempty"`
	Note                      string `json:"note,omitempty"`
}

// ReloadOptions holds options for SSH service reload/restart.
type ReloadOptions struct {
	DryRun bool
}
