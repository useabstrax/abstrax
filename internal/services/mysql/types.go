package mysql

// Config holds MySQL connection settings.
type Config struct {
	Host     string `toml:"host" json:"host"`
	Port     int    `toml:"port" json:"port"`
	User     string `toml:"user" json:"user"`
	Password string `toml:"password" json:"-"`
	Socket   string `toml:"socket" json:"socket"`
	Database string `toml:"database" json:"database"`
}

// InstallOptions holds options for mysql install.
type InstallOptions struct {
	Version string
	Secure  bool
	DryRun  bool
}

// DBAddOptions holds options for database add.
type DBAddOptions struct {
	Name        string
	Charset     string
	Collation   string
	IfNotExists bool
	DryRun      bool
}

// UserAddOptions holds options for mysql user add.
type UserAddOptions struct {
	Name       string
	Host       string
	Password   string
	GrantDB    string
	Privileges string
	Preset     string
	DryRun     bool
}

// Database describes a MySQL database.
type Database struct {
	Name string `json:"name"`
}

// UserInfo describes a MySQL user.
type UserInfo struct {
	Name   string   `json:"name"`
	Host   string   `json:"host"`
	Grants []string `json:"grants"`
}

// Preset privilege levels.
const (
	PresetReadonly = "readonly"
	PresetApp      = "app"
	PresetAdmin    = "admin"
)

// PresetPrivileges maps preset names to privilege lists.
var PresetPrivileges = map[string]string{
	PresetReadonly: "SELECT",
	PresetApp:      "SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, INDEX, DROP",
	PresetAdmin:    "ALL PRIVILEGES",
}
