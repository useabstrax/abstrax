package project

// vhostConfig holds nginx virtual host generation inputs.
type vhostConfig struct {
	Name        string
	Path        string
	Domains     []string
	Port        int
	Runtime     Runtime
	PHPVersion  string
	NodeVersion string
	RubyVersion string
	PublicDir   string
	WebRoot     string
	ProxyPort   int
	NodePort    int
	PHPSocket   string
}

func vhostOptionsFromAdd(opts AddOptions, state *State, paths *ValidatedPaths) vhostConfig {
	cfg := vhostConfig{
		Name:        opts.Name,
		Path:        paths.ProjectPath,
		Domains:     opts.Domains,
		Port:        opts.Port,
		Runtime:     opts.Runtime,
		PHPVersion:  state.PHPVersion,
		NodeVersion: state.NodeVersion,
		RubyVersion: state.RubyVersion,
		PublicDir:   opts.PublicDir,
		WebRoot:     opts.WebRoot,
		ProxyPort:   opts.ProxyPort,
		NodePort:    opts.NodePort,
		PHPSocket:   phpSocketForState(state),
	}
	if paths != nil && paths.DocumentRoot != "" && opts.WebRoot == "" {
		// Document root already resolved in validated paths.
	}
	return cfg
}

func (s *State) vhostConfig() vhostConfig {
	return vhostConfig{
		Name:        s.Name,
		Path:        s.Path,
		Domains:     s.Domains,
		Runtime:     s.Runtime,
		PHPVersion:  s.PHPVersion,
		NodeVersion: s.NodeVersion,
		RubyVersion: s.RubyVersion,
		PublicDir:   s.PublicDir,
		ProxyPort:   s.ProxyPort,
		PHPSocket:   phpSocketForState(s),
	}
}
