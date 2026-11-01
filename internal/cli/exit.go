package cli

import (
	"errors"
	"fmt"
)

// statusError is a finished command that should exit with code.
// The command has already reported its result. Execute uses the code and
// does not print this error.
type statusError struct {
	code int
}

func (e *statusError) Error() string {
	return fmt.Sprintf("exit status %d", e.code)
}

func statusCode(err error) (int, bool) {
	var se *statusError
	if errors.As(err, &se) && se != nil {
		return se.code, true
	}
	return 0, false
}
