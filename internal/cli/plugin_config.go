package cli

import (
	"slices"

	"abstrax/internal/globals"
	"abstrax/internal/services/config"
)

func effectiveAllowBlocked() []string {
	allowed := slices.Clone(globals.Flags.AllowBlockedPlugin)
	cfg, err := config.New().Effective()
	if err == nil && cfg.Plugins != nil {
		for _, name := range cfg.Plugins.AllowBlocked {
			if !slices.Contains(allowed, name) {
				allowed = append(allowed, name)
			}
		}
	}
	return allowed
}
