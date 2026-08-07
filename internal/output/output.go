// Package output provides human-readable and JSON output helpers used by all
// CLI commands.
package output

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/fatih/color"
)

// Result is the standard structured result returned by all service methods.
// It is rendered as human text or JSON depending on the --json / --json-stream flags.
type Result struct {
	Status    string      `json:"status"`
	Action    string      `json:"action"`
	Summary   string      `json:"summary,omitempty"`
	Message   string      `json:"message,omitempty"`
	ErrorCode string      `json:"error_code,omitempty"`
	Data      interface{} `json:"data,omitempty"`
}

// ProgressEvent is one NDJSON progress line under --json-stream.
type ProgressEvent struct {
	Type    string `json:"type"`
	Action  string `json:"action"`
	Step    string `json:"step"`
	Message string `json:"message"`
}

// streamResult wraps Result with a type discriminant for --json-stream.
type streamResult struct {
	Type      string      `json:"type"`
	Status    string      `json:"status"`
	Action    string      `json:"action"`
	Summary   string      `json:"summary,omitempty"`
	Message   string      `json:"message,omitempty"`
	ErrorCode string      `json:"error_code,omitempty"`
	Data      interface{} `json:"data,omitempty"`
}

// Success builds a successful result.
func Success(action, summary string, data interface{}) Result {
	return Result{
		Status:  "success",
		Action:  action,
		Summary: summary,
		Data:    data,
	}
}

// Failure builds a failure result.
func Failure(action, errorCode, message string) Result {
	return Result{
		Status:    "error",
		Action:    action,
		ErrorCode: errorCode,
		Message:   message,
	}
}

// Printer writes output to stdout/stderr, respecting global flags.
type Printer struct {
	jsonMode   bool
	jsonStream bool
	quiet      bool
	verbose    bool
	noColor    bool
}

// NewPrinter creates a Printer configured from the provided flags.
func NewPrinter(jsonMode, jsonStream, quiet, verbose, noColor bool) *Printer {
	if noColor {
		color.NoColor = true
	}
	return &Printer{
		jsonMode:   jsonMode,
		jsonStream: jsonStream,
		quiet:      quiet,
		verbose:    verbose,
		noColor:    noColor,
	}
}

func (p *Printer) machine() bool {
	return p.jsonMode || p.jsonStream
}

// Print renders a Result to stdout.
func (p *Printer) Print(r Result) {
	if p.jsonStream {
		PrintStreamResult(r)
		return
	}
	if p.jsonMode {
		PrintJSON(r)
		return
	}
	if r.Status == "error" {
		p.Error(r.Message)
		return
	}
	if r.Summary != "" {
		p.Success(r.Summary)
	}
}

// Progress emits one flushed NDJSON progress line when stream mode is on.
func (p *Printer) Progress(action, step, message string) {
	if !p.jsonStream {
		return
	}
	PrintProgress(action, step, message)
}

// PrintJSON encodes v as indented JSON to stdout (--json behaviour).
func PrintJSON(v interface{}) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "json encode error: %v\n", err)
	}
}

// PrintProgress writes one flushed NDJSON progress event to stdout.
func PrintProgress(action, step, message string) {
	writeNDJSON(ProgressEvent{
		Type:    "progress",
		Action:  action,
		Step:    step,
		Message: message,
	})
}

// PrintStreamResult writes one flushed NDJSON result event to stdout.
func PrintStreamResult(r Result) {
	writeNDJSON(streamResult{
		Type:      "result",
		Status:    r.Status,
		Action:    r.Action,
		Summary:   r.Summary,
		Message:   r.Message,
		ErrorCode: r.ErrorCode,
		Data:      r.Data,
	})
}

// WriteResult prints a final result for --json or --json-stream.
func WriteResult(r Result, stream bool) {
	if stream {
		PrintStreamResult(r)
		return
	}
	PrintJSON(r)
}

func writeNDJSON(v interface{}) {
	enc := json.NewEncoder(os.Stdout)
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "json encode error: %v\n", err)
		return
	}
	_ = os.Stdout.Sync()
}

// Info prints an informational message unless --quiet is set.
func (p *Printer) Info(format string, args ...interface{}) {
	if p.quiet {
		return
	}
	if p.machine() {
		return
	}
	fmt.Printf(format+"\n", args...)
}

// Success prints a green success message.
func (p *Printer) Success(format string, args ...interface{}) {
	if p.machine() {
		return
	}
	msg := fmt.Sprintf(format, args...)
	if p.noColor {
		fmt.Println(msg)
	} else {
		color.Green(msg)
	}
}

// Warn prints a yellow warning message.
func (p *Printer) Warn(format string, args ...interface{}) {
	if p.machine() {
		return
	}
	msg := fmt.Sprintf(format, args...)
	if p.noColor {
		fmt.Fprintln(os.Stderr, "WARNING: "+msg)
	} else {
		color.Yellow("WARNING: " + msg)
	}
}

// Error prints a red error message to stderr.
func (p *Printer) Error(format string, args ...interface{}) {
	if p.machine() {
		return
	}
	msg := fmt.Sprintf(format, args...)
	if p.noColor {
		fmt.Fprintln(os.Stderr, "ERROR: "+msg)
	} else {
		color.Red("ERROR: " + msg)
	}
}

// Verbose prints a message only when --verbose is set.
func (p *Printer) Verbose(format string, args ...interface{}) {
	if !p.verbose || p.machine() {
		return
	}
	fmt.Printf("[verbose] "+format+"\n", args...)
}

// DryRun prints a dry-run notice.
func (p *Printer) DryRun(format string, args ...interface{}) {
	if p.machine() {
		return
	}
	msg := fmt.Sprintf(format, args...)
	if p.noColor {
		fmt.Println("[dry-run] " + msg)
	} else {
		color.Cyan("[dry-run] " + msg)
	}
}

// Line prints a plain line regardless of quiet mode (useful for list output).
func (p *Printer) Line(format string, args ...interface{}) {
	if p.machine() {
		return
	}
	fmt.Printf(format+"\n", args...)
}
