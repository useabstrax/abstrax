package session

import "testing"

func TestIPFromSSHEnv(t *testing.T) {
	tests := []struct {
		name       string
		connection string
		client     string
		want       string
	}{
		{
			name:       "SSH_CONNECTION ipv4",
			connection: "203.0.113.5 54321 198.51.100.1 22",
			want:       "203.0.113.5",
		},
		{
			name:       "SSH_CONNECTION ipv6",
			connection: "2001:db8::1 54321 2001:db8::2 22",
			want:       "2001:db8::1",
		},
		{
			name:   "SSH_CLIENT ipv4",
			client: "203.0.113.5 54321 22",
			want:   "203.0.113.5",
		},
		{
			name:       "SSH_CONNECTION preferred over SSH_CLIENT",
			connection: "203.0.113.5 54321 198.51.100.1 22",
			client:     "198.51.100.99 12345 22",
			want:       "203.0.113.5",
		},
		{
			name:       "IPv4-mapped IPv6",
			connection: "::ffff:203.0.113.5 54321 198.51.100.1 22",
			want:       "203.0.113.5",
		},
		{
			name: "empty",
			want: "",
		},
		{
			name:       "invalid IP",
			connection: "not-an-ip 54321 198.51.100.1 22",
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ipFromSSHEnv(tt.connection, tt.client)
			if got != tt.want {
				t.Errorf("ipFromSSHEnv() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIPFromWhoLine(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{
			line: "mike     pts/0        2024-06-14 10:00 (203.0.113.5)",
			want: "203.0.113.5",
		},
		{
			line: "mike     pts/0        2024-06-14 10:00 (2001:db8::1)",
			want: "2001:db8::1",
		},
		{
			line: "mike     tty1         2024-06-14 10:00",
			want: "",
		},
		{
			line: "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := ipFromWhoLine(tt.line)
			if got != tt.want {
				t.Errorf("ipFromWhoLine(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}
