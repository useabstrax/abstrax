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
	GitRepo      string
	Branch       string
	DeployKey    bool
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
	PublicDir    string
	ProxyPort    int
	SSL          bool
	RemoveSSL    bool
	RedirectHTTP bool
	DryRun       bool
}

// State holds persisted project state.
type State struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	Domains    []string  `json:"domains"`
	WebServer  WebServer `json:"web_server"`
	Runtime    Runtime   `json:"runtime"`
	SSLEnabled bool      `json:"ssl_enabled"`
	VhostPath  string    `json:"vhost_path"`
	Owner      string    `json:"owner"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
