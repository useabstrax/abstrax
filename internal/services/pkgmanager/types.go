package pkgmanager

// Manager is the interface for package manager backends.
type Manager interface {
	Install(opts InstallOptions) error
	Remove(opts RemoveOptions) error
	Update() error
	Upgrade(securityOnly bool) error
	Search(query string) ([]PackageInfo, error)
	Info(name string) (*PackageInfo, error)
	List() ([]PackageInfo, error)
}

// InstallOptions holds options for installing a package.
type InstallOptions struct {
	Name    string
	Version string
	DryRun  bool
}

// RemoveOptions holds options for removing a package.
type RemoveOptions struct {
	Name   string
	Purge  bool
	DryRun bool
}

// PackageInfo describes a package.
type PackageInfo struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	Description  string `json:"description"`
	Status       string `json:"status"`
	Architecture string `json:"architecture"`
}
