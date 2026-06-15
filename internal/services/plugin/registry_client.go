package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RegistryClient fetches plugin information from a registry API.
type RegistryClient interface {
	GetRegistry(ctx context.Context) (*RegistryMetadataResponse, error)
	ListPlugins(ctx context.Context) ([]RegistryPluginSummary, error)
	GetPlugin(ctx context.Context, name string) (*RegistryPlugin, error)
	GetLatestVersion(ctx context.Context, name string, opts LatestVersionOptions) (*RegistryVersion, error)
	GetVersion(ctx context.Context, name, version string) (*RegistryVersion, error)
}

type registryCacheEntry struct {
	FetchedAt time.Time       `json:"fetched_at"`
	Body      json.RawMessage `json:"body"`
}

// RegistryCache stores registry HTTP responses locally.
type RegistryCache struct {
	dir string
	ttl time.Duration
}

// NewRegistryCache creates a registry response cache.
func NewRegistryCache(dir string, ttl time.Duration) *RegistryCache {
	if ttl == 0 {
		ttl = RegistryCacheTTL
	}
	return &RegistryCache{dir: dir, ttl: ttl}
}

func (c *RegistryCache) get(url string) (json.RawMessage, bool) {
	path := c.cachePath(url)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var entry registryCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}
	if time.Since(entry.FetchedAt) > c.ttl {
		return entry.Body, false
	}
	return entry.Body, true
}

func (c *RegistryCache) getStale(url string) (json.RawMessage, bool) {
	path := c.cachePath(url)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var entry registryCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}
	return entry.Body, true
}

func (c *RegistryCache) put(url string, body []byte) error {
	if err := os.MkdirAll(c.dir, 0750); err != nil {
		return err
	}
	entry := registryCacheEntry{
		FetchedAt: time.Now().UTC(),
		Body:      json.RawMessage(body),
	}
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(c.cachePath(url), data, 0640)
}

func (c *RegistryCache) cachePath(url string) string {
	sum := sha256.Sum256([]byte(url))
	name := hex.EncodeToString(sum[:]) + ".json"
	return filepath.Join(c.dir, name)
}

// CachedRegistryClient wraps registry HTTP access with local caching.
type CachedRegistryClient struct {
	baseURL string
	client  *http.Client
	cache   *RegistryCache
}

// NewCachedRegistryClient creates a caching registry client.
func NewCachedRegistryClient(baseURL string, cache *RegistryCache) *CachedRegistryClient {
	return &CachedRegistryClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 30 * time.Second},
		cache:   cache,
	}
}

func (c *CachedRegistryClient) fetch(ctx context.Context, path string, dest any) error {
	url := c.baseURL + path
	if body, ok := c.cache.get(url); ok {
		return json.Unmarshal(body, dest)
	}

	body, err := c.registryGet(ctx, url)
	if err != nil {
		if stale, ok := c.cache.getStale(url); ok {
			return json.Unmarshal(stale, dest)
		}
		return err
	}
	_ = c.cache.put(url, body)
	return json.Unmarshal(body, dest)
}

func (c *CachedRegistryClient) registryGet(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "abstrax-cli")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRegistryUnavailable, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: reading response: %v", ErrRegistryUnavailable, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, parseRegistryHTTPError(rawURL, resp.StatusCode, body)
	}
	return body, nil
}

func parseRegistryHTTPError(rawURL string, status int, body []byte) error {
	var apiErr RegistryErrorResponse
	if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Error.Code != "" {
		msg := apiErr.Error.Message
		if msg == "" {
			msg = strings.TrimSpace(string(body))
		}
		switch apiErr.Error.Code {
		case "plugin_not_found":
			return fmt.Errorf("%w: %s", ErrRegistryPluginNotFound, msg)
		case "version_not_found":
			return fmt.Errorf("%w: %s", ErrRegistryVersionNotFound, msg)
		case "no_compatible_version":
			return fmt.Errorf("%w: %s", ErrNoCompatibleVersion, msg)
		case "unsupported_platform":
			return fmt.Errorf("%w: %s", ErrUnsupportedPlatform, msg)
		case "plugin_blocked":
			return fmt.Errorf("%w: %s", ErrBlockedPlugin, msg)
		case "invalid_request":
			return fmt.Errorf("%w: %s", ErrRegistryUnavailable, msg)
		case "registry_error":
			return fmt.Errorf("%w: %s", ErrRegistryUnavailable, msg)
		default:
			return fmt.Errorf("%w: request to %s returned %d: %s", ErrRegistryUnavailable, rawURL, status, msg)
		}
	}
	return fmt.Errorf("%w: request to %s returned %d: %s", ErrRegistryUnavailable, rawURL, status, strings.TrimSpace(string(body)))
}

// GetRegistry returns registry metadata.
func (c *CachedRegistryClient) GetRegistry(ctx context.Context) (*RegistryMetadataResponse, error) {
	var resp RegistryMetadataResponse
	if err := c.fetch(ctx, "/registry", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListPlugins returns cached or fresh plugin list.
func (c *CachedRegistryClient) ListPlugins(ctx context.Context) ([]RegistryPluginSummary, error) {
	var resp RegistryPluginsResponse
	if err := c.fetch(ctx, "/plugins", &resp); err != nil {
		return nil, err
	}
	return resp.Plugins, nil
}

// GetPlugin returns a plugin record.
func (c *CachedRegistryClient) GetPlugin(ctx context.Context, name string) (*RegistryPlugin, error) {
	var resp RegistryPlugin
	if err := c.fetch(ctx, "/plugins/"+url.PathEscape(name), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetLatestVersion returns the latest compatible version.
func (c *CachedRegistryClient) GetLatestVersion(ctx context.Context, name string, opts LatestVersionOptions) (*RegistryVersion, error) {
	path := "/plugins/" + url.PathEscape(name) + "/versions/latest"
	query := url.Values{}
	if opts.AbstraxVersion != "" {
		query.Set("abstrax_version", opts.AbstraxVersion)
	}
	if opts.Platform != "" {
		query.Set("platform", opts.Platform)
	}
	if channel := strings.TrimSpace(opts.Channel); channel != "" {
		query.Set("channel", channel)
	} else {
		query.Set("channel", "stable")
	}
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}

	var resp RegistryVersion
	if err := c.fetch(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetVersion returns a specific version.
func (c *CachedRegistryClient) GetVersion(ctx context.Context, name, version string) (*RegistryVersion, error) {
	var resp RegistryVersion
	path := "/plugins/" + url.PathEscape(name) + "/versions/" + url.PathEscape(version)
	if err := c.fetch(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
