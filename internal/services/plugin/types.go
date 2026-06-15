// Package plugin manages Abstrax CLI plugins: discovery, dispatch, installation,
// and registry interaction.
package plugin

import (
	"time"

	"abstrax/internal/services/config"
)

const (
	// ProtocolVersion is the supported plugin metadata protocol version.
	ProtocolVersion = 1

	// SchemaVersion is the installation record schema version.
	SchemaVersion = 1

	// MetadataSubcommand is the plugin subcommand that returns metadata JSON.
	MetadataSubcommand = "plugin"

	// MetadataAction is the metadata action within the plugin subcommand.
	MetadataAction = "metadata"

	// TrustOfficial is the highest trust level for registry plugins.
	TrustOfficial = "official"

	// TrustVerified is the verified publisher trust level.
	TrustVerified = "verified"

	// TrustCommunity is the community publisher trust level.
	TrustCommunity = "community"

	// StatusActive indicates a plugin is available for installation.
	StatusActive = "active"

	// StatusDeprecated indicates a plugin is deprecated but may still run.
	StatusDeprecated = "deprecated"

	// StatusBlocked indicates a plugin must not be installed or executed.
	StatusBlocked = "blocked"

	// SourceRegistry indicates installation from the official registry.
	SourceRegistry = "registry"

	// SourceManifest indicates installation from a direct manifest URL.
	SourceManifest = "manifest"

	// RegistryCacheTTL is how long registry cache entries remain fresh.
	RegistryCacheTTL = time.Hour
)

// MetadataCommand describes a subcommand exposed by a plugin.
type MetadataCommand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Metadata is the plugin metadata protocol v1 response.
type Metadata struct {
	ProtocolVersion int               `json:"protocol_version"`
	Name            string            `json:"name"`
	DisplayName     string            `json:"display_name"`
	Description     string            `json:"description"`
	Version         string            `json:"version"`
	RequiresAbstrax string            `json:"requires_abstrax"`
	Homepage        string            `json:"homepage,omitempty"`
	Commands        []MetadataCommand `json:"commands"`
}

// InstallRecord stores local installation metadata separate from the binary.
type InstallRecord struct {
	SchemaVersion  int       `json:"schema_version"`
	Name           string    `json:"name"`
	Version        string    `json:"version"`
	Publisher      string    `json:"publisher"`
	TrustLevel     string    `json:"trust_level"`
	Source         string    `json:"source"`
	RegistryURL    string    `json:"registry_url,omitempty"`
	InstalledAt    time.Time `json:"installed_at"`
	SHA256         string    `json:"sha256"`
	BinaryPath     string    `json:"binary_path"`
	RegistryStatus string    `json:"registry_status,omitempty"`
	StatusCachedAt time.Time `json:"status_cached_at,omitempty"`
}

// MetadataCacheEntry holds cached validated plugin metadata for help and listing.
type MetadataCacheEntry struct {
	Name        string            `json:"name"`
	DisplayName string            `json:"display_name"`
	Description string            `json:"description"`
	Version     string            `json:"version"`
	Commands    []MetadataCommand `json:"commands"`
	CachedAt    time.Time         `json:"cached_at"`
}

// MetadataCache is the on-disk metadata cache file.
type MetadataCache struct {
	Plugins map[string]MetadataCacheEntry `json:"plugins"`
}

// RegistryPluginSummary is a concise registry plugin listing entry.
type RegistryPluginSummary struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	Publisher     string `json:"publisher"`
	TrustLevel    string `json:"trust_level"`
	Status        string `json:"status"`
	LatestVersion string `json:"latest_version"`
	DisplayName   string `json:"display_name,omitempty"`
}

// RegistryPlugin is a full registry plugin record.
type RegistryPlugin struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	Publisher   string `json:"publisher"`
	TrustLevel  string `json:"trust_level"`
	Status      string `json:"status"`
	Homepage    string `json:"homepage,omitempty"`
}

// RegistryVersionSummary describes an available plugin version.
type RegistryVersionSummary struct {
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
}

// RegistryPlatformBinary describes a downloadable platform binary.
type RegistryPlatformBinary struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size,omitempty"`
}

// RegistryVersion is a full registry version record with platform binaries.
type RegistryVersion struct {
	Version         string                            `json:"version"`
	RequiresAbstrax string                            `json:"requires_abstrax"`
	Platforms       map[string]RegistryPlatformBinary `json:"platforms"`
	Stable          bool                              `json:"stable"`
	Channel         string                            `json:"channel,omitempty"`
	ProtocolVersion int                               `json:"protocol_version,omitempty"`
	ReleaseDate     string                            `json:"release_date,omitempty"`
	ManifestURL     string                            `json:"manifest_url,omitempty"`
}

// RegistryPagination describes list pagination metadata.
type RegistryPagination struct {
	CurrentPage int `json:"current_page"`
	LastPage    int `json:"last_page"`
	PerPage     int `json:"per_page"`
	Total       int `json:"total"`
}

// RegistryPluginsResponse is the response from GET /plugins.
type RegistryPluginsResponse struct {
	Plugins []RegistryPluginSummary `json:"plugins"`
	Meta    RegistryPagination      `json:"meta"`
}

// RegistryMetadata describes registry capabilities.
type RegistryMetadata struct {
	Name                     string   `json:"name"`
	SupportedPluginProtocols []int    `json:"supported_plugin_protocols"`
	SupportedPlatforms       []string `json:"supported_platforms"`
}

// RegistryMetadataResponse is the response from GET /registry.
type RegistryMetadataResponse struct {
	Registry RegistryMetadata `json:"registry"`
}

// RegistryErrorBody is the nested error object returned by the registry API.
type RegistryErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// RegistryErrorResponse is the registry API error envelope.
type RegistryErrorResponse struct {
	Error RegistryErrorBody `json:"error"`
}

// LatestVersionOptions configures GET /plugins/{name}/versions/latest.
type LatestVersionOptions struct {
	AbstraxVersion string
	Platform       string
	Channel        string
}

// Manifest describes a direct manifest installation source.
type Manifest struct {
	Name            string                            `json:"name"`
	Version         string                            `json:"version"`
	ProtocolVersion int                               `json:"protocol_version,omitempty"`
	Channel         string                            `json:"channel,omitempty"`
	Publisher       string                            `json:"publisher"`
	TrustLevel      string                            `json:"trust_level"`
	Description     string                            `json:"description,omitempty"`
	DisplayName     string                            `json:"display_name,omitempty"`
	RequiresAbstrax string                            `json:"requires_abstrax"`
	Platforms       map[string]RegistryPlatformBinary `json:"platforms"`
	Status          string                            `json:"status,omitempty"`
}

// Paths holds configurable plugin filesystem paths.
type Paths struct {
	SystemPluginDirs []string
	UserPluginDir    string
	RecordDir        string
	MetadataCache    string
	RegistryCacheDir string
	InstallDir       string
}

// DefaultPaths returns the default plugin paths for the current user.
func DefaultPaths() (*Paths, error) {
	return NewPaths("", "")
}

// NewPaths creates plugin paths, optionally overriding user home and install dir.
func NewPaths(homeDir, installDir string) (*Paths, error) {
	if homeDir == "" {
		var err error
		homeDir, err = userHomeDir()
		if err != nil {
			return nil, err
		}
	}

	userBase := homeDir + "/.local/share/abstrax/plugins"
	if installDir == "" {
		installDir = userBase
	}

	return &Paths{
		SystemPluginDirs: []string{
			"/usr/local/lib/abstrax/plugins",
			"/usr/lib/abstrax/plugins",
		},
		UserPluginDir:    userBase,
		RecordDir:        userBase + "/records",
		MetadataCache:    userBase + "/cache/metadata.json",
		RegistryCacheDir: userBase + "/cache/registry",
		InstallDir:       installDir,
	}, nil
}

// SystemPaths returns paths for root/system installations.
func SystemPaths() *Paths {
	return &Paths{
		SystemPluginDirs: []string{
			"/usr/local/lib/abstrax/plugins",
			"/usr/lib/abstrax/plugins",
		},
		UserPluginDir:    "",
		RecordDir:        "/var/lib/abstrax/plugins",
		MetadataCache:    "/var/lib/abstrax/plugins/cache/metadata.json",
		RegistryCacheDir: "/var/lib/abstrax/plugins/cache/registry",
		InstallDir:       "/usr/local/lib/abstrax/plugins",
	}
}

// DefaultRegistryURL returns the configured or default registry base URL.
func DefaultRegistryURL() string {
	return config.DefaultPluginRegistryURL
}
