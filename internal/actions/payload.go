package actions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"abstrax/internal/globals"
)

const payloadArgsKey = "args"

// PayloadToArgs converts a JSON object payload into CLI flags and positional
// arguments. The reserved key "args" (a string array) becomes positionals.
// Every other key becomes --key or --key=value.
func PayloadToArgs(raw json.RawMessage) (flags []string, args []string, err error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil, nil
	}

	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil, nil, fmt.Errorf("invalid payload: %w", err)
	}
	if obj == nil {
		return nil, nil, nil
	}

	if rawArgs, ok := obj[payloadArgsKey]; ok {
		args, err = decodeArgs(rawArgs)
		if err != nil {
			return nil, nil, err
		}
		delete(obj, payloadArgsKey)
	}

	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if strings.TrimSpace(key) == "" {
			return nil, nil, fmt.Errorf("invalid payload: empty flag name")
		}
		flagArgs, err := decodeFlag(key, obj[key])
		if err != nil {
			return nil, nil, err
		}
		flags = append(flags, flagArgs...)
	}
	return flags, args, nil
}

func decodeArgs(v any) ([]string, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("invalid payload: %q must be an array of strings", payloadArgsKey)
	}
	out := make([]string, 0, len(list))
	for i, item := range list {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("invalid payload: %q[%d] must be a string", payloadArgsKey, i)
		}
		out = append(out, s)
	}
	return out, nil
}

func decodeFlag(key string, v any) ([]string, error) {
	flag := "--" + key
	switch val := v.(type) {
	case nil:
		return nil, fmt.Errorf("invalid payload: %q must not be null", key)
	case bool:
		if !val {
			return nil, nil
		}
		return []string{flag}, nil
	case string:
		return []string{flag + "=" + val}, nil
	case json.Number:
		return []string{flag + "=" + val.String()}, nil
	case []any:
		out := make([]string, 0, len(val))
		for i, item := range val {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("invalid payload: %q[%d] must be a string", key, i)
			}
			out = append(out, flag+"="+s)
		}
		return out, nil
	case map[string]any:
		return nil, fmt.Errorf("invalid payload: %q must not be an object", key)
	default:
		return nil, fmt.Errorf("invalid payload: unsupported type for %q", key)
	}
}

// GlobalArgs returns global CLI flags to inject for --action dispatch.
// --yes is always included so agent runs are non-interactive.
func GlobalArgs(f *globals.GlobalFlags) []string {
	if f == nil {
		return []string{"--yes"}
	}
	out := make([]string, 0, 8)
	switch {
	case f.JSONStream:
		out = append(out, "--json-stream")
	case f.JSON:
		out = append(out, "--json")
	}
	out = append(out, "--yes")
	if f.DryRun {
		out = append(out, "--dry-run")
	}
	if f.Quiet {
		out = append(out, "--quiet")
	}
	if f.Verbose {
		out = append(out, "--verbose")
	}
	if f.NoColor {
		out = append(out, "--no-color")
	}
	if f.EnableRequiredRepos {
		out = append(out, "--enable-required-repos")
	}
	for _, name := range f.AllowBlockedPlugin {
		if strings.TrimSpace(name) == "" {
			continue
		}
		out = append(out, "--allow-blocked-plugin="+name)
	}
	return out
}
