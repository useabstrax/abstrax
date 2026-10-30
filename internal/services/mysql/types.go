package mysql

import "encoding/json"

// Config holds MySQL connection settings.
type Config struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"-"`
	Socket   string `json:"socket,omitempty"`
	Database string `json:"database,omitempty"`
}

// RootPasswordResult is returned when a root password is set during install or reset.
type RootPasswordResult struct {
	RootPassword string `json:"root_password"`
	Generated    bool   `json:"password_generated"`
}

// InstallOptions holds options for mysql install.
type InstallOptions struct {
	Version      string
	RootPassword string
	DryRun       bool
}

// ResetRootPasswordOptions holds options for mysql reset-root-password.
type ResetRootPasswordOptions struct {
	RootPassword string
	DryRun       bool
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

// UserAddResult is returned when a MySQL user is created.
type UserAddResult struct {
	Name      string `json:"name"`
	Host      string `json:"host"`
	Password  string `json:"password,omitempty"`
	Generated bool   `json:"password_generated"`
}

// Database describes a MySQL database.
type Database struct {
	Name string `json:"name"`
}

// UserListOptions controls mysql user list.
type UserListOptions struct {
	Grants        bool
	ExcludeSystem bool
}

// DatabaseGrant is a parsed database-level grant.
type DatabaseGrant struct {
	Name       string   `json:"name"`
	Privileges []string `json:"privileges"`
	Preset     *string  `json:"preset"`
}

// UserInfo describes a MySQL user.
// Grants, Databases and GlobalPrivileges are set only when requested.
type UserInfo struct {
	Name             string          `json:"name"`
	Host             string          `json:"host"`
	Grants           []string        `json:"grants,omitempty"`
	Databases        []DatabaseGrant `json:"databases,omitempty"`
	GlobalPrivileges []string        `json:"global_privileges,omitempty"`
}

// MarshalJSON keeps a list without grants as name and host only.
// When grants were loaded, empty privilege lists stay as empty arrays.
func (u UserInfo) MarshalJSON() ([]byte, error) {
	if u.Grants == nil && u.Databases == nil && u.GlobalPrivileges == nil {
		return json.Marshal(struct {
			Name string `json:"name"`
			Host string `json:"host"`
		}{Name: u.Name, Host: u.Host})
	}
	if u.Grants == nil {
		u.Grants = []string{}
	}
	if u.Databases == nil {
		u.Databases = []DatabaseGrant{}
	}
	if u.GlobalPrivileges == nil {
		u.GlobalPrivileges = []string{}
	}
	return json.Marshal(struct {
		Name             string          `json:"name"`
		Host             string          `json:"host"`
		Grants           []string        `json:"grants"`
		Databases        []DatabaseGrant `json:"databases"`
		GlobalPrivileges []string        `json:"global_privileges"`
	}{
		Name:             u.Name,
		Host:             u.Host,
		Grants:           u.Grants,
		Databases:        u.Databases,
		GlobalPrivileges: u.GlobalPrivileges,
	})
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
	PresetApp:      "SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, INDEX, DROP, REFERENCES",
	PresetAdmin:    "ALL PRIVILEGES",
}
