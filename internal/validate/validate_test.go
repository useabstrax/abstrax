package validate_test

import (
	"strings"
	"testing"

	"abstrax/internal/validate"
)

func TestUsername(t *testing.T) {
	cases := []struct {
		input string
		valid bool
	}{
		{"mike", true},
		{"mike_barlow", true},
		{"mike-barlow", true},
		{"_system", true},
		{"Mike", false},
		{"0mike", false},
		{"", false},
		{"this-username-is-way-too-long-to-be-a-valid-linux-name", false},
	}
	for _, c := range cases {
		err := validate.Username(c.input)
		if c.valid && err != nil {
			t.Errorf("Username(%q) expected valid, got error: %v", c.input, err)
		}
		if !c.valid && err == nil {
			t.Errorf("Username(%q) expected error, got nil", c.input)
		}
	}
}

func TestPort(t *testing.T) {
	if err := validate.Port(22); err != nil {
		t.Errorf("Port(22) should be valid: %v", err)
	}
	if err := validate.Port(0); err == nil {
		t.Error("Port(0) should be invalid")
	}
	if err := validate.Port(65536); err == nil {
		t.Error("Port(65536) should be invalid")
	}
	if err := validate.Port(65535); err != nil {
		t.Errorf("Port(65535) should be valid: %v", err)
	}
}

func TestCronExpression(t *testing.T) {
	valid := []string{
		"* * * * *",
		"0 3 * * *",
		"*/5 * * * *",
		"0 0 1 1 *",
		"0 9 * * 1-5",
		"0 0 * JAN SUN",
		"0 0 * jan mon",
		"30 4 1,15 * 0-7",
		"0-59/2 0-23/1 1-31/1 JAN-DEC SUN-SAT",
		"@reboot",
		"@yearly",
		"@annually",
		"@monthly",
		"@weekly",
		"@daily",
		"@midnight",
		"@hourly",
		"  @Daily  ",
	}
	for _, expr := range valid {
		if err := validate.CronExpression(expr); err != nil {
			t.Errorf("CronExpression(%q) should be valid: %v", expr, err)
		}
	}

	invalid := []struct {
		expr string
		want string
	}{
		{"* *", "5 fields"},
		{"99 * * * *", "minute: 99 is out of range (0-59)"},
		{"0 25 * * *", "hour: 25 is out of range (0-23)"},
		{"0 0 32 * *", "day: 32 is out of range (1-31)"},
		{"0 0 * 13 *", "month: 13 is out of range (1-12)"},
		{"0 0 * * 8", "weekday: 8 is out of range (0-7)"},
		{"10-2 * * * *", "minute: range 10-2 is reversed"},
		{"*/0 * * * *", `minute: step "0" must be 1 or more`},
		{"0 0 * FOO *", `month: invalid value "FOO"`},
		{"@weeklyish", `unknown cron macro "@weeklyish"`},
		{"@reboot extra", `unknown cron macro "@reboot extra"`},
	}
	for _, tt := range invalid {
		err := validate.CronExpression(tt.expr)
		if err == nil {
			t.Errorf("CronExpression(%q) should be invalid", tt.expr)
			continue
		}
		if !strings.Contains(err.Error(), tt.want) {
			t.Errorf("CronExpression(%q) = %q, want substring %q", tt.expr, err.Error(), tt.want)
		}
	}
}

func TestPluginName(t *testing.T) {
	valid := []string{"deploy", "backup", "my-plugin", "a"}
	for _, name := range valid {
		if err := validate.PluginName(name); err != nil {
			t.Errorf("PluginName(%q) should be valid: %v", name, err)
		}
	}
	invalid := []string{"", "Deploy", "my_plugin", "1bad"}
	for _, name := range invalid {
		if err := validate.PluginName(name); err == nil {
			t.Errorf("PluginName(%q) should be invalid", name)
		}
	}
}

func TestIPAddress(t *testing.T) {
	if err := validate.IPAddress("127.0.0.1"); err != nil {
		t.Errorf("127.0.0.1 should be valid: %v", err)
	}
	if err := validate.IPAddress("::1"); err != nil {
		t.Errorf("::1 should be valid: %v", err)
	}
	if err := validate.IPAddress("not-an-ip"); err == nil {
		t.Error("not-an-ip should be invalid")
	}
}

func TestCIDRRange(t *testing.T) {
	if err := validate.CIDRRange("192.168.0.0/24"); err != nil {
		t.Errorf("192.168.0.0/24 should be valid: %v", err)
	}
	if err := validate.CIDRRange("10.0.0.1"); err != nil {
		t.Errorf("plain IP should be valid CIDR: %v", err)
	}
	if err := validate.CIDRRange("bad-cidr"); err == nil {
		t.Error("bad-cidr should be invalid")
	}
}

func TestDomain(t *testing.T) {
	if err := validate.Domain("example.com"); err != nil {
		t.Errorf("example.com should be valid: %v", err)
	}
	if err := validate.Domain("sub.example.co.uk"); err != nil {
		t.Errorf("sub.example.co.uk should be valid: %v", err)
	}
	if err := validate.Domain(""); err == nil {
		t.Error("empty domain should be invalid")
	}
}

func TestCronID(t *testing.T) {
	if err := validate.CronID("backup-daily"); err != nil {
		t.Errorf("backup-daily should be valid: %v", err)
	}
	if err := validate.CronID(""); err == nil {
		t.Error("empty cron ID should be invalid")
	}
	if err := validate.CronID("has space"); err == nil {
		t.Error("cron ID with space should be invalid")
	}
}

func TestProjectName(t *testing.T) {
	cases := []struct {
		input string
		valid bool
	}{
		{"myapp", true},
		{"my-app", true},
		{"my_app", true},
		{"example.com", true},
		{"sub.example.co.uk", true},
		{"", false},
		{"has space", false},
		{"has/slash", false},
	}
	for _, c := range cases {
		err := validate.ProjectName(c.input)
		if c.valid && err != nil {
			t.Errorf("ProjectName(%q) expected valid, got error: %v", c.input, err)
		}
		if !c.valid && err == nil {
			t.Errorf("ProjectName(%q) expected error, got nil", c.input)
		}
	}
}

func TestDatabaseName(t *testing.T) {
	if err := validate.DatabaseName("myapp_db"); err != nil {
		t.Errorf("myapp_db should be valid: %v", err)
	}
	if err := validate.DatabaseName("my-app"); err == nil {
		t.Error("my-app should be invalid (hyphens not allowed)")
	}
	if err := validate.DatabaseName(""); err == nil {
		t.Error("empty db name should be invalid")
	}
}
