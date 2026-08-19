package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"abstrax/internal/actions"
	"abstrax/internal/globals"
	"abstrax/internal/services/plugin"
)

var actionFlags struct {
	Action  string
	Payload string
}

func runAction(rootArgs []string) error {
	action := strings.TrimSpace(actionFlags.Action)
	if action == "" {
		return nil
	}
	if len(rootArgs) > 0 {
		return fmt.Errorf("unexpected arguments with --action")
	}

	payload, err := readPayload(actionFlags.Payload)
	if err != nil {
		return err
	}

	g := *globals.Flags
	if parsed, ok := plugin.ParsePluginAction(action); ok {
		return runPluginAction(parsed, payload, &g)
	}
	return runBuiltinAction(action, payload, &g)
}

func readPayload(raw string) (json.RawMessage, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return json.RawMessage(`{}`), nil
	}
	if s == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("reading payload from stdin: %w", err)
		}
		data = bytes.TrimSpace(data)
		if len(data) == 0 {
			return json.RawMessage(`{}`), nil
		}
		if !json.Valid(data) {
			return nil, fmt.Errorf("invalid payload: not JSON")
		}
		return json.RawMessage(data), nil
	}
	if !json.Valid([]byte(s)) {
		return nil, fmt.Errorf("invalid payload: not JSON")
	}
	return json.RawMessage(s), nil
}

func runPluginAction(parsed plugin.PluginAction, payload json.RawMessage, g *globals.GlobalFlags) error {
	svc, err := pluginService()
	if err != nil {
		return err
	}
	dispatcher, err := svc.NewDispatcher()
	if err != nil {
		return err
	}
	_, err = dispatcher.DispatchAction(context.Background(), parsed, payload, g, plugin.DispatchOptions{
		AllowBlocked: effectiveAllowBlocked(),
	})
	return err
}

func runBuiltinAction(action string, payload json.RawMessage, g *globals.GlobalFlags) error {
	path, ok := actions.CommandPath(action)
	if !ok {
		return fmt.Errorf("%w: %s", actions.ErrUnknownAction, action)
	}
	flags, args, err := actions.PayloadToArgs(payload)
	if err != nil {
		return err
	}

	innerArgs := make([]string, 0, len(path)+8+len(flags)+len(args))
	innerArgs = append(innerArgs, actions.GlobalArgs(g)...)
	innerArgs = append(innerArgs, path...)
	innerArgs = append(innerArgs, flags...)
	innerArgs = append(innerArgs, args...)

	globals.Flags = &globals.GlobalFlags{}
	root := NewRootCmd()
	root.SetArgs(innerArgs)
	return root.Execute()
}
