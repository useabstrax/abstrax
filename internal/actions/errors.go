package actions

import "errors"

var (
	// ErrUnknownAction indicates --action did not match a builtin or plugin command.
	ErrUnknownAction = errors.New("unknown action")
)
