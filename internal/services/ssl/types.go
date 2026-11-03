package ssl

// AddOptions holds options for adding SSL to a project.
type AddOptions struct {
	ProjectName  string
	Domains      []string
	Email        string
	Staging      bool
	RedirectHTTP bool
	DryRun       bool

	// CertName is the certbot lineage name. Add fills it in; callers normally
	// leave it empty.
	CertName string
}

// RenewOptions holds options for renewing certificates.
type RenewOptions struct {
	Project string
	DryRun  bool
}

// Certificate status values reported by Status.
const (
	StatusValid    = "valid"
	StatusExpiring = "expiring"
	StatusExpired  = "expired"
	StatusMissing  = "missing"
)

// CertStatus describes the SSL certificate status for a project.
type CertStatus struct {
	// ProjectName is the Abstrax project. It is empty for a certificate no
	// project owns.
	ProjectName string `json:"project_name"`
	// CertificateName is the certbot lineage name.
	CertificateName string   `json:"certificate_name"`
	Domains         []string `json:"domains"`
	// Expiry is the certificate notAfter as an RFC3339 UTC timestamp.
	Expiry        string `json:"expiry,omitempty"`
	DaysRemaining *int   `json:"days_remaining,omitempty"`
	// Status is valid, expiring (under 30 days), expired, or missing.
	Status       string `json:"status"`
	RedirectHTTP bool   `json:"redirect_http"`
	Staging      bool   `json:"staging,omitempty"`
}
