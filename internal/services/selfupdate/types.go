// Package selfupdate implements CLI self-update against GitHub releases.
package selfupdate

const (
	githubOwner = "useabstrax"
	githubRepo  = "abstrax"
	binaryName  = "abstrax"
)

// Options configures a self-update run.
type Options struct {
	// RequestedVersion is an explicit target version (without a leading "v").
	// When empty, the service picks the best compatible release.
	RequestedVersion string
	// AllowBreaking permits upgrading across major version boundaries.
	AllowBreaking bool
	DryRun        bool
	Verbose       bool
}

// Result describes the outcome of a self-update attempt.
type Result struct {
	CurrentVersion          string `json:"current_version"`
	TargetVersion           string `json:"target_version,omitempty"`
	LatestVersion           string `json:"latest_version,omitempty"`
	BreakingVersion         string `json:"breaking_version,omitempty"`
	Updated                 bool   `json:"updated"`
	AlreadyUpToDate         bool   `json:"already_up_to_date"`
	BreakingUpdateAvailable bool   `json:"breaking_update_available"`
	Message                 string `json:"message"`
	Notice                  string `json:"notice,omitempty"`
}
