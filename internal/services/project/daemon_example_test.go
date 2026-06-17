package project

import "testing"

func TestDaemonAddExampleForNode(t *testing.T) {
	ex := DaemonAddExampleFor(&State{
		Name:      "myapp",
		Path:      "/home/mike/myapp",
		Owner:     "mike",
		Runtime:   RuntimeNode,
		ProxyPort: 4000,
	})
	if ex == nil {
		t.Fatal("expected example")
	}
	lines := ex.FormatLines()
	if len(lines) != 5 {
		t.Fatalf("lines = %#v", lines)
	}
	if lines[0] != `sudo abstrax daemon add abstrax-myapp-web \` {
		t.Fatalf("first line = %q", lines[0])
	}
	if lines[4] != "  --environment=PORT=4000" {
		t.Fatalf("last line = %q", lines[4])
	}
}

func TestDaemonAddExampleForRuby(t *testing.T) {
	ex := DaemonAddExampleFor(&State{
		Name:    "myapp",
		Path:    "/var/www/myapp",
		Owner:   "www-data",
		Runtime: RuntimeRuby,
	})
	lines := ex.FormatLines()
	if len(lines) != 4 {
		t.Fatalf("lines = %#v", lines)
	}
	if lines[1] != `  --command="bundle exec puma -p 3000 -b 127.0.0.1" \` {
		t.Fatalf("command line = %q", lines[1])
	}
}

func TestDaemonAddExampleForPHP(t *testing.T) {
	if ex := DaemonAddExampleFor(&State{Name: "myapp", Runtime: RuntimePHP}); ex != nil {
		t.Fatalf("expected nil, got %#v", ex)
	}
}
