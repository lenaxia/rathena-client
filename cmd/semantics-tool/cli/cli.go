// Package cli implements the human-facing CLI for semantics-tool. It mirrors
// the MCP server's tool surface but with positional/flag arguments suitable
// for shell use. Built on the stdlib `flag` package — no cobra/yaml.
//
// Commands map 1:1 to MCP tools:
//
//	semantics-tool list-actions
//	semantics-tool get-action <name>
//	semantics-tool list-implementations <name>
//	semantics-tool get-implementation <action> <packet_id>
//	semantics-tool search [--name X] [--struct X] [--packet X] [--openkore X] [--desc X]
//	semantics-tool validate
//	semantics-tool stats
//	semantics-tool export
//	semantics-tool create-action <name> [--description X] [--openkore X]
//	semantics-tool update-action <name> [--description X] [--openkore X]
//	semantics-tool delete-action <name>
//	semantics-tool rename-action <oldName> <newName>
//	semantics-tool add-implementation <action> -id 0xXXXX -struct NAME [-min N] [-max N]
//	semantics-tool update-implementation <action> -id 0xXXXX [-struct NAME] [-min N] [-max N]
//	semantics-tool delete-implementation <action> -id 0xXXXX
//
// Each mutating command loads → mutates → Saves in one transaction.
package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/lenaxia/rathena-client/cmd/semantics-tool/mcp"
	"github.com/lenaxia/rathena-client/internal/semanticsdb"
)

// DefaultMappingsPath is the relative path from the repo root to mappings.yaml.
const DefaultMappingsPath = "semantics/mappings.yaml"

// Run parses argv and dispatches. Returns an error suitable for exit code 1.
// argv0 (the subcommand name) is excluded from args.
func Run(args []string, out io.Writer) error {
	if len(args) == 0 {
		printUsage(out)
		return fmt.Errorf("no command given")
	}

	// Peel off the global --file/-f flag if it comes before the subcommand.
	// Accept both `--file=path cmd` and `cmd --file=path` forms.
	mappingsPath := DefaultMappingsPath
	if args[0] == "--file" || args[0] == "-f" {
		if len(args) < 2 {
			return fmt.Errorf("--file requires a path argument")
		}
		mappingsPath = args[1]
		args = args[2:]
		if len(args) == 0 {
			printUsage(out)
			return fmt.Errorf("no command given")
		}
	}

	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "help", "-h", "--help":
		printUsage(out)
		return nil

	case "list-actions":
		return withDB(mappingsPath, false, out, func(db *semanticsdb.DB) error {
			return printJSON(out, map[string]any{"actions": db.ListActions()})
		})

	case "get-action":
		return withDB(mappingsPath, false, out, func(db *semanticsdb.DB) error {
			name, err := positional(rest, 0, "action name")
			if err != nil {
				return err
			}
			a, ok := db.GetAction(name)
			if !ok {
				return fmt.Errorf("action %q not found", name)
			}
			return printJSON(out, a)
		})

	case "list-implementations":
		return withDB(mappingsPath, false, out, func(db *semanticsdb.DB) error {
			name, err := positional(rest, 0, "action name")
			if err != nil {
				return err
			}
			a, ok := db.GetAction(name)
			if !ok {
				return fmt.Errorf("action %q not found", name)
			}
			return printJSON(out, a.Implementations)
		})

	case "get-implementation":
		return withDB(mappingsPath, false, out, func(db *semanticsdb.DB) error {
			action, err := positional(rest, 0, "action name")
			if err != nil {
				return err
			}
			pid, err := positional(rest, 1, "packet_id")
			if err != nil {
				return err
			}
			impl, ok := db.GetImplementation(action, pid)
			if !ok {
				return fmt.Errorf("implementation %s not found on action %q", pid, action)
			}
			return printJSON(out, impl)
		})

	case "search":
		fs := flag.NewFlagSet("search", flag.ContinueOnError)
		fs.SetOutput(out)
		name := fs.String("name", "", "substring of action name")
		structName := fs.String("struct", "", "substring of rAthena struct name")
		packetID := fs.String("packet", "", "exact packet_id")
		openkore := fs.String("openkore", "", "substring of openkore_name")
		desc := fs.String("desc", "", "substring of description")
		limit := fs.Int("limit", 0, "max results (0=unlimited)")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		return withDB(mappingsPath, false, out, func(db *semanticsdb.DB) error {
			results := db.Search(semanticsdb.SearchQuery{
				Name: *name, StructName: *structName, PacketID: *packetID,
				OpenkoreName: *openkore, Description: *desc,
			}, *limit)
			return printJSON(out, results)
		})

	case "validate":
		return withDB(mappingsPath, false, out, func(db *semanticsdb.DB) error {
			errs := db.Validate()
			if len(errs) == 0 {
				fmt.Fprintln(out, "OK: no validation errors")
				return nil
			}
			fmt.Fprintln(out, semanticsdb.FormatErrors(errs))
			return nil
		})

	case "stats":
		return withDB(mappingsPath, false, out, func(db *semanticsdb.DB) error {
			return printJSON(out, db.Statistics())
		})

	case "export":
		return withDB(mappingsPath, false, out, func(db *semanticsdb.DB) error {
			var actions []semanticsdb.Action
			for _, name := range db.ListActions() {
				a, _ := db.GetAction(name)
				actions = append(actions, a)
			}
			return printJSON(out, map[string]any{
				"actions": actions,
				"count":   len(actions),
			})
		})

	case "create-action":
		fs := flag.NewFlagSet("create-action", flag.ContinueOnError)
		fs.SetOutput(out)
		desc := fs.String("description", "", "human-readable description")
		openkore := fs.String("openkore", "", "OpenKore packet name")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		name, err := positional(fs.Args(), 0, "action name")
		if err != nil {
			return err
		}
		return withDB(mappingsPath, true, out, func(db *semanticsdb.DB) error {
			if err := db.CreateAction(name, *desc, *openkore); err != nil {
				return err
			}
			fmt.Fprintf(out, "✓ Created action %q\n", name)
			return nil
		})

	case "update-action":
		fs := flag.NewFlagSet("update-action", flag.ContinueOnError)
		fs.SetOutput(out)
		desc := fs.String("description", "", "new description (omit to leave unchanged)")
		openkore := fs.String("openkore", "", "new openkore_name (omit to leave unchanged)")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		name, err := positional(fs.Args(), 0, "action name")
		if err != nil {
			return err
		}
		return withDB(mappingsPath, true, out, func(db *semanticsdb.DB) error {
			var d, o *string
			if isFlagSet(fs, "description") {
				d = desc
			}
			if isFlagSet(fs, "openkore") {
				o = openkore
			}
			if err := db.UpdateActionMetadata(name, d, o); err != nil {
				return err
			}
			fmt.Fprintf(out, "✓ Updated action %q\n", name)
			return nil
		})

	case "delete-action":
		return withDB(mappingsPath, true, out, func(db *semanticsdb.DB) error {
			name, err := positional(rest, 0, "action name")
			if err != nil {
				return err
			}
			if err := db.DeleteAction(name); err != nil {
				return err
			}
			fmt.Fprintf(out, "✓ Deleted action %q\n", name)
			return nil
		})

	case "rename-action":
		return withDB(mappingsPath, true, out, func(db *semanticsdb.DB) error {
			oldName, err := positional(rest, 0, "old action name")
			if err != nil {
				return err
			}
			newName, err := positional(rest, 1, "new action name")
			if err != nil {
				return err
			}
			if err := db.RenameAction(oldName, newName); err != nil {
				return err
			}
			fmt.Fprintf(out, "✓ Renamed action %q → %q\n", oldName, newName)
			return nil
		})

	case "add-implementation":
		fs := flag.NewFlagSet("add-implementation", flag.ContinueOnError)
		fs.SetOutput(out)
		pid := fs.String("id", "", "packet id (e.g. 0x00FB) — required")
		structName := fs.String("struct", "", "rAthena struct name — required")
		pvMin := fs.Int("min", 0, "packetver_min (0=null/unbounded)")
		pvMax := fs.Int("max", 0, "packetver_max (0=null/unbounded)")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if *pid == "" || *structName == "" {
			return fmt.Errorf("usage: add-implementation <action> -id 0xXXXX -struct NAME [-min N] [-max N]")
		}
		action, err := positional(fs.Args(), 0, "action name")
		if err != nil {
			return err
		}
		return withDB(mappingsPath, true, out, func(db *semanticsdb.DB) error {
			impl := semanticsdb.Implementation{
				PacketID: *pid, StructName: *structName,
				PacketverMin: *pvMin, PacketverMax: *pvMax,
			}
			if err := db.AddImplementation(action, impl); err != nil {
				return err
			}
			fmt.Fprintf(out, "✓ Added implementation %s to action %q\n", impl.PacketID, action)
			return nil
		})

	case "update-implementation":
		fs := flag.NewFlagSet("update-implementation", flag.ContinueOnError)
		fs.SetOutput(out)
		pid := fs.String("id", "", "packet id to update (e.g. 0x00FB) — required")
		structName := fs.String("struct", "", "new struct name (omit to leave unchanged)")
		pvMin := fs.Int("min", 0, "new packetver_min (omit to leave unchanged)")
		pvMax := fs.Int("max", 0, "new packetver_max (omit to leave unchanged)")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if *pid == "" {
			return fmt.Errorf("usage: update-implementation <action> -id 0xXXXX [-struct NAME] [-min N] [-max N]")
		}
		action, err := positional(fs.Args(), 0, "action name")
		if err != nil {
			return err
		}
		return withDB(mappingsPath, true, out, func(db *semanticsdb.DB) error {
			var sn *string
			var minP, maxP *int
			if isFlagSet(fs, "struct") {
				sn = structName
			}
			if isFlagSet(fs, "min") {
				minP = pvMin
			}
			if isFlagSet(fs, "max") {
				maxP = pvMax
			}
			if err := db.UpdateImplementation(action, *pid, sn, minP, maxP); err != nil {
				return err
			}
			fmt.Fprintf(out, "✓ Updated implementation %s on action %q\n", *pid, action)
			return nil
		})

	case "delete-implementation":
		fs := flag.NewFlagSet("delete-implementation", flag.ContinueOnError)
		fs.SetOutput(out)
		pid := fs.String("id", "", "packet id to delete (e.g. 0x00FB) — required")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if *pid == "" {
			return fmt.Errorf("usage: delete-implementation <action> -id 0xXXXX")
		}
		action, err := positional(fs.Args(), 0, "action name")
		if err != nil {
			return err
		}
		return withDB(mappingsPath, true, out, func(db *semanticsdb.DB) error {
			if err := db.DeleteImplementation(action, *pid); err != nil {
				return err
			}
			fmt.Fprintf(out, "✓ Deleted implementation %s from action %q\n", *pid, action)
			return nil
		})

	case "tools":
		// Diagnostic: print the MCP tool schema (useful for MCP-client authors).
		return printJSON(out, mcp.ToolDefinitions())

	default:
		printUsage(out)
		return fmt.Errorf("unknown command: %s", cmd)
	}
}

// withDB loads mappingsPath, runs fn (which may mutate the DB), and saves
// the DB if mutating==true and fn returned no error.
func withDB(path string, mutating bool, out io.Writer, fn func(*semanticsdb.DB) error) error {
	db, err := semanticsdb.Load(path)
	if err != nil {
		return fmt.Errorf("load %s: %w", path, err)
	}
	if err := fn(db); err != nil {
		return err
	}
	if mutating {
		if err := db.Save(); err != nil {
			return fmt.Errorf("save %s: %w", path, err)
		}
	}
	return nil
}

// positional extracts args[i] or returns a friendly error.
func positional(args []string, i int, what string) (string, error) {
	if i >= len(args) {
		return "", fmt.Errorf("missing positional argument: %s", what)
	}
	return args[i], nil
}

// isFlagSet reports whether the named flag was explicitly passed.
func isFlagSet(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func printJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, strings.TrimSpace(`
semantics-tool — editor for rathena-client/semantics/mappings.yaml

Usage:
  semantics-tool [--file PATH] <command> [args]
  semantics-tool serve [--file PATH]   (run as MCP stdio server)

Read-only commands:
  list-actions
  get-action <name>
  list-implementations <name>
  get-implementation <action> <packet_id>
  search [-name X] [-struct X] [-packet X] [-openkore X] [-desc X] [-limit N]
  validate
  stats
  export

Mutating commands:
  create-action <name> [-description X] [-openkore X]
  update-action <name> [-description X] [-openkore X]
  delete-action <name>
  rename-action <oldName> <newName>
  add-implementation <action> -id 0xXXXX -struct NAME [-min N] [-max N]
  update-implementation <action> -id 0xXXXX [-struct NAME] [-min N] [-max N]
  delete-implementation <action> -id 0xXXXX

Diagnostic:
  tools   (print MCP tool schema as JSON)

Default mappings path: semantics/mappings.yaml
`))
}
