package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
)

type githubRelease struct {
	TagName     string `json:"tag_name"`
	Prerelease  bool   `json:"prerelease"`
	Draft       bool   `json:"draft"`
	PublishedAt string `json:"published_at"`
}

type releaseClient struct {
	owner  string
	repo   string
	client *http.Client
}

func newReleaseClient() *releaseClient {
	return &releaseClient{
		owner: githubOwner,
		repo:  githubRepo,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *releaseClient) listReleases(ctx context.Context) ([]githubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?per_page=100", c.owner, c.repo)

	var all []githubRelease
	for url != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "abstrax-cli")

		resp, err := c.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching releases: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading releases response: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}

		var page []githubRelease
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("decoding releases: %w", err)
		}
		all = append(all, page...)

		url = parseNextLink(resp.Header.Get("Link"))
	}

	return all, nil
}

func parseNextLink(linkHeader string) string {
	if linkHeader == "" {
		return ""
	}
	for _, part := range strings.Split(linkHeader, ",") {
		part = strings.TrimSpace(part)
		if !strings.HasSuffix(part, `rel="next"`) {
			continue
		}
		start := strings.Index(part, "<")
		end := strings.Index(part, ">")
		if start >= 0 && end > start {
			return part[start+1 : end]
		}
	}
	return ""
}

func (c *releaseClient) publishedVersions(ctx context.Context) ([]*semver.Version, error) {
	releases, err := c.listReleases(ctx)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var versions []*semver.Version

	for _, release := range releases {
		if release.Draft || release.Prerelease {
			continue
		}
		raw := strings.TrimPrefix(release.TagName, "v")
		v, err := semver.NewVersion(raw)
		if err != nil {
			continue
		}
		key := v.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		versions = append(versions, v)
	}

	if len(versions) == 0 {
		return nil, fmt.Errorf("no published releases found for %s/%s", c.owner, c.repo)
	}

	sort.Slice(versions, func(i, j int) bool {
		return versions[i].LessThan(versions[j])
	})
	return versions, nil
}

func (c *releaseClient) versionExists(ctx context.Context, version string) (bool, error) {
	versions, err := c.publishedVersions(ctx)
	if err != nil {
		return false, err
	}
	target, err := semver.NewVersion(normalizeVersion(version))
	if err != nil {
		return false, fmt.Errorf("invalid version %q: %w", version, err)
	}
	for _, v := range versions {
		if v.Equal(target) {
			return true, nil
		}
	}
	return false, nil
}
