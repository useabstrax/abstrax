package project

import "fmt"

// DaemonAddExample is a suggested abstrax daemon add command for proxy runtimes.
type DaemonAddExample struct {
	DaemonName string
	Command    string
	Directory  string
	User       string
	Port       int
	Runtime    Runtime
}

// DaemonAddExampleFor returns an example daemon configuration for Node.js and
// Ruby projects. Other runtimes return nil.
func DaemonAddExampleFor(state *State) *DaemonAddExample {
	if state == nil {
		return nil
	}
	switch state.Runtime {
	case RuntimeNode, RuntimeRuby:
	default:
		return nil
	}

	user := state.Owner
	if user == "" {
		user = SharedWebUser
	}

	return &DaemonAddExample{
		DaemonName: "abstrax-" + state.Name + "-web",
		Directory:  state.Path,
		User:       user,
		Port:       proxyPort(state.ProxyPort),
		Runtime:    state.Runtime,
	}
}

func (e *DaemonAddExample) appCommand() string {
	switch e.Runtime {
	case RuntimeNode:
		return "node app.js"
	case RuntimeRuby:
		return fmt.Sprintf("bundle exec puma -p %d -b 127.0.0.1", e.Port)
	default:
		return ""
	}
}

// FormatLines returns human-readable command lines suitable for terminal output.
func (e *DaemonAddExample) FormatLines() []string {
	if e == nil {
		return nil
	}

	lines := []string{
		fmt.Sprintf("sudo abstrax daemon add %s \\", e.DaemonName),
		fmt.Sprintf("  --command=%q \\", e.appCommand()),
		fmt.Sprintf("  --directory=%s \\", e.Directory),
	}
	if e.Runtime == RuntimeNode {
		lines = append(lines,
			fmt.Sprintf("  --user=%s \\", e.User),
			fmt.Sprintf("  --environment=PORT=%d", e.Port),
		)
	} else {
		lines = append(lines, fmt.Sprintf("  --user=%s", e.User))
	}
	return lines
}

func proxyPort(port int) int {
	if port == 0 {
		return 3000
	}
	return port
}
