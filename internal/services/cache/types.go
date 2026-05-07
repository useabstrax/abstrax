package cache

// Driver identifies a cache backend.
type Driver string

const (
	DriverRedis     Driver = "redis"
	DriverMemcached Driver = "memcached"
)

// InstallOptions holds options for installing a cache driver.
type InstallOptions struct {
	Driver          Driver
	Version         string
	Port            int
	Bind            string
	Memory          string
	Password        string
	Enable          bool
	Start           bool
	MaxMemoryPolicy string
	DryRun          bool
}

// RemoveOptions holds options for removing a cache driver.
type RemoveOptions struct {
	Driver Driver
	Purge  bool
	DryRun bool
}

// StatusInfo describes a cache driver status.
type StatusInfo struct {
	Driver  Driver `json:"driver"`
	Running bool   `json:"running"`
	Enabled bool   `json:"enabled"`
	Port    string `json:"port,omitempty"`
	Bind    string `json:"bind,omitempty"`
}
