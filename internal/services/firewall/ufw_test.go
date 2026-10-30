package firewall

import (
	"context"
	"strings"
	"testing"

	executil "abstrax/internal/exec"
)

func TestParseUFWRulesFields(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		number    string
		action    string
		direction string
		port      string
		protocol  string
		from      string
		to        string
		comment   string
		ipv6      bool
	}{
		{
			name:      "v6 marker",
			line:      "[ 4] 22/tcp (v6)   ALLOW IN    Anywhere (v6)",
			number:    "4",
			action:    "ALLOW",
			direction: "in",
			port:      "22",
			protocol:  "tcp",
			ipv6:      true,
		},
		{
			name:      "comment",
			line:      "[ 2] 3306/tcp   ALLOW IN    10.0.0.12   # mysql from app",
			number:    "2",
			action:    "ALLOW",
			direction: "in",
			port:      "3306",
			protocol:  "tcp",
			from:      "10.0.0.12",
			comment:   "mysql from app",
		},
		{
			name:      "protocol split from port",
			line:      "[ 1] 22/tcp   ALLOW IN   Anywhere",
			number:    "1",
			action:    "ALLOW",
			direction: "in",
			port:      "22",
			protocol:  "tcp",
		},
		{
			name:      "deny from cidr with no port",
			line:      "[ 5] Anywhere   DENY IN   203.0.113.0/24",
			number:    "5",
			action:    "DENY",
			direction: "in",
			from:      "203.0.113.0/24",
		},
		{
			name:      "destination address and port",
			line:      "[ 6] 10.0.0.5 22/tcp   ALLOW IN   Anywhere",
			number:    "6",
			action:    "ALLOW",
			direction: "in",
			port:      "22",
			protocol:  "tcp",
			to:        "10.0.0.5",
		},
		{
			name:      "outbound range",
			line:      "[ 7] 8000:8100/tcp ALLOW OUT Anywhere (out)",
			number:    "7",
			action:    "ALLOW",
			direction: "out",
			port:      "8000:8100",
			protocol:  "tcp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := parseUFWRules(tt.line)
			if len(rules) != 1 {
				t.Fatalf("got %d rules", len(rules))
			}
			r := rules[0]
			if r.Number != tt.number || r.Action != tt.action || r.Direction != tt.direction {
				t.Fatalf("identity = %#v", r)
			}
			if r.Port != tt.port || r.Protocol != tt.protocol || r.From != tt.from || r.To != tt.to {
				t.Fatalf("fields = %#v", r)
			}
			if r.Comment != tt.comment || r.IPv6 != tt.ipv6 {
				t.Fatalf("comment/ipv6 = %#v", r)
			}
			if r.ID != ufwStableID(r) || len(r.ID) != 8 {
				t.Fatalf("id = %q", r.ID)
			}
		})
	}
}

func TestParseUFWRulesRealStatus(t *testing.T) {
	out := `Status: active

     To                         Action      From
     --                         ------      ----
[ 1] 22/tcp                     ALLOW IN    Anywhere
[ 2] 80/tcp                     ALLOW IN    Anywhere
[ 3] 443/tcp                    ALLOW IN    Anywhere
[ 4] 22/tcp (v6)                ALLOW IN    Anywhere (v6)
[ 5] 3306/tcp                   ALLOW IN    10.0.0.12                 # mysql from app
[ 6] Anywhere                   DENY IN     203.0.113.0/24
[ 7] 10.0.0.5 22/tcp            ALLOW IN    Anywhere
[ 8] 8000:8100/tcp              ALLOW OUT   Anywhere                   (out)
[ 9] OpenSSH                    ALLOW IN    Anywhere
[10] Nginx Full                 ALLOW IN    Anywhere
`
	rules := parseUFWRules(out)
	if len(rules) != 10 {
		t.Fatalf("got %d rules, want 10: %#v", len(rules), rules)
	}
	if rules[3].IPv6 != true || rules[3].Port != "22" || rules[3].From != "" {
		t.Fatalf("v6 rule = %#v", rules[3])
	}
	if rules[4].Comment != "mysql from app" || rules[4].From != "10.0.0.12" {
		t.Fatalf("comment rule = %#v", rules[4])
	}
	if rules[5].Port != "" || rules[5].From != "203.0.113.0/24" || rules[5].Action != "DENY" {
		t.Fatalf("deny rule = %#v", rules[5])
	}
	if rules[6].To != "10.0.0.5" || rules[6].Port != "22" {
		t.Fatalf("dest rule = %#v", rules[6])
	}
	if rules[7].Direction != "out" || rules[7].Port != "8000:8100" {
		t.Fatalf("out rule = %#v", rules[7])
	}
	if rules[8].Port != "OpenSSH" {
		t.Fatalf("app rule = %#v", rules[8])
	}
	if rules[9].Port != "Nginx Full" || rules[9].Number != "10" {
		t.Fatalf("multiword app = %#v", rules[9])
	}

	// v4 and v6 copies of the same port must not share an id.
	if rules[0].ID == rules[3].ID {
		t.Fatalf("v4 and v6 ssh rules share id %s", rules[0].ID)
	}
}

func TestUFWStableIDSurvivesRenumbering(t *testing.T) {
	before := parseUFWRules(`[ 3] 3306/tcp ALLOW IN 10.0.0.12
[ 4] 6379/tcp ALLOW IN 10.0.0.12`)
	after := parseUFWRules(`[ 3] 6379/tcp ALLOW IN 10.0.0.12`)
	if before[1].ID != after[0].ID {
		t.Fatalf("redis id changed from %s to %s", before[1].ID, after[0].ID)
	}
	if before[1].Number == after[0].Number {
		t.Fatalf("expected the position to change")
	}
}

func TestUFWDeleteArgs(t *testing.T) {
	rules := parseUFWRules("[ 2] 3306/tcp ALLOW IN 10.0.0.12 # mysql")
	got := strings.Join(ufwDeleteArgs(rules[0]), " ")
	want := "--force delete allow from 10.0.0.12 to any port 3306 proto tcp"
	if got != want {
		t.Fatalf("got %q", got)
	}

	simple := parseUFWRules("[ 1] 22/tcp ALLOW IN Anywhere")
	got = strings.Join(ufwDeleteArgs(simple[0]), " ")
	if got != "--force delete allow 22/tcp" {
		t.Fatalf("simple = %q", got)
	}

	out := parseUFWRules("[ 7] 8000:8100/tcp ALLOW OUT Anywhere (out)")
	got = strings.Join(ufwDeleteArgs(out[0]), " ")
	if got != "--force delete allow out 8000:8100/tcp" {
		t.Fatalf("out = %q", got)
	}
}

func TestUFWRuleRemoveByStableID(t *testing.T) {
	status := `[ 1] 22/tcp ALLOW IN Anywhere
[ 2] 3306/tcp ALLOW IN 10.0.0.12
[ 3] 6379/tcp ALLOW IN 10.0.0.12
`
	rules := parseUFWRules(status)
	var mysql Rule
	for _, r := range rules {
		if r.Port == "3306" {
			mysql = r
		}
	}

	runner := executil.New(true, false)
	runner.SetCommandFunc(func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		if name == "ufw" && len(args) >= 1 && args[0] == "status" {
			return executil.Result{Stdout: status}, true, nil
		}
		return executil.Result{}, false, nil
	})
	svc := &Service{runner: runner, backend: "ufw"}
	if err := svc.RuleRemove(context.Background(), mysql.ID); err != nil {
		t.Fatal(err)
	}

	var deleteCmd []string
	for _, cmd := range runner.Commands() {
		if len(cmd) > 1 && cmd[1] == "--force" {
			deleteCmd = cmd
		}
	}
	got := strings.Join(deleteCmd, " ")
	want := "ufw --force delete allow from 10.0.0.12 to any port 3306 proto tcp"
	if got != want {
		t.Fatalf("got %q", got)
	}
}

func TestUFWRuleRemoveByNumberWarnsAndUsesPosition(t *testing.T) {
	status := "[ 4] 6379/tcp ALLOW IN 10.0.0.12\n"
	runner := executil.New(true, false)
	runner.SetCommandFunc(func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		if name == "ufw" && len(args) >= 1 && args[0] == "status" {
			return executil.Result{Stdout: status}, true, nil
		}
		return executil.Result{}, false, nil
	})
	svc := &Service{runner: runner, backend: "ufw"}
	if err := svc.RuleRemove(context.Background(), "4"); err != nil {
		t.Fatal(err)
	}
	var deleteCmd []string
	for _, cmd := range runner.Commands() {
		if len(cmd) > 2 && cmd[1] == "--force" && cmd[2] == "delete" {
			deleteCmd = cmd
		}
	}
	got := strings.Join(deleteCmd, " ")
	if got != "ufw --force delete 4" {
		t.Fatalf("got %q", got)
	}
}

func TestUFWRuleRemoveMissing(t *testing.T) {
	runner := executil.New(true, false)
	runner.SetCommandFunc(func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		if name == "ufw" && len(args) >= 1 && args[0] == "status" {
			return executil.Result{Stdout: "[ 1] 22/tcp ALLOW IN Anywhere\n"}, true, nil
		}
		return executil.Result{}, false, nil
	})
	svc := &Service{runner: runner, backend: "ufw"}
	err := svc.RuleRemove(context.Background(), "deadbeef")
	if err == nil || !strings.Contains(err.Error(), "rule not found, it may already have been removed") {
		t.Fatalf("err = %v", err)
	}
}

func TestAllowIPCommentAndProtocol(t *testing.T) {
	runner := executil.New(true, false)
	svc := &Service{runner: runner, backend: "ufw"}
	err := svc.allowIP(context.Background(), AllowOptions{
		From:     "10.0.0.12",
		Port:     "3306",
		Protocol: "tcp",
		Comment:  "mysql from app",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(runner.Commands()[0], " ")
	want := "ufw allow from 10.0.0.12 port 3306 proto tcp comment mysql from app"
	if got != want {
		t.Fatalf("got %q", got)
	}
}

func TestDenyIPComment(t *testing.T) {
	runner := executil.New(true, false)
	svc := &Service{runner: runner, backend: "ufw"}
	err := svc.denyIP(context.Background(), AllowOptions{
		From:    "203.0.113.0/24",
		Comment: "blocked",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(runner.Commands()[0], " ")
	want := "ufw deny from 203.0.113.0/24 comment blocked"
	if got != want {
		t.Fatalf("got %q", got)
	}
}
