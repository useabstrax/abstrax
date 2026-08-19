package plugin

import (
	"context"
	"fmt"
	"strings"

	"abstrax/internal/actions"
	"abstrax/internal/globals"
)

// PluginAction is a parsed plugin.<name>.<command> action.
type PluginAction struct {
	Plugin  string
	Command []string
}

// ParsePluginAction splits a 3+ segment plugin.* action.
// Two-segment names such as plugin.install are core CLI actions, not plugins.
func ParsePluginAction(action string) (PluginAction, bool) {
	action = strings.TrimSpace(action)
	parts := strings.Split(action, ".")
	if len(parts) < 3 || parts[0] != "plugin" {
		return PluginAction{}, false
	}
	for _, p := range parts {
		if p == "" {
			return PluginAction{}, false
		}
	}
	cmd := make([]string, 0, len(parts)-2)
	for _, p := range parts[2:] {
		cmd = append(cmd, strings.ReplaceAll(p, "_", "-"))
	}
	return PluginAction{Plugin: parts[1], Command: cmd}, true
}

// DispatchAction runs a plugin action: metadata check, then Dispatch with
// command + global flags + payload flags + payload args.
func (d *Dispatcher) DispatchAction(ctx context.Context, parsed PluginAction, payload []byte, g *globals.GlobalFlags, opts DispatchOptions) (int, error) {
	if parsed.Plugin == "" || len(parsed.Command) == 0 {
		return 1, fmt.Errorf("%w: invalid plugin action", actions.ErrUnknownAction)
	}

	binaryPath, err := d.discoverer.FindBinary(parsed.Plugin)
	if err != nil {
		return 1, err
	}
	meta, err := FetchMetadata(ctx, binaryPath)
	if err != nil {
		return 1, err
	}
	if err := ValidateMetadata(meta, parsed.Plugin); err != nil {
		return 1, err
	}
	if !meta.hasCommand(parsed.Command[0]) {
		return 1, fmt.Errorf("%w: %s %s", ErrUnknownPluginCommand, parsed.Plugin, parsed.Command[0])
	}

	flags, args, err := actions.PayloadToArgs(payload)
	if err != nil {
		return 1, err
	}

	pluginArgs := make([]string, 0, len(parsed.Command)+8+len(flags)+len(args))
	pluginArgs = append(pluginArgs, parsed.Command...)
	pluginArgs = append(pluginArgs, actions.GlobalArgs(g)...)
	pluginArgs = append(pluginArgs, flags...)
	pluginArgs = append(pluginArgs, args...)

	return d.Dispatch(ctx, parsed.Plugin, pluginArgs, opts)
}

func (m *Metadata) hasCommand(name string) bool {
	if m == nil || len(m.Commands) == 0 {
		return true
	}
	for _, c := range m.Commands {
		if c.Name == name {
			return true
		}
		if c.Action != "" {
			if parsed, ok := ParsePluginAction(c.Action); ok && len(parsed.Command) > 0 && parsed.Command[0] == name {
				return true
			}
		}
	}
	return false
}
