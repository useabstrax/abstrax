package web

// Backend identifies the web server backend.
type Backend string

const (
	BackendNginx  Backend = "nginx"
	BackendApache Backend = "apache"
)

// InstallOptions holds options for installing a web server.
type InstallOptions struct {
	Backend Backend
	Enable  bool
	Start   bool
	DryRun  bool
}

// TestResult holds the result of a web server configuration test.
type TestResult struct {
	OK      bool   `json:"ok"`
	Output  string `json:"output"`
	Backend string `json:"backend"`
}
