package project

const (
	DefaultPHPVersion  = "8.5"
	DefaultNodeVersion = "24"
	DefaultRubyVersion = "4.0"
)

func normalizePHPVersion(v string) string {
	if v == "" {
		return DefaultPHPVersion
	}
	return v
}

func normalizeNodeVersion(v string) string {
	if v == "" {
		return DefaultNodeVersion
	}
	return v
}

func normalizeRubyVersion(v string) string {
	if v == "" {
		return DefaultRubyVersion
	}
	return v
}
