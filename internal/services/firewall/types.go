package firewall

// RuleKind identifies how a listed rule should be removed on firewalld.
type RuleKind string

const (
	RuleKindService RuleKind = "service"
	RuleKindPort    RuleKind = "port"
	RuleKindRich    RuleKind = "rich"
	RuleKindUFW     RuleKind = "ufw"
)

// Rule describes a firewall rule.
//
// On UFW and firewalld, ID is a stable hash of the rule and Number is its
// current list position. Number can change when other rules are inserted or
// deleted.
type Rule struct {
	ID        string   `json:"id"`
	Number    string   `json:"number,omitempty"`
	Action    string   `json:"action"`
	Direction string   `json:"direction,omitempty"`
	Port      string   `json:"port,omitempty"`
	Protocol  string   `json:"protocol,omitempty"`
	From      string   `json:"from,omitempty"`
	To        string   `json:"to,omitempty"`
	Comment   string   `json:"comment,omitempty"`
	IPv6      bool     `json:"ipv6,omitempty"`
	Kind      RuleKind `json:"kind,omitempty"`
	Target    string   `json:"target,omitempty"`
}

// AllowOptions holds options for allow/deny commands.
type AllowOptions struct {
	Port     string
	Protocol string
	From     string
	To       string
	Comment  string
	DryRun   bool
	// Force skips the SSH lockout check. --yes does not.
	Force bool
}

// GuardOptions controls the SSH lockout check on remove commands.
type GuardOptions struct {
	// Force skips the check. --yes does not.
	Force bool
}

// InstallOptions holds options for installing the platform firewall package.
type InstallOptions struct {
	DryRun bool
}

// InstallResult describes the outcome of installing the firewall package.
type InstallResult struct {
	Backend          string `json:"backend"`
	Package          string `json:"package"`
	AlreadyInstalled bool   `json:"already_installed,omitempty"`
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

// SSHProtectResult describes the outcome of automatic SSH lockout protection.
type SSHProtectResult struct {
	Applied  bool   `json:"applied,omitempty"`
	Added    bool   `json:"added,omitempty"`
	ClientIP string `json:"client_ip,omitempty"`
	SSHPort  int    `json:"ssh_port,omitempty"`
}

// SSHCheck is the lockout check included in --json results for remove and deny.
// Blocked is true when the resulting rules would not allow TCP to Port.
// The command errors instead of returning this when the firewall is active
// and Blocked would be true.
type SSHCheck struct {
	Port    int  `json:"port"`
	Blocked bool `json:"blocked"`
}

// SSHCheckResult is the JSON data for firewall remove commands.
type SSHCheckResult struct {
	SSHCheck *SSHCheck `json:"ssh_check,omitempty"`
}

// RuleChangeResult is the JSON data for firewall deny.
type RuleChangeResult struct {
	SSHProtectResult
	SSHCheck *SSHCheck `json:"ssh_check,omitempty"`
}
