package ssl

// AddOptions holds options for adding SSL to a project.
type AddOptions struct {
	ProjectName  string
	Domains      []string
	Email        string
	Staging      bool
	RedirectHTTP bool
	DryRun       bool
}

// RenewOptions holds options for renewing certificates.
type RenewOptions struct {
	Project string
	DryRun  bool
}

// CertStatus describes the SSL certificate status for a project.
type CertStatus struct {
	ProjectName string   `json:"project_name"`
	Domains     []string `json:"domains"`
	Expiry      string   `json:"expiry,omitempty"`
	Status      string   `json:"status"`
}
