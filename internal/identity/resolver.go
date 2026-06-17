// Package identity resolves Linux user and group account information.
package identity

import (
	"context"
	"fmt"
)

// Account holds resolved Linux account details for a user.
type Account struct {
	Username     string
	UID          int
	GID          int
	PrimaryGroup string
	Home         string
}

// HomeEntry maps a username to a resolved home directory path.
type HomeEntry struct {
	Username string
	Home     string
}

// Resolver looks up Linux account information.
type Resolver interface {
	Lookup(ctx context.Context, username string) (*Account, error)
	ListHomes(ctx context.Context) ([]HomeEntry, error)
}

// NotFoundError indicates the requested user does not exist.
type NotFoundError struct {
	Username string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("user %q does not exist; create the user first or omit --user to use shared www-data mode", e.Username)
}
