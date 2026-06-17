package config

// Settings holds Abstrax configuration. Values in the on-disk file override
// built-in defaults for the keys that are present.
type Settings struct {
	PHP      *PHPSettings     `json:"php,omitempty"`
	Plugins  *PluginSettings  `json:"plugins,omitempty"`
	Projects *ProjectSettings `json:"projects,omitempty"`
}

// ProjectSettings holds project-related configuration.
type ProjectSettings struct {
	ApprovedRoots []string `json:"approved_roots,omitempty"`
}

// PluginSettings holds plugin-related configuration.
type PluginSettings struct {
	RegistryURL  string   `json:"registry_url,omitempty"`
	AllowBlocked []string `json:"allow_blocked,omitempty"`
}

// DefaultPluginRegistryURL is the default Abstrax plugin registry base URL.
const DefaultPluginRegistryURL = "https://plugins.useabstrax.com/api/v1"

// PHPSettings holds PHP-related configuration.
type PHPSettings struct {
	Extensions []string `json:"extensions,omitempty"`
}

// DefaultPHPExtensions are installed alongside php-fpm and php-cli when PHP
// is installed for a project. Values are apt package suffixes, not full names.
var DefaultPHPExtensions = []string{
	"mysql",
	"xml",
	"curl",
	"mbstring",
	"zip",
	"bcmath",
	"gd",
}

// PHPPackages returns apt package names for a PHP version and extension list.
func PHPPackages(version string, extensions []string) []string {
	fpm := "php" + version + "-fpm"
	cli := "php" + version + "-cli"
	pkgs := []string{fpm, cli}
	for _, ext := range extensions {
		pkgs = append(pkgs, "php"+version+"-"+ext)
	}
	return pkgs
}
