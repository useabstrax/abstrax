package plugin

import (
	"context"
	"sort"
	"strings"
)

const (
	// SkipReasonLocal is set when a plugin was installed from a local path.
	SkipReasonLocal = "local"
	// SkipReasonUpToDate is set when the installed version is already the latest compatible release.
	SkipReasonUpToDate = "up_to_date"
)

// PluginUpdateItem is one plugin considered by UpdateAll.
type PluginUpdateItem struct {
	Name            string `json:"name"`
	Version         string `json:"version,omitempty"`
	PreviousVersion string `json:"previous_version,omitempty"`
	TargetVersion   string `json:"target_version,omitempty"`
	Reason          string `json:"reason,omitempty"`
	Error           string `json:"error,omitempty"`
}

// UpdateAllResult is the outcome of updating every installed plugin.
type UpdateAllResult struct {
	DryRun  bool               `json:"dry_run"`
	Updated []PluginUpdateItem `json:"updated"`
	Skipped []PluginUpdateItem `json:"skipped"`
	Failed  []PluginUpdateItem `json:"failed"`
}

// UpdateAll updates every installed plugin that can be updated from the registry.
// Local plugins are skipped. A plugin already on the latest compatible version is
// left in place. A failure for one plugin does not stop the others.
// When dryRun is true, UpdateAll reports what would change and does not install.
func (s *Service) UpdateAll(ctx context.Context, dryRun bool) (*UpdateAllResult, error) {
	records, err := s.store.List()
	if err != nil {
		return nil, err
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].Name < records[j].Name
	})

	out := &UpdateAllResult{
		DryRun:  dryRun,
		Updated: []PluginUpdateItem{},
		Skipped: []PluginUpdateItem{},
		Failed:  []PluginUpdateItem{},
	}

	platform, err := s.platformForUpdates(records)
	if err != nil {
		return nil, err
	}

	client := s.RegistryClient()
	for _, rec := range records {
		if rec.Source == SourceLocal {
			out.Skipped = append(out.Skipped, PluginUpdateItem{
				Name:    rec.Name,
				Version: rec.Version,
				Reason:  SkipReasonLocal,
			})
			continue
		}

		latest, err := client.GetLatestVersion(ctx, rec.Name, LatestVersionOptions{
			AbstraxVersion: AbstraxVersionString(),
			Platform:       platform,
			Channel:        "stable",
		})
		if err != nil {
			out.Failed = append(out.Failed, PluginUpdateItem{
				Name:    rec.Name,
				Version: rec.Version,
				Error:   err.Error(),
			})
			continue
		}
		if strings.TrimSpace(latest.Version) == "" {
			out.Failed = append(out.Failed, PluginUpdateItem{
				Name:    rec.Name,
				Version: rec.Version,
				Error:   "registry returned an empty version",
			})
			continue
		}
		if latest.Version == rec.Version {
			out.Skipped = append(out.Skipped, PluginUpdateItem{
				Name:    rec.Name,
				Version: rec.Version,
				Reason:  SkipReasonUpToDate,
			})
			continue
		}
		if dryRun {
			out.Updated = append(out.Updated, PluginUpdateItem{
				Name:          rec.Name,
				Version:       rec.Version,
				TargetVersion: latest.Version,
			})
			continue
		}

		updated, err := s.Update(ctx, rec.Name)
		if err != nil {
			out.Failed = append(out.Failed, PluginUpdateItem{
				Name:    rec.Name,
				Version: rec.Version,
				Error:   err.Error(),
			})
			continue
		}
		out.Updated = append(out.Updated, PluginUpdateItem{
			Name:            updated.Name,
			Version:         updated.Version,
			PreviousVersion: rec.Version,
		})
	}

	return out, nil
}

// platformForUpdates returns the registry platform when any plugin can be
// updated from the registry. Local-only installs do not need a platform.
func (s *Service) platformForUpdates(records []InstallRecord) (string, error) {
	for _, rec := range records {
		if rec.Source != SourceLocal {
			return CurrentPlatform()
		}
	}
	return "", nil
}
