// Package main is the entry point for semantics-tool. With no subcommand or
// `serve`, it runs as an MCP (Model Context Protocol) stdio server. With any
// other first argument, it runs the corresponding CLI command.
//
// Usage:
//
//	semantics-tool serve [--file PATH]   # MCP server (default if first arg)
//	semantics-tool <command> [args]       # CLI one-shot
//
// The MCP server is the recommended interface for AI agents editing
// mappings.yaml: it preserves file formatting on round-trip, validates
// mutations before Save, and exposes a stable tool surface independent of
// the underlying YAML schema.
//
// The CLI is the recommended interface for humans and shell scripts.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/lenaxia/rathena-client/cmd/semantics-tool/cli"
	"github.com/lenaxia/rathena-client/cmd/semantics-tool/mcp"
)

func main() {
	args := os.Args[1:]

	// MCP server mode: explicit `serve`, or no args at all.
	if len(args) == 0 || args[0] == "serve" {
		mappingsPath := cli.DefaultMappingsPath
		rest := args
		if len(args) > 0 {
			rest = args[1:]
		}
		for i := 0; i+1 < len(rest); i++ {
			if rest[i] == "--file" || rest[i] == "-f" {
				mappingsPath = rest[i+1]
				break
			}
			if strings.HasPrefix(rest[i], "--file=") {
				mappingsPath = strings.TrimPrefix(rest[i], "--file=")
				break
			}
		}
		server := mcp.NewServer(mappingsPath)
		if err := server.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "semantics-tool serve: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := cli.Run(args, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "semantics-tool: %v\n", err)
		os.Exit(1)
	}
}
