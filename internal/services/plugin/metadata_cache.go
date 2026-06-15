package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// MetadataCacheStore manages cached plugin metadata for help and listing.
type MetadataCacheStore struct {
	path string
}

// NewMetadataCacheStore creates a metadata cache store.
func NewMetadataCacheStore(path string) *MetadataCacheStore {
	return &MetadataCacheStore{path: path}
}

// Load reads the metadata cache.
func (m *MetadataCacheStore) Load() (*MetadataCache, error) {
	data, err := os.ReadFile(m.path)
	if os.IsNotExist(err) {
		return &MetadataCache{Plugins: map[string]MetadataCacheEntry{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var cache MetadataCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}
	if cache.Plugins == nil {
		cache.Plugins = map[string]MetadataCacheEntry{}
	}
	return &cache, nil
}

// Save writes the metadata cache.
func (m *MetadataCacheStore) Save(cache *MetadataCache) error {
	if err := os.MkdirAll(filepath.Dir(m.path), 0750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(m.path, data, 0640)
}

// UpdateEntry updates a single plugin entry in the cache.
func (m *MetadataCacheStore) UpdateEntry(name string, meta *Metadata) error {
	cache, err := m.Load()
	if err != nil {
		return err
	}
	cache.Plugins[name] = MetadataCacheEntry{
		Name:        meta.Name,
		DisplayName: meta.DisplayName,
		Description: meta.Description,
		Version:     meta.Version,
		Commands:    meta.Commands,
		CachedAt:    time.Now().UTC(),
	}
	return m.Save(cache)
}

// RemoveEntry removes a plugin from the metadata cache.
func (m *MetadataCacheStore) RemoveEntry(name string) error {
	cache, err := m.Load()
	if err != nil {
		return err
	}
	delete(cache.Plugins, name)
	return m.Save(cache)
}

// ListEntries returns cached metadata entries sorted by name.
func (m *MetadataCacheStore) ListEntries() ([]MetadataCacheEntry, error) {
	cache, err := m.Load()
	if err != nil {
		return nil, err
	}
	entries := make([]MetadataCacheEntry, 0, len(cache.Plugins))
	for _, e := range cache.Plugins {
		entries = append(entries, e)
	}
	return entries, nil
}
