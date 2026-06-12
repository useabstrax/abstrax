package validate_test

import (
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
	if err := validate.CronExpression("* * * * *"); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
	if err := validate.CronExpression("0 3 * * *"); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
	if err := validate.CronExpression("* *"); err == nil {
		t.Error("expected error for 2-field expression")
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
