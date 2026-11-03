package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	executil "abstrax/internal/exec"
	"abstrax/internal/platform"
)

// DefaultLetsEncryptDir is where certbot keeps certificates and nginx TLS defaults.
const DefaultLetsEncryptDir = "/etc/letsencrypt"

// CertNamePrefix prefixes the certbot lineage name Abstrax creates for a project.
const CertNamePrefix = "abstrax-"

var (
	certNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

	sslCertPathRe = regexp.MustCompile(`(?m)^\s*ssl_certificate\s+([^;\s]+)\s*;`)
	redirectRe    = regexp.MustCompile(`(?m)^\s*return\s+30[1278]\s+https://`)
)

// StableCertName returns the certbot certificate name Abstrax uses for a project.
func StableCertName(projectName string) string {
	return CertNamePrefix + projectName
}

// ValidCertName reports whether name is safe to use as a certbot lineage name
// and as a path segment under /etc/letsencrypt/live.
func ValidCertName(name string) bool {
	return certNameRe.MatchString(name) && !strings.Contains(name, "..")
}

// CertificateName returns the certbot lineage name for the project.
// It prefers the stored name. A project that only has ssl_enabled set falls
// back to its first domain, which was certbot's default name before Abstrax
// passed --cert-name. It returns "" when SSL is not enabled for the project.
func (s *State) CertificateName() string {
	if s == nil {
		return ""
	}
	if s.SSL != nil && s.SSL.CertificateName != "" {
		return s.SSL.CertificateName
	}
	if s.SSLEnabled && len(s.Domains) > 0 {
		return s.Domains[0]
	}
	return ""
}

// EffectiveSSL returns the SSL settings to render into the vhost, or nil when
// the project does not terminate TLS. A legacy project (ssl_enabled with no ssl
// object) gets a synthesized value that redirects HTTP and uses Domains[0].
func (s *State) EffectiveSSL() *SSLState {
	if s == nil || !s.SSLEnabled {
		return nil
	}
	if s.SSL != nil && s.SSL.CertificateName != "" {
		ssl := *s.SSL
		return &ssl
	}
	name := s.CertificateName()
	if name == "" {
		return nil
	}
	ssl := SSLState{CertificateName: name, RedirectHTTP: true, Domains: append([]string(nil), s.Domains...)}
	if s.SSL != nil {
		ssl.Domains = s.SSL.Domains
		ssl.RedirectHTTP = s.SSL.RedirectHTTP
		ssl.Staging = s.SSL.Staging
	}
	return &ssl
}

// DiscoverSSLFromVhost reads an existing vhost and returns the certificate name
// from its ssl_certificate path (…/live/<name>/…) and whether it redirects HTTP
// to HTTPS. found is false when the vhost has no Let's Encrypt certificate path.
func DiscoverSSLFromVhost(vhostPath string) (certName string, redirect bool, found bool) {
	if vhostPath == "" {
		return "", false, false
	}
	data, err := os.ReadFile(vhostPath)
	if err != nil {
		return "", false, false
	}
	conf := string(data)
	redirect = redirectRe.MatchString(conf)
	for _, m := range sslCertPathRe.FindAllStringSubmatch(conf, -1) {
		path := filepath.ToSlash(m[1])
		parts := strings.Split(path, "/")
		for i := 0; i+2 < len(parts); i++ {
			if parts[i] == "live" && ValidCertName(parts[i+1]) {
				return parts[i+1], redirect, true
			}
		}
	}
	return "", redirect, false
}

// AdoptSSL fills state.SSL for a project that was set up before Abstrax stored
// certificate metadata. It tries the existing vhost's ssl_certificate path, then
// the first domain. It reports whether state changed. Callers persist the result.
func (s *Service) AdoptSSL(state *State) bool {
	if state == nil || !state.SSLEnabled {
		return false
	}
	if state.SSL != nil && state.SSL.CertificateName != "" {
		return false
	}

	name, redirect, found := DiscoverSSLFromVhost(state.VhostPath)
	if !found {
		if len(state.Domains) == 0 {
			return false
		}
		name = state.Domains[0]
		redirect = true
	}

	ssl := SSLState{}
	if state.SSL != nil {
		ssl = *state.SSL
	}
	ssl.CertificateName = name
	if state.SSL == nil {
		ssl.RedirectHTTP = redirect
		ssl.Domains = append([]string(nil), state.Domains...)
	}
	state.SSL = &ssl
	state.UpdatedAt = time.Now()
	return true
}

// LetsEncryptDir returns the certbot config directory this service reads.
func (s *Service) LetsEncryptDir() string {
	if s != nil && s.letsEncryptDir != "" {
		return s.letsEncryptDir
	}
	return DefaultLetsEncryptDir
}

// LayoutOverride replaces filesystem locations the service uses. Tests set it;
// empty fields keep the platform defaults.
type LayoutOverride struct {
	StateDir       string
	NginxAvailable string
	NginxEnabled   string
	NginxLogDir    string
	LetsEncryptDir string
}

// OverrideLayout applies o. A non-empty NginxAvailable selects the
// sites-available/sites-enabled layout.
func (s *Service) OverrideLayout(o LayoutOverride) {
	if o.StateDir != "" {
		s.stateDir = o.StateDir
	}
	if o.NginxAvailable != "" {
		s.nginxLayout = platform.NginxSitesAvailableEnabled
		s.nginxAvail = o.NginxAvailable
	}
	if o.NginxEnabled != "" {
		s.nginxEnabled = o.NginxEnabled
	}
	if o.NginxLogDir != "" {
		s.nginxLogDir = o.NginxLogDir
	}
	if o.LetsEncryptDir != "" {
		s.letsEncryptDir = o.LetsEncryptDir
	}
}

// SetRunner replaces the command runner (for tests).
func (s *Service) SetRunner(r *executil.Runner) {
	s.runner = r
}

// RenderVhost rewrites the project's nginx vhost from its state, including TLS
// when the project has SSL enabled. It validates with nginx -t and reloads. On
// failure the previous vhost is restored. A disabled vhost stays disabled. Projects without an nginx vhost are
// left alone.
func (s *Service) RenderVhost(ctx context.Context, state *State) error {
	if state == nil {
		return fmt.Errorf("project state is required")
	}
	if state.WebServer != WebServerNginx || state.VhostPath == "" {
		return nil
	}
	// A vhost turned off with Disable stays off when it is rewritten.
	vhostPath, err := s.writeNginxVhost(ctx, state.vhostConfig(), state.VhostDisabled)
	if err != nil {
		return err
	}
	state.VhostPath = vhostPath
	return nil
}

// tlsAssets fills the optional certbot-provided TLS includes if they exist.
func (s *Service) tlsAssets(opts *vhostConfig) {
	if opts.SSL == nil {
		return
	}
	if opts.LetsEncryptDir == "" {
		opts.LetsEncryptDir = s.LetsEncryptDir()
	}
	options := filepath.Join(opts.LetsEncryptDir, "options-ssl-nginx.conf")
	if opts.SSLOptionsPath == "" && fileExists(options) {
		opts.SSLOptionsPath = options
	}
	dh := filepath.Join(opts.LetsEncryptDir, "ssl-dhparams.pem")
	if opts.SSLDHParamPath == "" && fileExists(dh) {
		opts.SSLDHParamPath = dh
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// writeTLSDirectives appends certificate and protocol settings for an HTTPS server.
func writeTLSDirectives(sb *strings.Builder, opts vhostConfig) {
	dir := opts.LetsEncryptDir
	if dir == "" {
		dir = DefaultLetsEncryptDir
	}
	live := filepath.ToSlash(filepath.Join(dir, "live", opts.SSL.CertificateName))
	fmt.Fprintf(sb, "    ssl_certificate %s/fullchain.pem;\n", live)
	fmt.Fprintf(sb, "    ssl_certificate_key %s/privkey.pem;\n", live)
	if opts.SSLOptionsPath != "" {
		fmt.Fprintf(sb, "    include %s;\n", filepath.ToSlash(opts.SSLOptionsPath))
	} else {
		sb.WriteString("    ssl_protocols TLSv1.2 TLSv1.3;\n")
		sb.WriteString("    ssl_prefer_server_ciphers off;\n")
	}
	if opts.SSLDHParamPath != "" {
		fmt.Fprintf(sb, "    ssl_dhparam %s;\n", filepath.ToSlash(opts.SSLDHParamPath))
	}
}
