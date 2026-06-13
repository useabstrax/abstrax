// Package config manages Abstrax settings stored in /etc/abstrax/config.json.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"abstrax/internal/platform/debian"
)

// Service manages Abstrax configuration.
type Service struct {
	path string
}

// New creates a Service using the default config path.
func New() *Service {
	return &Service{path: debian.AbstraxConfig}
}

// NewWithPath creates a Service with a custom config path (for tests).
func NewWithPath(path string) *Service {
	return &Service{path: path}
}

// Load reads stored settings from disk. A missing file returns empty settings.
func (s *Service) Load() (*Settings, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return &Settings{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var stored Settings
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &stored, nil
}

// Effective returns settings merged with built-in defaults.
func (s *Service) Effective() (*Settings, error) {
	stored, err := s.Load()
	if err != nil {
		return nil, err
	}
	return mergeDefaults(stored), nil
}

// PHPExtensions returns the effective PHP extension suffix list.
func (s *Service) PHPExtensions() ([]string, error) {
	effective, err := s.Effective()
	if err != nil {
		return nil, err
	}
	return slices.Clone(effective.PHP.Extensions), nil
}

// Save writes settings to disk.
func (s *Service) Save(stored *Settings) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(s.path, data, 0640); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

// Get returns the effective value for a config key.
func (s *Service) Get(key string) (any, error) {
	key, err := ParseKey(key)
	if err != nil {
		return nil, err
	}

	effective, err := s.Effective()
	if err != nil {
		return nil, err
	}

	switch key {
	case keyPHPExtensions:
		return slices.Clone(effective.PHP.Extensions), nil
	default:
		return nil, fmt.Errorf("unknown config key %q", key)
	}
}

// Set replaces the value for a list config key.
func (s *Service) Set(key string, values []string) error {
	key, err := ParseKey(key)
	if err != nil {
		return err
	}
	if len(values) == 0 {
		return fmt.Errorf("at least one value is required; use %q to restore defaults", "config reset "+key)
	}

	stored, err := s.Load()
	if err != nil {
		return err
	}

	switch key {
	case keyPHPExtensions:
		if stored.PHP == nil {
			stored.PHP = &PHPSettings{}
		}
		stored.PHP.Extensions = dedupe(values)
	default:
		return fmt.Errorf("unknown config key %q", key)
	}

	return s.Save(stored)
}

// Add appends a value to a list config key.
func (s *Service) Add(key, value string) error {
	key, err := ParseKey(key)
	if err != nil {
		return err
	}
	value = stringsTrim(value)
	if value == "" {
		return errors.New("value is required")
	}

	effective, err := s.Effective()
	if err != nil {
		return err
	}

	switch key {
	case keyPHPExtensions:
		values := append(slices.Clone(effective.PHP.Extensions), value)
		return s.Set(key, values)
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
}

// Remove removes a value from a list config key.
func (s *Service) Remove(key, value string) error {
	key, err := ParseKey(key)
	if err != nil {
		return err
	}
	value = stringsTrim(value)
	if value == "" {
		return errors.New("value is required")
	}

	effective, err := s.Effective()
	if err != nil {
		return err
	}

	switch key {
	case keyPHPExtensions:
		var values []string
		found := false
		for _, ext := range effective.PHP.Extensions {
			if ext == value {
				found = true
				continue
			}
			values = append(values, ext)
		}
		if !found {
			return fmt.Errorf("%q is not in %s", value, key)
		}
		if len(values) == 0 {
			return fmt.Errorf("cannot remove the last value from %s; use %q instead", key, "config reset "+key)
		}
		return s.Set(key, values)
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
}

// Reset restores defaults for one key or the entire config file.
func (s *Service) Reset(key string) error {
	if key == "" {
		if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("resetting config: %w", err)
		}
		return nil
	}

	key, err := ParseKey(key)
	if err != nil {
		return err
	}

	stored, err := s.Load()
	if err != nil {
		return err
	}

	switch key {
	case keyPHPExtensions:
		if stored.PHP != nil {
			stored.PHP.Extensions = nil
			stored.PHP = nil
		}
	default:
		return fmt.Errorf("unknown config key %q", key)
	}

	if isEmptyStored(stored) {
		return s.Reset("")
	}
	return s.Save(stored)
}

func mergeDefaults(stored *Settings) *Settings {
	effective := &Settings{
		PHP: &PHPSettings{
			Extensions: slices.Clone(DefaultPHPExtensions),
		},
	}
	if stored == nil || stored.PHP == nil || stored.PHP.Extensions == nil {
		return effective
	}
	effective.PHP.Extensions = slices.Clone(stored.PHP.Extensions)
	return effective
}

func isEmptyStored(stored *Settings) bool {
	return stored == nil || (stored.PHP == nil)
}

func dedupe(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = stringsTrim(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func stringsTrim(s string) string {
	return strings.TrimSpace(s)
}