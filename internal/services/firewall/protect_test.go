package firewall

import "testing"

func TestHasSSHAllowRule(t *testing.T) {
	tests := []struct {
		name     string
		rules    []Rule
		clientIP string
		sshPort  int
		want     bool
	}{
		{
			name: "matching rule",
			rules: []Rule{
				{Action: "ALLOW", From: "203.0.113.5", Port: "22/tcp"},
			},
			clientIP: "203.0.113.5",
			sshPort:  22,
			want:     true,
		},
		{
			name: "wrong IP",
			rules: []Rule{
				{Action: "ALLOW", From: "198.51.100.1", Port: "22/tcp"},
			},
			clientIP: "203.0.113.5",
			sshPort:  22,
			want:     false,
		},
		{
			name: "wrong port",
			rules: []Rule{
				{Action: "ALLOW", From: "203.0.113.5", Port: "2222/tcp"},
			},
			clientIP: "203.0.113.5",
			sshPort:  22,
			want:     false,
		},
		{
			name: "deny rule ignored",
			rules: []Rule{
				{Action: "DENY", From: "203.0.113.5", Port: "22/tcp"},
			},
			clientIP: "203.0.113.5",
			sshPort:  22,
			want:     false,
		},
		{
			name:     "no rules",
			rules:    nil,
			clientIP: "203.0.113.5",
			sshPort:  22,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasSSHAllowRule(tt.rules, tt.clientIP, tt.sshPort)
			if got != tt.want {
				t.Errorf("hasSSHAllowRule() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseUFWRulesFromField(t *testing.T) {
	output := `[ 1] 22/tcp                     ALLOW IN    Anywhere
[ 2] 22/tcp                     ALLOW IN    203.0.113.5
[ 3] 443/tcp                    ALLOW IN    Anywhere`

	rules := parseUFWRules(output)
	if len(rules) != 3 {
		t.Fatalf("got %d rules, want 3", len(rules))
	}
	if rules[0].From != "" {
		t.Errorf("rule 0 From = %q, want empty", rules[0].From)
	}
	if rules[1].From != "203.0.113.5" {
		t.Errorf("rule 1 From = %q, want 203.0.113.5", rules[1].From)
	}
}
