// Package confirm provides interactive confirmation prompts for destructive
// commands.
package confirm

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Ask prompts the user for a yes/no confirmation and returns true if they
// confirm. If yes is true (from --yes flag) the prompt is skipped.
func Ask(prompt string, yes bool) (bool, error) {
	if yes {
		return true, nil
	}
	fmt.Printf("%s [y/N] ", prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("reading confirmation: %w", err)
	}
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes", nil
}

// MustAsk is like Ask but returns false (without an error) when the user
// declines, making it convenient for inline use.
func MustAsk(prompt string, yes bool) bool {
	ok, err := Ask(prompt, yes)
	if err != nil {
		return false
	}
	return ok
}
