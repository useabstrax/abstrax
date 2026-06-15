package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"

	"abstrax/internal/validate"
	"abstrax/internal/version"
)

var pluginCommandNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// FetchMetadata runs the plugin metadata subcommand and parses the JSON response.
func FetchMetadata(ctx context.Context, binaryPath string) (*Metadata, error) {
	cmd := exec.CommandContext(ctx, binaryPath, MetadataSubcommand, MetadataAction)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%w: running metadata command: %v", ErrMalformedMetadata, err)
	}

	var meta Metadata
	if err := json.Unmarshal(out, &meta); err != nil {
		return nil, fmt.Errorf("%w: parsing metadata JSON: %v", ErrMalformedMetadata, err)
	}
	return &meta, nil
}

// ValidateMetadata checks plugin metadata against protocol v1 requirements.
func ValidateMetadata(meta *Metadata, expectedCommand string) error {
	if meta == nil {
		return fmt.Errorf("%w: metadata is nil", ErrMalformedMetadata)
	}
	if meta.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("%w: got version %d, want %d", ErrUnsupportedProtocol, meta.ProtocolVersion, ProtocolVersion)
	}
	if strings.TrimSpace(meta.Name) == "" {
		return fmt.Errorf("%w: name is required", ErrMalformedMetadata)
	}
	if strings.TrimSpace(meta.DisplayName) == "" {
		return fmt.Errorf("%w: display_name is required", ErrMalformedMetadata)
	}
	if strings.TrimSpace(meta.Version) == "" {
		return fmt.Errorf("%w: version is required", ErrMalformedMetadata)
	}
	if strings.TrimSpace(meta.RequiresAbstrax) == "" {
		return fmt.Errorf("%w: requires_abstrax is required", ErrMalformedMetadata)
	}
	if err := validate.PluginName(meta.Name); err != nil {
		return fmt.Errorf("%w: %v", ErrMalformedMetadata, err)
	}
	if meta.Name != expectedCommand {
		return fmt.Errorf("%w: metadata name %q does not match executable command %q", ErrMalformedMetadata, meta.Name, expectedCommand)
	}
	for _, c := range meta.Commands {
		if strings.TrimSpace(c.Name) == "" {
			return fmt.Errorf("%w: command name is required", ErrMalformedMetadata)
		}
		if !pluginCommandNameRe.MatchString(c.Name) {
			return fmt.Errorf("%w: invalid command name %q", ErrMalformedMetadata, c.Name)
		}
	}
	return ValidateAbstraxConstraint(meta.RequiresAbstrax)
}

// ValidateAbstraxConstraint checks whether the current Abstrax version satisfies a constraint.
func ValidateAbstraxConstraint(constraintStr string) error {
	constraint, err := semver.NewConstraint(constraintStr)
	if err != nil {
		return fmt.Errorf("%w: invalid requires_abstrax constraint %q: %v", ErrMalformedMetadata, constraintStr, err)
	}
	current, err := parseAbstraxVersion()
	if err != nil {
		return err
	}
	if !constraint.Check(current) {
		return fmt.Errorf("%w: plugin requires %s, current Abstrax is %s", ErrIncompatibleAbstrax, constraintStr, current.String())
	}
	return nil
}

func parseAbstraxVersion() (*semver.Version, error) {
	raw := strings.TrimSpace(version.Version)
	if raw == "" || raw == "dev" {
		return semver.NewVersion("0.0.0")
	}
	raw = strings.TrimPrefix(raw, "v")
	if idx := strings.IndexByte(raw, ' '); idx >= 0 {
		raw = raw[:idx]
	}
	v, err := semver.NewVersion(raw)
	if err != nil {
		return semver.NewVersion("0.0.0")
	}
	return v, nil
}

// AbstraxVersionString returns a clean semver string for the current Abstrax build.
func AbstraxVersionString() string {
	v, err := parseAbstraxVersion()
	if err != nil {
		return "0.0.0"
	}
	return v.String()
}
