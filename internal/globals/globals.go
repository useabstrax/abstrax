// Package globals holds the parsed values of global CLI flags so they are
// accessible to service and output layers without threading cobra.Command
// through every call stack.
package globals

// Flags is populated by the root Cobra command's PersistentPreRun hook.
var Flags = &GlobalFlags{}

// GlobalFlags contains the parsed global flag values.
type GlobalFlags struct {
	JSON                bool
	JSONStream          bool
	DryRun              bool
	Yes                 bool
	Quiet               bool
	Verbose             bool
	NoColor             bool
	EnableRequiredRepos bool
	AllowBlockedPlugin  []string
}

// MachineOutput reports whether output should be machine-readable (--json or --json-stream).
func MachineOutput() bool {
	return Flags.JSON || Flags.JSONStream
}
