package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"abstrax/internal/services/config"
	"abstrax/internal/validate"
)

// InstallOptions configures plugin installation.
type InstallOptions struct {
	Name        string
	ManifestURL string
	RegistryURL string
	Force       bool
}

// InstallResult describes a completed installation.
type InstallResult struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	Publisher  string `json:"publisher"`
	TrustLevel string `json:"trust_level"`
	Source     string `json:"source"`
	BinaryPath string `json:"binary_path"`
}

// Service coordinates plugin management operations.
type Service struct {
	paths         *Paths
	store         *Store
	metaCache     *MetadataCacheStore
	registryCache *RegistryCache
	registryURL   string
	discoverer    *Discoverer
}

// New creates a Service with effective paths for the current user.
func New() (*Service, error) {
	paths, err := EffectivePaths()
	if err != nil {
		return nil, err
	}
	return NewWithPaths(paths, config.DefaultPluginRegistryURL), nil
}

// NewWithPaths creates a Service with custom paths (for tests).
func NewWithPaths(paths *Paths, registryURL string) *Service {
	if registryURL == "" {
		registryURL = config.DefaultPluginRegistryURL
	}
	return &Service{
		paths:         paths,
		store:         NewStore(paths.RecordDir),
		metaCache:     NewMetadataCacheStore(paths.MetadataCache),
		registryCache: NewRegistryCache(paths.RegistryCacheDir, RegistryCacheTTL),
		registryURL:   registryURL,
		discoverer:    NewDiscoverer(paths),
	}
}

// RegistryClient returns a caching registry client.
func (s *Service) RegistryClient() RegistryClient {
	return NewCachedRegistryClient(s.registryURL, s.registryCache)
}

// Paths returns the service paths.
func (s *Service) Paths() *Paths {
	return s.paths
}

// Store returns the installation record store.
func (s *Service) Store() *Store {
	return s.store
}

// MetadataCache returns the metadata cache store.
func (s *Service) MetadataCache() *MetadataCacheStore {
	return s.metaCache
}

// NewDispatcher creates a dispatcher for plugin execution.
func (s *Service) NewDispatcher() (*Dispatcher, error) {
	return NewDispatcher(s.paths, s.store)
}

// Install installs a plugin from the registry or a manifest URL.
func (s *Service) Install(ctx context.Context, opts InstallOptions) (*InstallResult, error) {
	if err := validate.PluginName(opts.Name); err != nil {
		return nil, err
	}

	if opts.ManifestURL != "" {
		return s.installFromManifest(ctx, opts)
	}
	return s.installFromRegistry(ctx, opts)
}

func (s *Service) installFromRegistry(ctx context.Context, opts InstallOptions) (*InstallResult, error) {
	client := s.RegistryClient()
	registryURL := opts.RegistryURL
	if registryURL == "" {
		registryURL = s.registryURL
	}

	pluginInfo, err := client.GetPlugin(ctx, opts.Name)
	if err != nil {
		return nil, err
	}
	if pluginInfo.Status == StatusBlocked {
		return nil, fmt.Errorf("%w: %s is blocked by the registry", ErrBlockedPlugin, opts.Name)
	}

	platform, err := CurrentPlatform()
	if err != nil {
		return nil, err
	}

	versionInfo, err := client.GetLatestVersion(ctx, opts.Name, LatestVersionOptions{
		AbstraxVersion: AbstraxVersionString(),
		Platform:       platform,
		Channel:        "stable",
	})
	if err != nil {
		return nil, err
	}
	if !versionInfo.Stable && versionInfo.Channel != "stable" {
		return nil, fmt.Errorf("no stable version available for plugin %q", opts.Name)
	}
	if err := ValidateAbstraxConstraint(versionInfo.RequiresAbstrax); err != nil {
		return nil, err
	}

	binary, ok := versionInfo.Platforms[platform]
	if !ok {
		return nil, fmt.Errorf("%w: plugin %q has no binary for %s", ErrUnsupportedPlatform, opts.Name, platform)
	}

	return s.installBinary(ctx, installBinaryOpts{
		name:           opts.Name,
		version:        versionInfo.Version,
		publisher:      pluginInfo.Publisher,
		trustLevel:     pluginInfo.TrustLevel,
		source:         SourceRegistry,
		registryURL:    registryURL,
		registryStatus: pluginInfo.Status,
		binaryURL:      binary.URL,
		sha256:         binary.SHA256,
	})
}

func (s *Service) installFromManifest(ctx context.Context, opts InstallOptions) (*InstallResult, error) {
	manifest, err := fetchManifest(ctx, opts.ManifestURL)
	if err != nil {
		return nil, err
	}
	if manifest.Name != opts.Name {
		return nil, fmt.Errorf("manifest plugin name %q does not match requested name %q", manifest.Name, opts.Name)
	}
	if manifest.Status == StatusBlocked {
		return nil, fmt.Errorf("%w: %s is blocked", ErrBlockedPlugin, manifest.Name)
	}
	if err := ValidateAbstraxConstraint(manifest.RequiresAbstrax); err != nil {
		return nil, err
	}

	platform, err := CurrentPlatform()
	if err != nil {
		return nil, err
	}
	binary, ok := manifest.Platforms[platform]
	if !ok {
		return nil, fmt.Errorf("%w: manifest has no binary for %s", ErrUnsupportedPlatform, platform)
	}

	trust := manifest.TrustLevel
	if trust == "" {
		trust = TrustCommunity
	}

	return s.installBinary(ctx, installBinaryOpts{
		name:           manifest.Name,
		version:        manifest.Version,
		publisher:      manifest.Publisher,
		trustLevel:     trust,
		source:         SourceManifest,
		registryStatus: manifest.Status,
		binaryURL:      binary.URL,
		sha256:         binary.SHA256,
	})
}

type installBinaryOpts struct {
	name           string
	version        string
	publisher      string
	trustLevel     string
	source         string
	registryURL    string
	registryStatus string
	binaryURL      string
	sha256         string
}

func (s *Service) installBinary(ctx context.Context, opts installBinaryOpts) (*InstallResult, error) {
	tmpFile, err := downloadToTemp(ctx, opts.binaryURL)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile)

	if err := verifyFileSHA256(tmpFile, opts.sha256); err != nil {
		return nil, err
	}

	if err := os.Chmod(tmpFile, 0o755); err != nil {
		return nil, fmt.Errorf("making plugin executable: %w", err)
	}

	meta, err := FetchMetadata(ctx, tmpFile)
	if err != nil {
		return nil, err
	}
	if err := ValidateMetadata(meta, opts.name); err != nil {
		return nil, err
	}

	destPath := filepath.Join(s.paths.InstallDir, PluginBinaryName(opts.name))
	if err := os.MkdirAll(s.paths.InstallDir, 0755); err != nil {
		return nil, fmt.Errorf("creating plugin directory: %w", err)
	}
	if err := atomicInstall(tmpFile, destPath); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	rec := &InstallRecord{
		Name:           opts.name,
		Version:        opts.version,
		Publisher:      opts.publisher,
		TrustLevel:     opts.trustLevel,
		Source:         opts.source,
		RegistryURL:    opts.registryURL,
		InstalledAt:    now,
		SHA256:         opts.sha256,
		BinaryPath:     destPath,
		RegistryStatus: opts.registryStatus,
		StatusCachedAt: now,
	}
	if err := s.store.Save(rec); err != nil {
		return nil, err
	}
	if err := s.metaCache.UpdateEntry(opts.name, meta); err != nil {
		return nil, err
	}

	return &InstallResult{
		Name:       opts.name,
		Version:    opts.version,
		Publisher:  opts.publisher,
		TrustLevel: opts.trustLevel,
		Source:     opts.source,
		BinaryPath: destPath,
	}, nil
}

// Update reinstalls a plugin using the safe replacement flow.
func (s *Service) Update(ctx context.Context, name string) (*InstallResult, error) {
	if _, err := s.store.Load(name); err != nil {
		return nil, err
	}
	return s.installFromRegistry(ctx, InstallOptions{Name: name})
}

// Remove deletes a plugin binary and its installation record.
func (s *Service) Remove(name string) error {
	rec, err := s.store.Load(name)
	if err != nil {
		return err
	}
	if rec.BinaryPath != "" {
		if err := os.Remove(rec.BinaryPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing plugin binary: %w", err)
		}
	}
	if err := s.store.Remove(name); err != nil {
		return err
	}
	return s.metaCache.RemoveEntry(name)
}

// ListInstalled returns installed plugins with optional update availability.
func (s *Service) ListInstalled(ctx context.Context) ([]ListEntry, error) {
	records, err := s.store.List()
	if err != nil {
		return nil, err
	}

	var entries []ListEntry
	for _, rec := range records {
		entry := ListEntry{
			Name:       rec.Name,
			Version:    rec.Version,
			Publisher:  rec.Publisher,
			TrustLevel: rec.TrustLevel,
			Status:     rec.RegistryStatus,
			Source:     rec.Source,
		}
		if entry.Status == "" {
			entry.Status = StatusActive
		}
		if latest, err := s.RegistryClient().GetLatestVersion(ctx, rec.Name, LatestVersionOptions{
			AbstraxVersion: AbstraxVersionString(),
			Channel:        "stable",
		}); err == nil && latest.Version != rec.Version {
			entry.UpdateAvailable = latest.Version
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// ListEntry describes an installed plugin for display.
type ListEntry struct {
	Name            string `json:"name"`
	Version         string `json:"version"`
	Publisher       string `json:"publisher"`
	TrustLevel      string `json:"trust_level"`
	Status          string `json:"status"`
	Source          string `json:"source"`
	UpdateAvailable string `json:"update_available,omitempty"`
}

// InfoEntry describes detailed plugin information.
type InfoEntry struct {
	Name            string            `json:"name"`
	DisplayName     string            `json:"display_name"`
	Version         string            `json:"version"`
	Description     string            `json:"description"`
	Publisher       string            `json:"publisher"`
	TrustLevel      string            `json:"trust_level"`
	Source          string            `json:"source"`
	Homepage        string            `json:"homepage,omitempty"`
	RequiresAbstrax string            `json:"requires_abstrax,omitempty"`
	InstalledPath   string            `json:"installed_path"`
	Commands        []MetadataCommand `json:"commands"`
	RegistryStatus  string            `json:"registry_status"`
	UpdateAvailable string            `json:"update_available,omitempty"`
}

// Info returns detailed information about an installed plugin.
func (s *Service) Info(ctx context.Context, name string) (*InfoEntry, error) {
	rec, err := s.store.Load(name)
	if err != nil {
		return nil, err
	}

	entry := &InfoEntry{
		Name:           rec.Name,
		Version:        rec.Version,
		Publisher:      rec.Publisher,
		TrustLevel:     rec.TrustLevel,
		Source:         rec.Source,
		InstalledPath:  rec.BinaryPath,
		RegistryStatus: rec.RegistryStatus,
	}
	if entry.RegistryStatus == "" {
		entry.RegistryStatus = StatusActive
	}

	if cache, err := s.metaCache.Load(); err == nil {
		if cached, ok := cache.Plugins[name]; ok {
			entry.DisplayName = cached.DisplayName
			entry.Description = cached.Description
			entry.Commands = cached.Commands
		}
	}

	if rec.BinaryPath != "" {
		if meta, err := FetchMetadata(ctx, rec.BinaryPath); err == nil {
			entry.DisplayName = meta.DisplayName
			entry.Description = meta.Description
			entry.Homepage = meta.Homepage
			entry.RequiresAbstrax = meta.RequiresAbstrax
			entry.Commands = meta.Commands
			_ = s.metaCache.UpdateEntry(name, meta)
		}
	}

	if pluginInfo, err := s.RegistryClient().GetPlugin(ctx, name); err == nil {
		entry.RegistryStatus = pluginInfo.Status
		rec.RegistryStatus = pluginInfo.Status
		rec.StatusCachedAt = time.Now().UTC()
		_ = s.store.Save(rec)
	}
	if latest, err := s.RegistryClient().GetLatestVersion(ctx, name, LatestVersionOptions{
		AbstraxVersion: AbstraxVersionString(),
		Channel:        "stable",
	}); err == nil && latest.Version != rec.Version {
		entry.UpdateAvailable = latest.Version
	}

	return entry, nil
}

// Search queries the registry for plugins matching a query string.
func (s *Service) Search(ctx context.Context, query string) ([]RegistryPluginSummary, error) {
	plugins, err := s.RegistryClient().ListPlugins(ctx)
	if err != nil {
		return nil, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return plugins, nil
	}
	var matches []RegistryPluginSummary
	for _, p := range plugins {
		if strings.Contains(strings.ToLower(p.Name), query) ||
			strings.Contains(strings.ToLower(p.Description), query) ||
			strings.Contains(strings.ToLower(p.Publisher), query) {
			matches = append(matches, p)
		}
	}
	return matches, nil
}

// RefreshMetadataCache updates cached metadata for an installed plugin.
func (s *Service) RefreshMetadataCache(ctx context.Context, name string) error {
	rec, err := s.store.Load(name)
	if err != nil {
		return err
	}
	meta, err := FetchMetadata(ctx, rec.BinaryPath)
	if err != nil {
		return err
	}
	return s.metaCache.UpdateEntry(name, meta)
}

func fetchManifest(ctx context.Context, url string) (*Manifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "abstrax-cli")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: fetching manifest: %v", ErrRegistryUnavailable, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: manifest request returned %d", ErrRegistryUnavailable, resp.StatusCode)
	}

	var manifest Manifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}
	return &manifest, nil
}

func downloadToTemp(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "abstrax-cli")

	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: downloading plugin: %v", ErrRegistryUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("%w: download returned %d: %s", ErrRegistryUnavailable, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	tmp, err := os.CreateTemp("", "abstrax-plugin-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("writing plugin download: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return "", err
	}
	return tmpPath, nil
}

func verifyFileSHA256(path, expected string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	actual := hex.EncodeToString(sum[:])
	if !strings.EqualFold(actual, strings.TrimSpace(expected)) {
		return fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, expected, actual)
	}
	return nil
}

func atomicInstall(src, dest string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	mode := srcInfo.Mode().Perm() | 0o111
	if err := os.Chmod(src, mode); err != nil {
		return err
	}

	if err := os.Rename(src, dest); err == nil {
		return nil
	}

	destDir := filepath.Dir(dest)
	tmpDest := filepath.Join(destDir, fmt.Sprintf(".%s.new.%d", filepath.Base(dest), os.Getpid()))

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(tmpDest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("writing plugin to %s: %w", tmpDest, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmpDest)
		return fmt.Errorf("writing plugin to %s: %w", tmpDest, err)
	}
	if err := out.Close(); err != nil {
		os.Remove(tmpDest)
		return fmt.Errorf("writing plugin to %s: %w", tmpDest, err)
	}
	if err := os.Rename(tmpDest, dest); err != nil {
		os.Remove(tmpDest)
		return fmt.Errorf("installing plugin to %s: %w", dest, err)
	}
	return nil
}
