package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Store manages plugin installation records on disk.
type Store struct {
	recordDir string
}

// NewStore creates a Store for the given record directory.
func NewStore(recordDir string) *Store {
	return &Store{recordDir: recordDir}
}

// RecordPath returns the path to a plugin's installation record.
func (s *Store) RecordPath(name string) string {
	return filepath.Join(s.recordDir, name+".json")
}

// Load reads an installation record.
func (s *Store) Load(name string) (*InstallRecord, error) {
	data, err := os.ReadFile(s.RecordPath(name))
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if err != nil {
		return nil, fmt.Errorf("reading install record: %w", err)
	}
	var rec InstallRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("parsing install record: %w", err)
	}
	return &rec, nil
}

// Save writes an installation record.
func (s *Store) Save(rec *InstallRecord) error {
	if err := os.MkdirAll(s.recordDir, 0750); err != nil {
		return fmt.Errorf("creating record directory: %w", err)
	}
	rec.SchemaVersion = SchemaVersion
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding install record: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(s.RecordPath(rec.Name), data, 0640); err != nil {
		return fmt.Errorf("writing install record: %w", err)
	}
	return nil
}

// Remove deletes an installation record.
func (s *Store) Remove(name string) error {
	err := os.Remove(s.RecordPath(name))
	if os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return err
}

// List returns all installation records.
func (s *Store) List() ([]InstallRecord, error) {
	entries, err := os.ReadDir(s.recordDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading install records: %w", err)
	}

	var records []InstallRecord
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		name := e.Name()[:len(e.Name())-len(".json")]
		rec, err := s.Load(name)
		if err != nil {
			continue
		}
		records = append(records, *rec)
	}
	return records, nil
}
