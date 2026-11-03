package cli

import (
	"testing"

	"abstrax/internal/services/project"
)

func TestSelectedRuntimeRequiresExactlyOne(t *testing.T) {
	tests := []struct {
		name                     string
		php, node, ruby, static_ bool
		want                     project.Runtime
		wantErr                  bool
	}{
		{name: "php", php: true, want: project.RuntimePHP},
		{name: "node", node: true, want: project.RuntimeNode},
		{name: "ruby", ruby: true, want: project.RuntimeRuby},
		{name: "static", static_: true, want: project.RuntimeStatic},
		{name: "none", wantErr: true},
		{name: "two", php: true, node: true, wantErr: true},
		{name: "all", php: true, node: true, ruby: true, static_: true, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectedRuntime(tc.php, tc.node, tc.ruby, tc.static_)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("runtime = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProjectRuntimeSetCommandRegistered(t *testing.T) {
	cmd, _, err := NewProjectCmd().Find([]string{"runtime", "set"})
	if err != nil || cmd == nil || cmd.Name() != "set" {
		t.Fatalf("project runtime set not registered: %v", err)
	}
	for _, flag := range []string{"php", "node", "ruby", "static", "php-version", "node-version", "ruby-version", "public-dir", "proxy-port"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("missing --%s", flag)
		}
	}
}
