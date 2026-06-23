package cache

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

const (
	redisConfigPathDefault     = "/etc/redis/redis.conf"
	memcachedConfigPathDefault = "/etc/memcached.conf"
)

var (
	redisConfigPath     = redisConfigPathDefault
	memcachedConfigPath = memcachedConfigPathDefault
)

func applyDriverConfig(opts InstallOptions) error {
	if opts.DryRun {
		return nil
	}

	switch opts.Driver {
	case DriverRedis:
		return applyRedisConfig(opts)
	case DriverMemcached:
		return applyMemcachedConfig(opts)
	default:
		return nil
	}
}

func applyRedisConfig(opts InstallOptions) error {
	if opts.Port == 0 && opts.Bind == "" && opts.Memory == "" {
		return nil
	}

	data, err := os.ReadFile(redisConfigPath)
	if err != nil {
		return fmt.Errorf("reading redis config: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	if opts.Port > 0 {
		lines = setConfigLine(lines, "port", strconv.Itoa(opts.Port))
	}
	if opts.Bind != "" {
		lines = setConfigLine(lines, "bind", opts.Bind)
	}
	if opts.Memory != "" {
		lines = setConfigLine(lines, "maxmemory", opts.Memory)
	}

	content := strings.Join(lines, "\n")
	if err := os.WriteFile(redisConfigPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing redis config: %w", err)
	}
	return nil
}

func applyMemcachedConfig(opts InstallOptions) error {
	if opts.Port == 0 && opts.Bind == "" && opts.Memory == "" {
		return nil
	}

	data, err := os.ReadFile(memcachedConfigPath)
	if err != nil {
		return fmt.Errorf("reading memcached config: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	updated := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "-") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "-p":
			if opts.Port > 0 {
				lines[i] = replaceFlagField(line, "-p", strconv.Itoa(opts.Port))
				updated = true
			}
		case "-l":
			if opts.Bind != "" {
				lines[i] = replaceFlagField(line, "-l", opts.Bind)
				updated = true
			}
		case "-m":
			if opts.Memory != "" {
				lines[i] = replaceFlagField(line, "-m", opts.Memory)
				updated = true
			}
		}
	}

	if !updated {
		var extras []string
		if opts.Memory != "" {
			extras = append(extras, "-m", opts.Memory)
		}
		if opts.Port > 0 {
			extras = append(extras, "-p", strconv.Itoa(opts.Port))
		}
		if opts.Bind != "" {
			extras = append(extras, "-l", opts.Bind)
		}
		if len(extras) > 0 {
			lines = append(lines, strings.Join(extras, " "))
		}
	}

	content := strings.Join(lines, "\n")
	if err := os.WriteFile(memcachedConfigPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing memcached config: %w", err)
	}
	return nil
}

func setConfigLine(lines []string, key, value string) []string {
	prefix := key + " "
	re := regexp.MustCompile(`^\s*` + regexp.QuoteMeta(key) + `\s+`)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if re.MatchString(trimmed) {
			lines[i] = prefix + value
			return lines
		}
	}
	return append(lines, prefix+value)
}

func replaceFlagField(line, flag, value string) string {
	fields := strings.Fields(line)
	for i := 0; i < len(fields)-1; i++ {
		if fields[i] == flag {
			fields[i+1] = value
			return strings.Join(fields, " ")
		}
	}
	return line
}
