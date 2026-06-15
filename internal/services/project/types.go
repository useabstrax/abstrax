package project

import "time"

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
	WebServer    WebServer
	NoVhost      bool
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
type ModifyOptions struct {
	Name         string
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
	SSL          bool
	RemoveSSL    bool
	RedirectHTTP bool
	Yes          bool
	DryRun       bool
}

// State holds persisted project state.
type State struct {
	Name        string           `json:"name"`
	Path        string           `json:"path"`
	Domains     []string         `json:"domains"`
	WebServer   WebServer        `json:"web_server"`
	Runtime     Runtime          `json:"runtime"`
	PHPVersion  string           `json:"php_version,omitempty"`
	NodeVersion string           `json:"node_version,omitempty"`
	RubyVersion string           `json:"ruby_version,omitempty"`
	PublicDir   string           `json:"public_dir,omitempty"`
	ProxyPort   int              `json:"proxy_port,omitempty"`
	SSLEnabled  bool             `json:"ssl_enabled"`
	VhostPath   string           `json:"vhost_path"`
	Owner       string           `json:"owner"`
	Services    []ProjectService `json:"services,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// ProjectService describes a service associated with a project.
type ProjectService struct {
	Name string `json:"name"`
	Type string `json:"type"`
}
