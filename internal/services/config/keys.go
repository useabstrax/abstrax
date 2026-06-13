package config

import (
	"fmt"
	"strings"
)

const (
	keyPHPExtensions = "php.extensions"
)

// ParseKey validates a dot-notation config key.
func ParseKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	switch key {
	case keyPHPExtensions:
		return key, nil
	default:
		return "", fmt.Errorf("unknown config key %q", key)
	}
}
