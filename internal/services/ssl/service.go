// Package ssl manages SSL certificates using Certbot.
//
// Certbot only obtains and renews certificates (certonly with the nginx
// authenticator). TLS itself is rendered into the project's nginx vhost by the
// project package, so rewriting the vhost, for example in project modify, keeps
// HTTPS in place. Certificate metadata is stored on the project state.
package ssl

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	executil "abstrax/internal/exec"
	"abstrax/internal/services/project"
)

// nginxReloadHook makes certbot reload nginx after each renewal. certonly has
// no installer, so without a deploy hook nginx would keep serving the old cert.
const nginxReloadHook = "nginx -s reload"

// Service manages SSL certificates.
type Service struct {
	runner         *executil.Runner
	projectsSvc    *project.Service
	dryRun         bool
	letsEncryptDir string
	installed      func() bool
	now            func() time.Time
}

// New creates a Service.
func New(dryRun, verbose bool) *Service {
	projects := project.New(dryRun, verbose)
	return &Service{
		runner:         executil.New(dryRun, verbose),
		projectsSvc:    projects,
		dryRun:         dryRun,
		letsEncryptDir: projects.LetsEncryptDir(),
		installed:      Installed,
		now:            time.Now,
	}
}

// Add obtains a certificate for a project and renders HTTPS into its vhost.
func (s *Service) Add(ctx context.Context, opts AddOptions) error {
	if !s.installed() {
		return fmt.Errorf("certbot is not installed; install it with: %s", InstallCommand())
	}

	state, err := s.projectsSvc.Info(ctx, opts.ProjectName)
	if err != nil {
		return fmt.Errorf("project %q not found: %w", opts.ProjectName, err)
	}
	if state.WebServer != project.WebServerNginx || state.VhostPath == "" {
		return fmt.Errorf("project %q has no nginx vhost; SSL requires one", opts.ProjectName)
	}
	s.projectsSvc.AdoptSSL(state)

	domains := cleanDomains(opts.Domains)
	if len(domains) == 0 {
		domains = cleanDomains(state.Domains)
	}
	if len(domains) == 0 {
		return fmt.Errorf("no domains configured for project %q", opts.ProjectName)
	}

	if opts.Email == "" {
		return fmt.Errorf("--email is required for SSL certificate issuance")
	}

	certName := opts.CertName
	if certName == "" {
		certName = state.CertificateName()
	}
	if certName == "" {
		certName = project.StableCertName(state.Name)
	}
	if !project.ValidCertName(certName) {
		return fmt.Errorf("invalid certificate name %q", certName)
	}

	args := buildCertbotAddArgs(AddOptions{
		Email:    opts.Email,
		Domains:  domains,
		Staging:  opts.Staging,
		CertName: certName,
	})

	res, err := s.runner.Run(ctx, "certbot", args...)
	if err != nil {
		return certbotError(res, err)
	}
	if s.dryRun {
		return nil
	}

	now := s.now()
	next := *state
	next.SSLEnabled = true
	next.UpdatedAt = now
	next.SSL = &project.SSLState{
		CertificateName: certName,
		Domains:         domains,
		RedirectHTTP:    opts.RedirectHTTP,
		Staging:         opts.Staging,
		IssuedAt:        now,
	}
	if info, err := readCertificate(s.letsEncryptDir, certName); err == nil {
		next.SSL.ExpiresAt = info.NotAfter
	}

	if err := s.projectsSvc.RenderVhost(ctx, &next); err != nil {
		// The certificate exists but HTTPS is not live. Record why and leave
		// the previous vhost (restored by RenderVhost) and ssl_enabled alone.
		failed := *state
		failed.SSL = withLastError(state.SSL, next.SSL, err)
		_ = s.projectsSvc.SaveState(&failed)
		return fmt.Errorf("certificate %q issued but enabling HTTPS in nginx failed: %w", certName, err)
	}

	if err := s.projectsSvc.SaveState(&next); err != nil {
		return fmt.Errorf("updating project SSL state: %w", err)
	}
	return nil
}

// Remove removes the SSL certificate for a project.
//
// Order matters: HTTPS is taken out of the vhost and nginx is validated before
// the certificate is deleted, so nginx never points at a missing certificate.
func (s *Service) Remove(ctx context.Context, projectName string) error {
	state, err := s.projectsSvc.Info(ctx, projectName)
	if err != nil {
		return fmt.Errorf("project %q not found: %w", projectName, err)
	}
	s.projectsSvc.AdoptSSL(state)

	certName := state.CertificateName()
	if certName == "" {
		return fmt.Errorf("project %q has no SSL certificate", projectName)
	}
	if !project.ValidCertName(certName) {
		return fmt.Errorf("invalid certificate name %q", certName)
	}

	hasVhost := state.WebServer == project.WebServerNginx && state.VhostPath != ""
	if hasVhost && !s.dryRun {
		plain := *state
		plain.SSLEnabled = false
		if err := s.projectsSvc.RenderVhost(ctx, &plain); err != nil {
			return fmt.Errorf("removing HTTPS from nginx vhost: %w", err)
		}
	}

	if s.dryRun || s.certificateExists(certName) {
		res, err := s.runner.Run(ctx, "certbot", "delete",
			"--cert-name", certName,
			"--non-interactive")
		if err != nil {
			if hasVhost && !s.dryRun {
				// Certificate is still there; put HTTPS back so state and nginx agree.
				_ = s.projectsSvc.RenderVhost(ctx, state)
			}
			return certbotError(res, err)
		}
	}
	if s.dryRun {
		return nil
	}

	state.SSLEnabled = false
	state.SSL = nil
	state.UpdatedAt = s.now()
	if err := s.projectsSvc.SaveState(state); err != nil {
		return fmt.Errorf("updating project SSL state: %w", err)
	}
	return nil
}

// Renew renews certificates.
func (s *Service) Renew(ctx context.Context, opts RenewOptions) error {
	args := []string{"renew"}
	if opts.DryRun {
		args = append(args, "--dry-run")
	}

	var targets []*project.State
	if opts.Project != "" {
		state, err := s.projectsSvc.Info(ctx, opts.Project)
		if err != nil {
			return fmt.Errorf("project %q not found: %w", opts.Project, err)
		}
		s.projectsSvc.AdoptSSL(state)
		certName := state.CertificateName()
		if certName == "" {
			return fmt.Errorf("project %q has no SSL certificate", opts.Project)
		}
		args = append(args, "--cert-name", certName)
		targets = []*project.State{state}
	}

	res, err := s.runner.Run(ctx, "certbot", args...)
	if err != nil {
		return certbotError(res, err)
	}
	if opts.DryRun || s.dryRun {
		return nil
	}

	if _, err := s.runner.Run(ctx, "nginx", "-s", "reload"); err != nil {
		return fmt.Errorf("certificates renewed but nginx reload failed: %w", err)
	}

	if targets == nil {
		all, err := s.projectsSvc.List(ctx)
		if err != nil {
			return nil
		}
		for i := range all {
			st := all[i]
			targets = append(targets, &st)
		}
	}
	s.refreshExpiry(targets)
	return nil
}

// refreshExpiry stores the renewed certificate's expiry on each project state.
// It is best-effort: a state that cannot be updated does not fail the renewal.
func (s *Service) refreshExpiry(states []*project.State) {
	for _, st := range states {
		if !st.SSLEnabled && st.SSL == nil {
			continue
		}
		s.projectsSvc.AdoptSSL(st)
		name := st.CertificateName()
		if name == "" || st.SSL == nil {
			continue
		}
		info, err := readCertificate(s.letsEncryptDir, name)
		if err != nil {
			continue
		}
		if st.SSL.ExpiresAt.Equal(info.NotAfter) && st.SSL.LastError == "" {
			continue
		}
		ssl := *st.SSL
		ssl.ExpiresAt = info.NotAfter
		ssl.LastError = ""
		st.SSL = &ssl
		st.UpdatedAt = s.now()
		_ = s.projectsSvc.SaveState(st)
	}
}

// Status returns SSL certificate status for a project or all projects.
//
// Status comes from project state and the certificate files under
// /etc/letsencrypt/live. Human-readable `certbot certificates` output is only a
// fallback, for certificates that cannot be read and for certificates no
// project owns.
func (s *Service) Status(ctx context.Context, projectName string) ([]CertStatus, error) {
	var states []project.State
	if projectName != "" {
		st, err := s.projectsSvc.Info(ctx, projectName)
		if err != nil {
			return nil, fmt.Errorf("project %q not found: %w", projectName, err)
		}
		states = []project.State{*st}
	} else {
		all, err := s.projectsSvc.List(ctx)
		if err != nil {
			return nil, err
		}
		states = all
	}

	var scraped map[string]scrapedCert
	loadScraped := func() map[string]scrapedCert {
		if scraped != nil {
			return scraped
		}
		scraped = map[string]scrapedCert{}
		// certbot may be absent or need root; the file read is the primary source.
		if res, err := s.runner.RunSilent(ctx, "certbot", "certificates"); err == nil {
			for _, c := range parseCertbotCertificates(res.Stdout) {
				scraped[c.Name] = c
			}
		}
		return scraped
	}

	now := s.now()
	statuses := []CertStatus{}
	owned := map[string]bool{}

	for i := range states {
		st := &states[i]
		if !st.SSLEnabled && st.SSL == nil {
			continue
		}
		s.projectsSvc.AdoptSSL(st)
		certName := st.CertificateName()
		if certName == "" {
			continue
		}
		owned[certName] = true

		cs := CertStatus{
			ProjectName:     st.Name,
			CertificateName: certName,
			Domains:         append([]string(nil), st.Domains...),
		}
		if st.SSL != nil {
			cs.RedirectHTTP = st.SSL.RedirectHTTP
			cs.Staging = st.SSL.Staging
			if len(st.SSL.Domains) > 0 {
				cs.Domains = append([]string(nil), st.SSL.Domains...)
			}
		}

		var notAfter time.Time
		haveExpiry := false
		if info, err := readCertificate(s.letsEncryptDir, certName); err == nil {
			notAfter, haveExpiry = info.NotAfter, true
			if len(info.Domains) > 0 {
				cs.Domains = info.Domains
			}
		} else if c, ok := loadScraped()[certName]; ok && c.HasDate {
			notAfter, haveExpiry = c.NotAfter, true
			if len(c.Domains) > 0 {
				cs.Domains = c.Domains
			}
		}
		applyExpiry(&cs, now, notAfter, haveExpiry)
		statuses = append(statuses, cs)
	}

	if projectName != "" {
		return statuses, nil
	}

	// Certificates certbot knows about that no project claims.
	for _, c := range sortedCerts(loadScraped()) {
		if owned[c.Name] {
			continue
		}
		cs := CertStatus{CertificateName: c.Name, Domains: c.Domains}
		applyExpiry(&cs, now, c.NotAfter, c.HasDate)
		statuses = append(statuses, cs)
	}
	return statuses, nil
}

func applyExpiry(cs *CertStatus, now, notAfter time.Time, ok bool) {
	if !ok {
		cs.Status = StatusMissing
		return
	}
	status, days := classify(now, notAfter)
	cs.Status = status
	cs.Expiry = notAfter.UTC().Format(time.RFC3339)
	cs.DaysRemaining = &days
}

func sortedCerts(m map[string]scrapedCert) []scrapedCert {
	out := make([]scrapedCert, 0, len(m))
	for _, c := range m {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *Service) certificateExists(certName string) bool {
	_, err := os.Stat(filepath.Join(s.letsEncryptDir, "live", certName))
	return err == nil
}

func withLastError(previous, issued *project.SSLState, err error) *project.SSLState {
	base := issued
	if previous != nil {
		base = previous
	}
	out := *base
	out.LastError = err.Error()
	return &out
}

func certbotError(res executil.Result, err error) error {
	if res.Stderr != "" {
		return fmt.Errorf("certbot failed: %s", strings.TrimSpace(res.Stderr))
	}
	return fmt.Errorf("certbot failed: %w", err)
}

func cleanDomains(in []string) []string {
	var out []string
	for _, d := range in {
		if d = strings.TrimSpace(d); d != "" {
			out = append(out, d)
		}
	}
	return out
}

// buildCertbotAddArgs builds `certbot certonly --nginx` arguments. Certbot only
// authenticates and issues; Abstrax renders TLS into the vhost itself.
func buildCertbotAddArgs(opts AddOptions) []string {
	args := []string{
		"certonly",
		"--nginx",
		"--non-interactive",
		"--agree-tos",
		"--email", opts.Email,
		"--cert-name", opts.CertName,
		"--expand",
		"--deploy-hook", nginxReloadHook,
	}

	for _, d := range opts.Domains {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		args = append(args, "-d", d)
	}

	if opts.Staging {
		args = append(args, "--staging")
	}

	return args
}
