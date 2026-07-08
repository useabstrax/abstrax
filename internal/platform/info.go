package platform

// Info describes the detected platform capabilities.
type Info struct {
	OSName          string  `json:"os_name"`
	OSVersion       string  `json:"os_version"`
	OSPrettyName    string  `json:"os_pretty_name"`
	KernelVersion   string  `json:"kernel_version"`
	Architecture    string  `json:"architecture"`
	PackageManager  string  `json:"package_manager"`
	ServiceManager  string  `json:"service_manager"`
	FirewallBackend string  `json:"firewall_backend"`
	IsRoot          bool    `json:"is_root"`
	Profile         Profile `json:"profile"`
	// Supported signals whether Abstrax allows mutating commands on this platform.
	Supported   bool   `json:"supported"`
	SupportNote string `json:"support_note,omitempty"`
	// Family is the detected distro family (debian or unknown).
	Family string `json:"family"`
}

// Tool presence flags filled in by Detect().
type Tools struct {
	Nginx      bool `json:"nginx"`
	Apache2    bool `json:"apache2"`
	Certbot    bool `json:"certbot"`
	MySQL      bool `json:"mysql"`
	MariaDB    bool `json:"mariadb"`
	Supervisor bool `json:"supervisor"`
	Redis      bool `json:"redis"`
	Memcached  bool `json:"memcached"`
	UFW        bool `json:"ufw"`
	Curl       bool `json:"curl"`
	Git        bool `json:"git"`
}
