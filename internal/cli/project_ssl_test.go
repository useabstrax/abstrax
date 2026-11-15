package cli

import (
	"strings"
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

	if err := validateProjectSSLOptions(valid, true); err != nil {
		t.Fatalf("valid options: %v", err)
	}

	cases := []struct {
		name      string
		opts      project.AddOptions
		nginxFlag bool
		want      string
	}{
		{
			name: "missing nginx flag",
			opts: valid,
			want: "--ssl requires --nginx",
		},
		{
			name:      "missing domains",
			nginxFlag: true,
			opts: func() project.AddOptions {
				o := valid
				o.Domains = nil
				return o
			}(),
			want: "--domains is required",
		},
		{
			name:      "missing email",
			nginxFlag: true,
			opts: func() project.AddOptions {
				o := valid
				o.Email = ""
				return o
			}(),
			want: "--email is required",
		},
		{
			name:      "no vhost",
			nginxFlag: true,
			opts: func() project.AddOptions {
				o := valid
				o.NoVhost = true
				return o
			}(),
			want: "--no-vhost",
		},
		{
			name:      "non-nginx web server",
			nginxFlag: true,
			opts: func() project.AddOptions {
				o := valid
				o.WebServer = project.WebServerApache
				return o
			}(),
			want: "--nginx",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateProjectSSLOptions(tc.opts, tc.nginxFlag)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q should contain %q", err, tc.want)
			}
		})
	}
}
