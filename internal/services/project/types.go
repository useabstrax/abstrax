package project

import (
	"encoding/json"
	"time"
)

// WebServer identifies the web server backend.
type WebServer string

const (
	WebServerNginx  WebServer = "nginx"
	WebServerApache WebServer = "apache"
	WebServerNone   WebServer = "none"
)

// Runtime identifies the application runtime.
type Runtime string

const (
	RuntimePHP    Runtime = "php"
	RuntimeNode   Runtime = "node"
	RuntimeRuby   Runtime = "ruby"
	RuntimeStatic Runtime = "static"
)

// AddOptions holds options for creating a project.
type AddOptions struct {
	Name         string
	Path         string
	UserExplicit bool
	WebServer    WebServer
	NoVhost      bool
	SharedLogs   bool
	Domains      []string
	Port         int
	WebRoot      string
	Runtime      Runtime
	PHPVersion   string
	PHPFpm       bool
	PublicDir    string
	NodePort     int
	NodeVersion  string
	ProxyPort    int
	StartCommand string
	RubyVersion  string
	SSL          bool
	Email        string
	Staging      bool
	RedirectHTTP bool
	User         string
	Group        string
	Chown        bool
	Chmod        string
	Yes          bool
	DryRun       bool
	Progress     func(step, message string)
}

// RemoveOptions holds options for removing a project.
type RemoveOptions struct {
	Name          string
	RemoveVhost   bool
	RemoveSSL     bool
	DeleteFiles   bool
	KeepFiles     bool
	RemoveDaemons bool
	RemoveCron    bool
	Force         bool
	DryRun        bool
}

// ModifyOptions holds options for modifying a project.
//
// Changed records which flags were set explicitly (keys: domains, add-domain,
// remove-domain, path, php-version, node-version, ruby-version, public-dir,
// proxy-port, shared-logs). A changed flag with an empty value clears the
// stored field: empty domains, public-dir, or a version, or proxy-port 0.
// Fields without an entry in Changed are left as they are.
type ModifyOptions struct {
	Name string
	// Path updates the stored project path only. Files on disk are not moved.
	Path         string
	Domains      []string
	AddDomain    string
	RemoveDomain string
	WebServer    WebServer
	Runtime      Runtime
	PHPVersion   string
	NodeVersion  string
	RubyVersion  string
	PublicDir    string
	ProxyPort    int
	// SharedLogs, when set, updates whether the project uses server-wide nginx logs.
	// Nil leaves the stored value unchanged. False turns per-project logs back on.
	SharedLogs   *bool
	SSL          bool
	RemoveSSL    bool
	RedirectHTTP bool
	Yes          bool
	DryRun       bool
	Changed      map[string]bool
}

// State holds persisted project state.
type State struct {
	Name          string           `json:"name"`
	Path          string           `json:"path"`
	Domains       []string         `json:"domains"`
	WebServer     WebServer        `json:"web_server"`
	Runtime       Runtime          `json:"runtime"`
	PHPVersion    string           `json:"php_version,omitempty"`
	NodeVersion   string           `json:"node_version,omitempty"`
	RubyVersion   string           `json:"ruby_version,omitempty"`
	PublicDir     string           `json:"public_dir,omitempty"`
	ProxyPort     int              `json:"proxy_port,omitempty"`
	SSLEnabled    bool             `json:"ssl_enabled"`
	SSL           *SSLState        `json:"ssl,omitempty"`
	VhostPath     string           `json:"vhost_path"`
	SharedLogs    bool             `json:"shared_logs,omitempty"`
	AccessLogPath string           `json:"access_log_path,omitempty"`
	ErrorLogPath  string           `json:"error_log_path,omitempty"`
	Owner         string           `json:"owner"`
	Group         string           `json:"group,omitempty"`
	OwnershipMode OwnershipMode    `json:"ownership_mode,omitempty"`
	OwnerUID      int              `json:"owner_uid,omitempty"`
	OwnerGID      int              `json:"owner_gid,omitempty"`
	OwnerHome     string           `json:"owner_home,omitempty"`
	ApprovedRoot  string           `json:"approved_root,omitempty"`
	PHPPoolName   string           `json:"php_pool_name,omitempty"`
	PHPSocketPath string           `json:"php_socket_path,omitempty"`
	Services      []ProjectService `json:"services,omitempty"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

// ProjectService describes a service associated with a project.
type ProjectService struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// SSLState records the TLS certificate metadata Abstrax manages for a project.
//
// SSLEnabled on State is kept for backward compatibility. Projects that only
// have ssl_enabled set (no ssl object) are adopted into SSL the next time
// Abstrax needs the certificate name.
type SSLState struct {
	// CertificateName is the certbot lineage name (certbot --cert-name).
	CertificateName string `json:"certificate_name,omitempty"`
	// Domains are the names the certificate was issued for.
	Domains []string `json:"domains,omitempty"`
	// RedirectHTTP sends plain HTTP requests to HTTPS.
	RedirectHTTP bool `json:"redirect_http"`
	// Staging marks a Let's Encrypt staging certificate.
	Staging bool `json:"staging,omitempty"`
	// IssuedAt is when Abstrax last obtained the certificate.
	IssuedAt time.Time `json:"issued_at,omitempty"`
	// ExpiresAt is the certificate's notAfter, refreshed on add, renew, and status.
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	// LastError is the most recent SSL failure Abstrax recorded for the project.
	LastError string `json:"last_error,omitempty"`
}

// MarshalJSON omits zero timestamps, which omitempty does not do for time.Time.
func (s SSLState) MarshalJSON() ([]byte, error) {
	type plain struct {
		CertificateName string     `json:"certificate_name,omitempty"`
		Domains         []string   `json:"domains,omitempty"`
		RedirectHTTP    bool       `json:"redirect_http"`
		Staging         bool       `json:"staging,omitempty"`
		IssuedAt        *time.Time `json:"issued_at,omitempty"`
		ExpiresAt       *time.Time `json:"expires_at,omitempty"`
		LastError       string     `json:"last_error,omitempty"`
	}
	out := plain{
		CertificateName: s.CertificateName,
		Domains:         s.Domains,
		RedirectHTTP:    s.RedirectHTTP,
		Staging:         s.Staging,
		LastError:       s.LastError,
	}
	if !s.IssuedAt.IsZero() {
		t := s.IssuedAt
		out.IssuedAt = &t
	}
	if !s.ExpiresAt.IsZero() {
		t := s.ExpiresAt
		out.ExpiresAt = &t
	}
	return json.Marshal(out)
}
