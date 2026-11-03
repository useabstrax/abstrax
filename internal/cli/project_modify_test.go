package cli

import (
	"slices"
	"testing"
)

func TestSplitDomains(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{" , ,", nil},
		{"a.com", []string{"a.com"}},
		{"a.com, b.com,,", []string{"a.com", "b.com"}},
	}
	for _, tt := range tests {
		if got := splitDomains(tt.in); !slices.Equal(got, tt.want) {
			t.Fatalf("splitDomains(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestProjectModifyFlagsReportChanged(t *testing.T) {
	cmd := newProjectModifyCmd()
	if err := cmd.ParseFlags([]string{"--domains=", "--proxy-port=0", "--public-dir=", "--clear-domains"}); err != nil {
		t.Fatal(err)
	}
	got := changedFlags(cmd, "domains", "proxy-port", "public-dir", "php-version", "shared-logs")
	for _, name := range []string{"domains", "proxy-port", "public-dir"} {
		if !got[name] {
			t.Fatalf("%s not reported as changed: %v", name, got)
		}
	}
	if got["php-version"] || got["shared-logs"] {
		t.Fatalf("unset flags reported as changed: %v", got)
	}
}
