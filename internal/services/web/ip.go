package web

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"abstrax/internal/platform"
)

const (
	// IPConfigName is the Abstrax-managed nginx file for the server IP response.
	// It is not named abstrax-<project>, so project remove does not delete it.
	IPConfigName = "abstrax-ip.conf"
	// IPStateFile stores the configured return code under /etc/abstrax.
	IPStateFile = "web-ip.json"
)

// AllowedIPReturnCodes are the status codes web ip set accepts.
var AllowedIPReturnCodes = []int{404, 403, 444}

// IPStatus describes the current server IP response configuration.
type IPStatus struct {
	Configured bool   `json:"configured"`
	Return     int    `json:"return,omitempty"`
	Active     bool   `json:"active"`
	Path       string `json:"path,omitempty"`
	Note       string `json:"note,omitempty"`
}

type ipState struct {
	Return int `json:"return"`
}

// ShowIP reports the configured response for requests to the server IP.
func (s *Service) ShowIP(_ context.Context) (*IPStatus, error) {
	st, err := s.loadIPState()
	if err != nil {
		return nil, err
	}
	path := s.ipConfigPath()
	status := &IPStatus{Path: path}
	if st == nil {
		status.Note = "unset; a named site may answer the server IP as nginx's implicit default"
		return status, nil
	}
	status.Configured = true
	status.Return = st.Return
	active, err := s.ipBlockHoldsDefaultServer()
	if err != nil {
		return nil, err
	}
	status.Active = active
	if active {
		status.Note = fmt.Sprintf("server IP returns %d", st.Return)
	} else {
		status.Note = "configured, but a project currently holds default_server for the server IP"
	}
	return status, nil
}

// SetIP writes the managed IP response block with the given return code.
func (s *Service) SetIP(ctx context.Context, code int) error {
	if !validIPReturn(code) {
		return fmt.Errorf("unsupported return code %d; allowed: %v", code, AllowedIPReturnCodes)
	}
	if !Installed("nginx") {
		return fmt.Errorf("nginx is not installed; install it first with: %s", InstallCommand("nginx"))
	}
	if err := s.saveIPState(ipState{Return: code}); err != nil {
		return err
	}
	holds, err := projectHoldsDefaultServer()
	if err != nil {
		return err
	}
	return s.writeIPConfig(ctx, code, !holds)
}

// ClearIP removes the managed IP response block.
func (s *Service) ClearIP(ctx context.Context) error {
	if err := s.removeIPState(); err != nil {
		return err
	}
	return s.removeIPConfig(ctx)
}

// SyncIPDefaultServer rewrites the IP block so only one listener holds
// default_server. When projectHoldsDefault is true, the IP block keeps its
// return code but drops default_server. When false and a response is set, the
// IP block takes default_server again.
func (s *Service) SyncIPDefaultServer(ctx context.Context, projectHoldsDefault bool) error {
	st, err := s.loadIPState()
	if err != nil || st == nil {
		return err
	}
	return s.writeIPConfig(ctx, st.Return, !projectHoldsDefault)
}

// SyncIPDefaultServer is a package-level helper for callers that do not hold a Service.
func SyncIPDefaultServer(ctx context.Context, dryRun, verbose bool, projectHoldsDefault bool) error {
	return New(dryRun, verbose).SyncIPDefaultServer(ctx, projectHoldsDefault)
}

func validIPReturn(code int) bool {
	for _, c := range AllowedIPReturnCodes {
		if c == code {
			return true
		}
	}
	return false
}

func (s *Service) ipStatePath() string {
	return filepath.Join(s.provider.Paths().AbstraxConfigDir, IPStateFile)
}

func (s *Service) ipConfigPath() string {
	if s.provider.NginxLayout() == platform.NginxConfD {
		return filepath.Join(s.provider.NginxConfigDir(), IPConfigName)
	}
	return filepath.Join(s.provider.NginxSitesAvailable(), strings.TrimSuffix(IPConfigName, ".conf"))
}

func (s *Service) loadIPState() (*ipState, error) {
	path := s.ipStatePath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var st ipState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if !validIPReturn(st.Return) {
		return nil, fmt.Errorf("invalid return code %d in %s", st.Return, path)
	}
	return &st, nil
}

func (s *Service) saveIPState(st ipState) error {
	if s.dryRun {
		fmt.Printf("[dry-run] would write %s\n", s.ipStatePath())
		return nil
	}
	dir := filepath.Dir(s.ipStatePath())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.ipStatePath(), append(data, '\n'), 0644)
}

func (s *Service) removeIPState() error {
	if s.dryRun {
		fmt.Printf("[dry-run] would remove %s\n", s.ipStatePath())
		return nil
	}
	if err := os.Remove(s.ipStatePath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *Service) writeIPConfig(ctx context.Context, code int, asDefault bool) error {
	content := renderIPConfig(code, asDefault)
	path := s.ipConfigPath()
	if s.dryRun {
		fmt.Printf("[dry-run] would write %s\n", path)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := s.enableIPConfig(); err != nil {
		return err
	}
	res, err := s.runner.RunSilent(ctx, "nginx", "-t")
	if err != nil || res.ExitCode != 0 {
		_ = s.removeIPConfigFiles()
		return fmt.Errorf("nginx config test failed: %s", res.Stderr+res.Stdout)
	}
	_, _ = s.runner.Run(ctx, "nginx", "-s", "reload")
	return nil
}

func renderIPConfig(code int, asDefault bool) string {
	def := ""
	if asDefault {
		def = " default_server"
	}
	var sb strings.Builder
	sb.WriteString("# Managed by Abstrax\n")
	sb.WriteString("server {\n")
	sb.WriteString(fmt.Sprintf("    listen 80%s;\n", def))
	sb.WriteString(fmt.Sprintf("    listen [::]:80%s;\n", def))
	sb.WriteString("    server_name _;\n")
	sb.WriteString(fmt.Sprintf("    return %d;\n", code))
	sb.WriteString("}\n\n")
	sb.WriteString("server {\n")
	sb.WriteString(fmt.Sprintf("    listen 443%s ssl;\n", def))
	sb.WriteString(fmt.Sprintf("    listen [::]:443%s ssl;\n", def))
	sb.WriteString("    server_name _;\n")
	sb.WriteString("    ssl_reject_handshake on;\n")
	sb.WriteString("}\n")
	return sb.String()
}

func (s *Service) enableIPConfig() error {
	if s.provider.NginxLayout() == platform.NginxConfD {
		return nil
	}
	avail := s.ipConfigPath()
	link := filepath.Join(s.provider.NginxSitesEnabled(), filepath.Base(avail))
	if _, err := os.Lstat(link); err == nil {
		return nil
	}
	return os.Symlink(avail, link)
}

func (s *Service) removeIPConfig(ctx context.Context) error {
	if s.dryRun {
		fmt.Printf("[dry-run] would remove %s\n", s.ipConfigPath())
		return nil
	}
	if err := s.removeIPConfigFiles(); err != nil {
		return err
	}
	res, err := s.runner.RunSilent(ctx, "nginx", "-t")
	if err != nil || res.ExitCode != 0 {
		return fmt.Errorf("nginx config test failed: %s", res.Stderr+res.Stdout)
	}
	_, _ = s.runner.Run(ctx, "nginx", "-s", "reload")
	return nil
}

func (s *Service) removeIPConfigFiles() error {
	path := s.ipConfigPath()
	_ = os.Remove(path)
	link := filepath.Join(s.provider.NginxSitesEnabled(), filepath.Base(path))
	_ = os.Remove(link)
	_ = os.Remove(path + ".disabled")
	return nil
}

func (s *Service) ipBlockHoldsDefaultServer() (bool, error) {
	path := s.ipConfigPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.Contains(string(data), "default_server"), nil
}

// projectHoldsDefaultServer is set by the project package when wired; default
// scans nothing and assumes no project holds it (IP block may take default).
var projectHoldsDefaultServer = func() (bool, error) { return false, nil }

// SetProjectDefaultServerCheck registers how web IP set detects a project that
// already owns default_server. The project package calls this from init.
func SetProjectDefaultServerCheck(fn func() (bool, error)) {
	if fn != nil {
		projectHoldsDefaultServer = fn
	}
}
