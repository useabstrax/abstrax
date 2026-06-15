package plugin

import (
	"embed"
	"encoding/json"
	"testing"
)

// Registry contract fixtures mirrored from openapi/fixtures in the monorepo.
//
//go:embed testdata/fixtures/*
var registryFixtures embed.FS

func loadRegistryFixture(t *testing.T, name string) json.RawMessage {
	t.Helper()
	data, err := registryFixtures.ReadFile("testdata/fixtures/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}
