package project

import (
	"strings"
	"testing"
)

func TestBuildNginxConfigDefaultServer(t *testing.T) {
	conf, err := buildNginxConfig(vhostConfig{
		Name:          "bare",
		Path:          "/var/www/bare",
		Runtime:       RuntimeStatic,
		DefaultServer: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"listen 80 default_server;",
		"listen [::]:80 default_server;",
		"server_name _;",
	} {
		if !strings.Contains(conf, want) {
			t.Fatalf("missing %q in:\n%s", want, conf)
		}
	}
}

func TestDefaultServerMessages(t *testing.T) {
	notes := DefaultServerMessages(&State{VhostPath: "/etc/nginx/conf.d/abstrax-bare.conf", DefaultServer: true}, true)
	if len(notes) != 1 || !strings.Contains(notes[0], "server IP") {
		t.Fatalf("notes = %v", notes)
	}
	notes = DefaultServerMessages(&State{VhostPath: "/x", DefaultServer: false}, false)
	if len(notes) != 1 || !strings.Contains(notes[0], "will not answer") {
		t.Fatalf("notes = %v", notes)
	}
}
