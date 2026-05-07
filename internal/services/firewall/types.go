package firewall

// Rule describes a firewall rule.
type Rule struct {
	ID       string `json:"id"`
	Action   string `json:"action"`
	Port     string `json:"port,omitempty"`
	Protocol string `json:"protocol,omitempty"`
	From     string `json:"from,omitempty"`
	To       string `json:"to,omitempty"`
	Comment  string `json:"comment,omitempty"`
}

// AllowOptions holds options for allow/deny commands.
type AllowOptions struct {
	Port     string
	Protocol string
	From     string
	To       string
	Comment  string
	DryRun   bool
}

// EnableOptions holds options for enabling the firewall.
type EnableOptions struct {
	AllowSSH bool
	SSHPort  int
	DryRun   bool
}

// Status describes the firewall status.
type Status struct {
	Active  bool   `json:"active"`
	Backend string `json:"backend"`
	Rules   []Rule `json:"rules,omitempty"`
}
