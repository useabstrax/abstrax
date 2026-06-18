// Package random provides cryptographically secure random value generation.
package random

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

const defaultPasswordLength = 24

// passwordChars is a MySQL-safe alphabet (no quotes or backslashes).
const passwordChars = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!@#$%&*+-=?"

// Password generates a cryptographically secure random password.
func Password() (string, error) {
	return PasswordN(defaultPasswordLength)
}

// PasswordN generates a cryptographically secure random password of length n.
func PasswordN(n int) (string, error) {
	if n <= 0 {
		return "", fmt.Errorf("password length must be positive")
	}

	max := big.NewInt(int64(len(passwordChars)))
	buf := make([]byte, n)
	for i := range buf {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("generating password: %w", err)
		}
		buf[i] = passwordChars[idx.Int64()]
	}
	return string(buf), nil
}
