// Command abstrax-example is a reference Abstrax plugin for testing and documentation.
package main

import (
	"encoding/json"
	"fmt"
	"os"
)

const (
	protocolVersion = 1
	pluginName      = "example"
	pluginVersion   = "0.1.0"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "plugin":
		if len(os.Args) < 3 || os.Args[2] != "metadata" {
			fmt.Fprintf(os.Stderr, "usage: %s plugin metadata\n", os.Args[0])
			os.Exit(1)
		}
		printMetadata()
	case "hello":
		name := "world"
		if len(os.Args) > 2 {
			name = os.Args[2]
		}
		fmt.Printf("Hello, %s!\n", name)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printMetadata() {
	meta := map[string]any{
		"protocol_version": protocolVersion,
		"name":             pluginName,
		"display_name":     "Example Plugin",
		"description":      "An example Abstrax plugin",
		"version":          pluginVersion,
		"requires_abstrax": ">=0.1.0",
		"homepage":         "https://plugins.useabstrax.com/plugins/example",
		"commands": []map[string]string{
			{"name": "hello", "description": "Print a greeting"},
		},
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(meta)
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "usage: %s <command> [arguments]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "commands:\n")
	fmt.Fprintf(os.Stderr, "  plugin metadata   Print plugin metadata JSON\n")
	fmt.Fprintf(os.Stderr, "  hello [name]      Print a greeting\n")
}
