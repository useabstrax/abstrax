package cli

import (
	"testing"

	"abstrax/internal/services/project"
)

func TestValidateProjectSSLOptions(t *testing.T) {
	valid := project.AddOptions{
		SSL:       true,
		Domains:   []string{"example.com"},
		Email:     "admin@example.com",
		WebServer: project.WebServerNginx,
	}

	if err := validateProjectSSLOptions(valid); err != nil {
		t.Fatalf("valid options: %v", err)
	}

	cases := []struct {
		name string
		opts project.AddOptions
	}{
		{
			name: "missing domains",
			opts: func() project.AddOptions {
				o := valid
				o.Domains = nil
				return o
			}(),
		},
		{
			name: "missing email",
			opts: func() project.AddOptions {
				o := valid
				o.Email = ""
				return o
			}(),
		},
		{
			name: "no vhost",
			opts: func() project.AddOptions {
				o := valid
				o.NoVhost = true
				return o
			}(),
		},
		{
			name: "non-nginx web server",
			opts: func() project.AddOptions {
				o := valid
				o.WebServer = project.WebServerApache
				return o
			}(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateProjectSSLOptions(tc.opts); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
