package project

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// DefaultRedirectCode is used when a redirect is added without a status code.
const DefaultRedirectCode = 301

const maxRedirectFieldLen = 2048

// AddRedirectOptions holds options for adding a project redirect.
type AddRedirectOptions struct {
	Project     string
	Source      string
	Destination string
	// Code defaults to DefaultRedirectCode when zero.
	Code   int
	DryRun bool
}

// ModifyRedirectOptions holds options for changing a project redirect.
// Empty Source, empty Destination, and zero Code leave the stored value as is.
type ModifyRedirectOptions struct {
	Project     string
	ID          string
	Source      string
	Destination string
	Code        int
	DryRun      bool
}

// ListRedirects returns a project's redirects sorted by ID.
func (s *Service) ListRedirects(_ context.Context, project string) ([]Redirect, error) {
	state, err := s.loadState(project)
	if err != nil {
		return nil, fmt.Errorf("project %q not found", project)
	}
	out := append([]Redirect(nil), state.Redirects...)
	sortRedirects(out)
	return out, nil
}

// AddRedirect adds a redirect, rewrites the vhost, then saves state.
func (s *Service) AddRedirect(ctx context.Context, opts AddRedirectOptions) (*Redirect, error) {
	state, err := s.loadRedirectProject(opts.Project)
	if err != nil {
		return nil, err
	}
	if opts.Code == 0 {
		opts.Code = DefaultRedirectCode
	}
	r := Redirect{
		Source:      opts.Source,
		Destination: opts.Destination,
		Code:        opts.Code,
	}
	if err := validateRedirect(r); err != nil {
		return nil, err
	}
	if existing := findRedirectBySource(state.Redirects, r.Source); existing != nil {
		return nil, fmt.Errorf("a redirect for %s already exists (id %s)", r.Source, existing.ID)
	}
	r.ID = redirectID(r.Source)
	if findRedirectByID(state.Redirects, r.ID) != nil {
		return nil, fmt.Errorf("redirect id %s is already in use; choose a different source", r.ID)
	}

	next := append(append([]Redirect(nil), state.Redirects...), r)
	if err := s.applyRedirects(ctx, state, next, opts.DryRun); err != nil {
		return nil, err
	}
	return &r, nil
}

// ModifyRedirect changes an existing redirect. Its ID does not change.
func (s *Service) ModifyRedirect(ctx context.Context, opts ModifyRedirectOptions) (*Redirect, error) {
	state, err := s.loadRedirectProject(opts.Project)
	if err != nil {
		return nil, err
	}
	if opts.Source == "" && opts.Destination == "" && opts.Code == 0 {
		return nil, fmt.Errorf("nothing to change: pass --source, --destination, or --code")
	}

	idx := indexRedirectByID(state.Redirects, opts.ID)
	if idx < 0 {
		return nil, fmt.Errorf("redirect %q not found in project %q", opts.ID, opts.Project)
	}
	r := state.Redirects[idx]
	if opts.Source != "" {
		r.Source = opts.Source
	}
	if opts.Destination != "" {
		r.Destination = opts.Destination
	}
	if opts.Code != 0 {
		r.Code = opts.Code
	}
	if err := validateRedirect(r); err != nil {
		return nil, err
	}
	for i := range state.Redirects {
		if i != idx && state.Redirects[i].Source == r.Source {
			return nil, fmt.Errorf("a redirect for %s already exists (id %s)", r.Source, state.Redirects[i].ID)
		}
	}

	next := append([]Redirect(nil), state.Redirects...)
	next[idx] = r
	if err := s.applyRedirects(ctx, state, next, opts.DryRun); err != nil {
		return nil, err
	}
	return &r, nil
}

// RemoveRedirect removes a redirect by ID and returns it.
func (s *Service) RemoveRedirect(ctx context.Context, project, id string, dryRun bool) (*Redirect, error) {
	state, err := s.loadRedirectProject(project)
	if err != nil {
		return nil, err
	}
	idx := indexRedirectByID(state.Redirects, id)
	if idx < 0 {
		return nil, fmt.Errorf("redirect %q not found in project %q", id, project)
	}
	removed := state.Redirects[idx]
	next := append(append([]Redirect(nil), state.Redirects[:idx]...), state.Redirects[idx+1:]...)
	if err := s.applyRedirects(ctx, state, next, dryRun); err != nil {
		return nil, err
	}
	return &removed, nil
}

// loadRedirectProject loads a project that has a managed nginx vhost.
// Redirects live in that vhost, so a project without one cannot have them.
func (s *Service) loadRedirectProject(name string) (*State, error) {
	state, err := s.loadState(name)
	if err != nil {
		return nil, fmt.Errorf("project %q not found", name)
	}
	if state.WebServer != WebServerNginx {
		return nil, fmt.Errorf("project %q does not use nginx; redirects need a managed nginx vhost", name)
	}
	if state.VhostPath == "" {
		return nil, fmt.Errorf("project %q has no vhost; redirects need a managed nginx vhost", name)
	}
	return state, nil
}

// applyRedirects writes the vhost with next, then saves state. State is only
// saved after the vhost was written and nginx accepted it.
func (s *Service) applyRedirects(ctx context.Context, state *State, next []Redirect, dryRun bool) error {
	sortRedirects(next)
	updated := *state
	updated.Redirects = next
	updated.UpdatedAt = time.Now()

	if dryRun || s.dryRun {
		// Render to catch invalid state, but touch nothing on disk.
		_, err := s.renderNginxConfig(updated.vhostConfig())
		return err
	}

	if _, err := s.createNginxVhost(ctx, updated.vhostConfig()); err != nil {
		return err
	}
	if err := s.saveState(&updated); err != nil {
		return err
	}
	*state = updated
	return nil
}

// redirectID is the first 8 hex characters of SHA-256 over the source path.
func redirectID(source string) string {
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:])[:8]
}

func sortRedirects(r []Redirect) {
	sort.SliceStable(r, func(i, j int) bool { return r[i].ID < r[j].ID })
}

func findRedirectByID(rs []Redirect, id string) *Redirect {
	if i := indexRedirectByID(rs, id); i >= 0 {
		return &rs[i]
	}
	return nil
}

func indexRedirectByID(rs []Redirect, id string) int {
	for i := range rs {
		if rs[i].ID == id {
			return i
		}
	}
	return -1
}

func findRedirectBySource(rs []Redirect, source string) *Redirect {
	for i := range rs {
		if rs[i].Source == source {
			return &rs[i]
		}
	}
	return nil
}

// renderRedirects returns nginx location blocks for redirects, sorted by ID.
// Every redirect is validated again so a hand-edited state file cannot inject
// nginx directives.
func renderRedirects(redirects []Redirect) (string, error) {
	if len(redirects) == 0 {
		return "", nil
	}
	sorted := append([]Redirect(nil), redirects...)
	sortRedirects(sorted)
	var sb strings.Builder
	for _, r := range sorted {
		if err := validateRedirect(r); err != nil {
			return "", fmt.Errorf("redirect %s: %w", r.ID, err)
		}
		sb.WriteString(fmt.Sprintf("    location = %s {\n", r.Source))
		sb.WriteString(fmt.Sprintf("        return %d %s;\n", r.Code, r.Destination))
		sb.WriteString("    }\n\n")
	}
	return sb.String(), nil
}

func validRedirectCode(code int) bool {
	switch code {
	case 301, 302, 307, 308:
		return true
	}
	return false
}

func validateRedirect(r Redirect) error {
	if err := validateRedirectSource(r.Source); err != nil {
		return err
	}
	if err := validateRedirectDestination(r.Destination); err != nil {
		return err
	}
	if !validRedirectCode(r.Code) {
		return fmt.Errorf("invalid redirect code %d: use 301, 302, 307, or 308", r.Code)
	}
	return nil
}

// checkRedirectChars rejects characters that could end or alter an nginx
// directive: whitespace, control characters, non-ASCII, quotes, braces,
// semicolons, backslashes, and $ (nginx variable expansion).
func checkRedirectChars(field, value string) error {
	if value == "" {
		return fmt.Errorf("redirect %s is required", field)
	}
	if len(value) > maxRedirectFieldLen {
		return fmt.Errorf("redirect %s is longer than %d characters", field, maxRedirectFieldLen)
	}
	for _, c := range value {
		switch {
		case c <= ' ' || c >= 0x7f:
			return fmt.Errorf("redirect %s must not contain whitespace, control, or non-ASCII characters", field)
		case strings.ContainsRune(";{}\"'`\\$", c):
			return fmt.Errorf("redirect %s must not contain %q", field, string(c))
		}
	}
	return nil
}

func validateRedirectSource(source string) error {
	if err := checkRedirectChars("source", source); err != nil {
		return err
	}
	if !strings.HasPrefix(source, "/") {
		return fmt.Errorf("redirect source %q must start with /", source)
	}
	// nginx matches locations against the path only, never the query or fragment.
	if strings.ContainsAny(source, "?#") {
		return fmt.Errorf("redirect source %q must be a path without ? or #", source)
	}
	return nil
}

func validateRedirectDestination(dest string) error {
	if err := checkRedirectChars("destination", dest); err != nil {
		return err
	}
	if strings.HasPrefix(dest, "/") {
		if strings.HasPrefix(dest, "//") {
			return fmt.Errorf("redirect destination %q must be a path or an http(s) URL, not a scheme-relative URL", dest)
		}
		return nil
	}
	u, err := url.Parse(dest)
	if err != nil {
		return fmt.Errorf("invalid redirect destination %q: %w", dest, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("redirect destination %q must start with / or be an http(s) URL", dest)
	}
	if u.Hostname() == "" {
		return fmt.Errorf("redirect destination %q has no host", dest)
	}
	if u.User != nil {
		return fmt.Errorf("redirect destination %q must not contain credentials", dest)
	}
	return nil
}
