package web

import (
	"strings"
	"testing"
)

func TestRenderIPConfigAsDefault(t *testing.T) {
	got := renderIPConfig(404, true)
	for _, want := range []string{
		"listen 80 default_server;",
		"listen [::]:80 default_server;",
		"return 404;",
		"listen 443 default_server ssl;",
		"ssl_reject_handshake on;",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderIPConfigDemoted(t *testing.T) {
	got := renderIPConfig(444, false)
	if strings.Contains(got, "default_server") {
		t.Fatalf("demoted block should not hold default_server:\n%s", got)
	}
	if !strings.Contains(got, "return 444;") {
		t.Fatalf("missing return:\n%s", got)
	}
}

func TestValidIPReturn(t *testing.T) {
	for _, code := range []int{404, 403, 444} {
		if !validIPReturn(code) {
			t.Fatalf("%d should be allowed", code)
		}
	}
	if validIPReturn(200) {
		t.Fatal("200 should not be allowed")
	}
}
