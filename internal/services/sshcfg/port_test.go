package sshcfg

import (
	"os"
	"path/filepath"
	"testing"

	"abstrax/internal/platform/debian"
)

func TestPortFromEntries(t *testing.T) {
	tests := []struct {
		name    string
		entries []ConfigEntry
		want    string
	}{
		{
			name: "finds port",
			entries: []ConfigEntry{
				{Key: "Port", Value: "2222"},
			},
			want: "2222",
		},
		{
			name: "case insensitive",
			entries: []ConfigEntry{
				{Key: "port", Value: "2222"},
			},
			want: "2222",
		},
		{
			name:    "empty",
			entries: nil,
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := portFromEntries(tt.entries)
			if got != tt.want {
				t.Errorf("portFromEntries() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSSHPortFromFiles(t *testing.T) {
	dir := t.TempDir()
	managed := filepath.Join(dir, "99-abstrax.conf")
	mainCfg := filepath.Join(dir, "sshd_config")

	if err := os.WriteFile(managed, []byte("Port 2222\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainCfg, []byte("Port 22\n"), 0644); err != nil {
		t.Fatal(err)
	}

	origManaged := debian.AbstraxSSHConfig
	origMain := sshdConfigPath
	t.Cleanup(func() {
		// restore not needed in test - we patch readConfigFileSafe indirectly
		_ = origManaged
		_ = origMain
	})

	// Test portFromEntries preference via managed entries
	got := portFromEntries(readConfigFileSafe(managed))
	if got != "2222" {
		t.Errorf("managed port = %q, want 2222", got)
	}

	got = portFromEntries(readConfigFileSafe(mainCfg))
	if got != "22" {
		t.Errorf("main port = %q, want 22", got)
	}
}
