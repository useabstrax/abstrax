package selfupdate

import (
	"context"
	"fmt"

	"abstrax/internal/version"
)

// Service performs CLI self-updates from GitHub releases.
type Service struct {
	client *releaseClient
}

// New creates a self-update service.
func New() *Service {
	return &Service{client: newReleaseClient()}
}

// Update resolves the target release and installs it when needed.
func (s *Service) Update(ctx context.Context, opts Options) (Result, error) {
	currentRaw := version.Version
	current, err := parseCurrentVersion(currentRaw)
	if err != nil {
		return Result{}, err
	}

	published, err := s.client.publishedVersions(ctx)
	if err != nil {
		return Result{}, err
	}

	if opts.RequestedVersion != "" {
		exists, err := s.client.versionExists(ctx, opts.RequestedVersion)
		if err != nil {
			return Result{}, err
		}
		if !exists {
			return Result{}, fmt.Errorf("version %s does not exist on GitHub", normalizeVersion(opts.RequestedVersion))
		}
	}

	resolved, err := resolveVersion(resolveInput{
		current:       current,
		requested:     opts.RequestedVersion,
		allowBreaking: opts.AllowBreaking,
		published:     published,
		currentRaw:    currentRaw,
	})
	if err != nil {
		return Result{}, err
	}

	result := Result{
		CurrentVersion: current.String(),
		LatestVersion:  resolved.latestOverall.String(),
	}

	if resolved.breakingVersion != nil {
		result.BreakingVersion = resolved.breakingVersion.String()
		result.BreakingUpdateAvailable = resolved.breakingUpdateAvailable
	}

	if resolved.alreadyUpToDate || resolved.target == nil {
		result.AlreadyUpToDate = true
		if resolved.target != nil {
			result.TargetVersion = resolved.target.String()
		} else {
			result.TargetVersion = current.String()
		}
		result.Message = buildUpToDateMessage(result)
		if resolved.breakingUpdateAvailable {
			result.Notice = buildBreakingNotice(result.BreakingVersion)
		}
		return result, nil
	}

	result.TargetVersion = resolved.target.String()

	if opts.DryRun {
		result.Updated = false
		result.Message = fmt.Sprintf("Would update from %s to %s.", result.CurrentVersion, result.TargetVersion)
		if resolved.breakingUpdateAvailable {
			result.Notice = buildBreakingNotice(result.BreakingVersion)
		}
		return result, nil
	}

	installPath, err := installBinary(ctx, result.TargetVersion, false, opts.Verbose)
	if err != nil {
		return Result{}, err
	}

	result.Updated = true
	result.Message = fmt.Sprintf("Updated from %s to %s.", result.CurrentVersion, result.TargetVersion)
	if opts.Verbose {
		result.Message = fmt.Sprintf("%s Installed to %s.", result.Message, installPath)
	}

	if resolved.breakingUpdateAvailable {
		result.Notice = buildBreakingNotice(result.BreakingVersion)
	}

	return result, nil
}

func buildUpToDateMessage(r Result) string {
	if r.BreakingUpdateAvailable {
		return fmt.Sprintf("You are already on the latest compatible release (%s).", r.CurrentVersion)
	}
	return fmt.Sprintf("You are already on the latest release (%s).", r.CurrentVersion)
}

func buildBreakingNotice(breakingVersion string) string {
	return fmt.Sprintf(
		"A newer major release (%s) is available with breaking changes. Run `abstrax self update --allow-breaking` when you are ready to upgrade.",
		breakingVersion,
	)
}
