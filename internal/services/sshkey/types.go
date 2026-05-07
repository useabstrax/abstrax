package sshkey

// AddOptions holds options for adding an SSH key.
type AddOptions struct {
	Username string
	Key      string
	Name     string
	Comment  string
	FromFile bool
	Force    bool
	DryRun   bool
}

// RemoveOptions holds options for removing an SSH key.
type RemoveOptions struct {
	Username    string
	KeyID       string
	Fingerprint string
	Force       bool
	DryRun      bool
}

// ListOptions holds filters for listing SSH keys.
type ListOptions struct {
	Username    string
	ManagedOnly bool
}

// KeyInfo describes a single authorized key.
type KeyInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Comment     string `json:"comment"`
	Fingerprint string `json:"fingerprint"`
	Managed     bool   `json:"managed"`
	Line        int    `json:"line"`
}
