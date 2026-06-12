package selfupdate

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
)

var semverPrefix = regexp.MustCompile(`^v?(\d+\.\d+\.\d+)`)

type resolveInput struct {
	current         *semver.Version
	requested       string
	allowBreaking   bool
	published       []*semver.Version
	currentRaw      string
}

type resolveOutput struct {
	target                  *semver.Version
	latestOverall           *semver.Version
	breakingVersion         *semver.Version
	alreadyUpToDate         bool
	breakingUpdateAvailable bool
}

func normalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

func parseCurrentVersion(raw string) (*semver.Version, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "dev" || raw == "unknown" {
		return semver.NewVersion("0.0.0")
	}

	if match := semverPrefix.FindStringSubmatch(raw); len(match) == 2 {
		if v, err := semver.NewVersion(match[1]); err == nil {
			return v, nil
		}
	}

	if v, err := semver.NewVersion(normalizeVersion(raw)); err == nil {
		return v, nil
	}

	return nil, fmt.Errorf("could not parse current version %q", raw)
}

func resolveVersion(input resolveInput) (resolveOutput, error) {
	if len(input.published) == 0 {
		return resolveOutput{}, fmt.Errorf("no published releases available")
	}

	out := resolveOutput{
		latestOverall: input.published[len(input.published)-1],
	}

	if input.requested != "" {
		target, err := semver.NewVersion(normalizeVersion(input.requested))
		if err != nil {
			return resolveOutput{}, fmt.Errorf("invalid version %q: %w", input.requested, err)
		}
		if !containsVersion(input.published, target) {
			return resolveOutput{}, fmt.Errorf("version %s is not available on GitHub", target)
		}
		if !target.GreaterThan(input.current) {
			out.alreadyUpToDate = true
			out.target = target
			return out, nil
		}
		out.target = target
		out.breakingUpdateAvailable, out.breakingVersion = detectBreaking(input.current, out.latestOverall)
		return out, nil
	}

	if input.allowBreaking {
		if out.latestOverall.GreaterThan(input.current) {
			out.target = out.latestOverall
		} else {
			out.alreadyUpToDate = true
		}
		return out, nil
	}

	compatible := latestCompatible(input.current, input.published)
	if compatible != nil && compatible.GreaterThan(input.current) {
		out.target = compatible
		out.breakingUpdateAvailable, out.breakingVersion = detectBreaking(input.current, out.latestOverall)
		return out, nil
	}

	out.alreadyUpToDate = true
	out.breakingUpdateAvailable, out.breakingVersion = detectBreaking(input.current, out.latestOverall)
	return out, nil
}

func latestCompatible(current *semver.Version, published []*semver.Version) *semver.Version {
	var best *semver.Version
	for _, v := range published {
		if v.Major() != current.Major() {
			continue
		}
		if !v.GreaterThan(current) {
			continue
		}
		if best == nil || v.GreaterThan(best) {
			best = v
		}
	}
	return best
}

func detectBreaking(current, latest *semver.Version) (bool, *semver.Version) {
	if latest == nil || !latest.GreaterThan(current) {
		return false, nil
	}
	if latest.Major() > current.Major() {
		return true, latest
	}
	return false, nil
}

func containsVersion(versions []*semver.Version, target *semver.Version) bool {
	for _, v := range versions {
		if v.Equal(target) {
			return true
		}
	}
	return false
}
