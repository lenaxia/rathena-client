package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/lenaxia/rathena-client/internal/semanticsdb"
)

// ToolDefinitions returns the JSON schema for all exposed tools. The schema
// follows the MCP tools/list format — each tool declares its name,
// description, and a JSON Schema for its input arguments.
//
// Schemas are written in the simplest possible shape (object with
// named properties, all optional unless required). No $ref, no
// composition — keeps the LLM-facing surface small.
func ToolDefinitions() []MCPTool {
	prop := func(desc string) map[string]any {
		return map[string]any{"type": "string", "description": desc}
	}
	propInt := func(desc string) map[string]any {
		return map[string]any{"type": "integer", "description": desc}
	}
	schema := func(props map[string]any, required []string) map[string]any {
		out := map[string]any{
			"type":       "object",
			"properties": props,
		}
		if len(required) > 0 {
			out["required"] = required
		}
		return out
	}

	return []MCPTool{
		// --- read-only ---
		{
			Name:        "list_actions",
			Description: "List all semantic action names in alphabetical order.",
			InputSchema: schema(map[string]any{}, nil),
		},
		{
			Name:        "get_action",
			Description: "Get full detail (metadata + all implementations) for one semantic action.",
			InputSchema: schema(
				map[string]any{"action_name": prop("Name of the semantic action to retrieve.")},
				[]string{"action_name"},
			),
		},
		{
			Name:        "list_implementations",
			Description: "List the packet_id implementations of one action.",
			InputSchema: schema(
				map[string]any{"action_name": prop("Name of the action.")},
				[]string{"action_name"},
			),
		},
		{
			Name:        "get_implementation",
			Description: "Get one implementation of one action (matched by packet_id).",
			InputSchema: schema(
				map[string]any{
					"action_name": prop("Name of the action."),
					"packet_id":   prop("Packet ID, e.g. \"0x00FB\"."),
				},
				[]string{"action_name", "packet_id"},
			),
		},
		{
			Name:        "search_actions",
			Description: "Substring search (case-insensitive) across action name, openkore_name, description, struct_name, packet_id. Empty fields match anything.",
			InputSchema: schema(
				map[string]any{
					"name":          prop("Substring of action name."),
					"openkore_name": prop("Substring of openkore_name."),
					"description":   prop("Substring of description."),
					"struct_name":   prop("Substring of rAthena struct name on any implementation."),
					"packet_id":     prop("Exact packet_id (e.g. \"0x00FB\")."),
					"limit":         propInt("Max results (0 = unlimited)."),
				},
				nil,
			),
		},
		{
			Name:        "validate",
			Description: "Run structural validation over the whole DB. Returns a list of findings (empty list = OK).",
			InputSchema: schema(map[string]any{}, nil),
		},
		{
			Name:        "stats",
			Description: "Return aggregate counts: number of actions, total implementations, etc.",
			InputSchema: schema(map[string]any{}, nil),
		},
		{
			Name:        "export",
			Description: "Export the entire DB as a JSON object. Useful for diffing or backup.",
			InputSchema: schema(map[string]any{}, nil),
		},

		// --- mutating ---
		{
			Name:        "create_action",
			Description: "Create a new empty semantic action with the given metadata.",
			InputSchema: schema(
				map[string]any{
					"action_name":   prop("Name of the new action (lowercase_snake_case)."),
					"description":   prop("Human-readable description (optional)."),
					"openkore_name": prop("OpenKore packet name (optional)."),
				},
				[]string{"action_name"},
			),
		},
		{
			Name:        "update_action",
			Description: "Update the description or openkore_name of an existing action. Either field may be omitted to leave it unchanged.",
			InputSchema: schema(
				map[string]any{
					"action_name":   prop("Name of the action to update."),
					"description":   prop("New description (omit to leave unchanged)."),
					"openkore_name": prop("New openkore_name (omit to leave unchanged)."),
				},
				[]string{"action_name"},
			),
		},
		{
			Name:        "delete_action",
			Description: "Delete a semantic action and all its implementations.",
			InputSchema: schema(
				map[string]any{"action_name": prop("Name of the action to delete.")},
				[]string{"action_name"},
			),
		},
		{
			Name:        "rename_action",
			Description: "Rename a semantic action. Preserves document position, implementations, and metadata. The inner name: field is updated only if it currently mirrors the old key.",
			InputSchema: schema(
				map[string]any{
					"old_name": prop("Current name of the action."),
					"new_name": prop("New name for the action (lowercase_snake_case)."),
				},
				[]string{"old_name", "new_name"},
			),
		},
		{
			Name:        "add_implementation",
			Description: "Add one packet_id implementation to an existing action. packetver_min/max of 0 means null (no bound).",
			InputSchema: schema(
				map[string]any{
					"action_name":   prop("Name of the action to extend."),
					"packet_id":     prop("Packet ID, e.g. \"0x00FB\"."),
					"struct_name":   prop("rAthena struct name, e.g. PACKET_ZC_GROUP_LIST."),
					"packetver_min": propInt("Lower packetver bound (0 = null/no bound)."),
					"packetver_max": propInt("Upper packetver bound (0 = null/no bound)."),
				},
				[]string{"action_name", "packet_id", "struct_name"},
			),
		},
		{
			Name:        "update_implementation",
			Description: "Update struct_name and/or packetver range of one implementation (matched by packet_id). Omit fields to leave them unchanged.",
			InputSchema: schema(
				map[string]any{
					"action_name":   prop("Name of the action."),
					"packet_id":     prop("Packet ID of the implementation to update."),
					"struct_name":   prop("New struct name (omit to leave unchanged)."),
					"packetver_min": propInt("New lower bound (omit to leave unchanged; 0 = null)."),
					"packetver_max": propInt("New upper bound (omit to leave unchanged; 0 = null)."),
				},
				[]string{"action_name", "packet_id"},
			),
		},
		{
			Name:        "delete_implementation",
			Description: "Delete one packet_id implementation from an action.",
			InputSchema: schema(
				map[string]any{
					"action_name": prop("Name of the action."),
					"packet_id":   prop("Packet ID of the implementation to delete."),
				},
				[]string{"action_name", "packet_id"},
			),
		},
	}
}

// dispatchTool routes a tool call to its handler. Handlers receive a loaded
// DB and the tool arguments; they mutate the DB in place for write tools
// (the caller will Save if no error was returned).
func dispatchTool(db *semanticsdb.DB, name string, args map[string]any) (any, error) {
	switch name {
	// --- read-only ---
	case "list_actions":
		return map[string]any{
			"actions": db.ListActions(),
			"count":   len(db.ListActions()),
		}, nil

	case "get_action":
		actionName, err := reqStr(args, "action_name")
		if err != nil {
			return nil, err
		}
		a, ok := db.GetAction(actionName)
		if !ok {
			return nil, fmt.Errorf("action %q not found", actionName)
		}
		return map[string]any{"action": toJSONableAction(a)}, nil

	case "list_implementations":
		actionName, err := reqStr(args, "action_name")
		if err != nil {
			return nil, err
		}
		a, ok := db.GetAction(actionName)
		if !ok {
			return nil, fmt.Errorf("action %q not found", actionName)
		}
		return map[string]any{
			"action_name":     actionName,
			"implementations": toJSONableImpls(a.Implementations),
			"count":           len(a.Implementations),
		}, nil

	case "get_implementation":
		actionName, err := reqStr(args, "action_name")
		if err != nil {
			return nil, err
		}
		packetID, err := reqStr(args, "packet_id")
		if err != nil {
			return nil, err
		}
		impl, ok := db.GetImplementation(actionName, packetID)
		if !ok {
			return nil, fmt.Errorf("implementation %s not found on action %q", packetID, actionName)
		}
		return map[string]any{
			"action_name":    actionName,
			"implementation": toJSONableImpl(impl),
		}, nil

	case "search_actions":
		q := semanticsdb.SearchQuery{
			Name:         optStr(args, "name"),
			OpenkoreName: optStr(args, "openkore_name"),
			Description:  optStr(args, "description"),
			StructName:   optStr(args, "struct_name"),
			PacketID:     optStr(args, "packet_id"),
		}
		limit := optInt(args, "limit", 0)
		results := db.Search(q, limit)
		out := make([]map[string]any, 0, len(results))
		for _, a := range results {
			out = append(out, toJSONableAction(a))
		}
		return map[string]any{
			"results": out,
			"count":   len(out),
		}, nil

	case "validate":
		errs := db.Validate()
		out := make([]map[string]any, 0, len(errs))
		for _, e := range errs {
			out = append(out, map[string]any{
				"action_name": e.ActionName,
				"packet_id":   e.PacketID,
				"message":     e.Message,
			})
		}
		return map[string]any{
			"valid":       len(errs) == 0,
			"error_count": len(errs),
			"errors":      out,
		}, nil

	case "stats":
		s := db.Statistics()
		return map[string]any{
			"action_count":         s.ActionCount,
			"implementation_count": s.ImplementationCount,
			"actions_with_impls":   s.ActionsWithImpls,
		}, nil

	case "export":
		var actions []map[string]any
		for _, name := range db.ListActions() {
			a, _ := db.GetAction(name)
			actions = append(actions, toJSONableAction(a))
		}
		return map[string]any{
			"actions": actions,
			"count":   len(actions),
		}, nil

	// --- mutating ---
	case "create_action":
		actionName, err := reqStr(args, "action_name")
		if err != nil {
			return nil, err
		}
		desc := optStr(args, "description")
		openkore := optStr(args, "openkore_name")
		if err := db.CreateAction(actionName, desc, openkore); err != nil {
			return nil, err
		}
		return map[string]any{
			"success":     true,
			"action_name": actionName,
			"message":     fmt.Sprintf("Created action %q", actionName),
		}, nil

	case "update_action":
		actionName, err := reqStr(args, "action_name")
		if err != nil {
			return nil, err
		}
		var desc, openkore *string
		if v, ok := args["description"]; ok {
			s := toString(v)
			desc = &s
		}
		if v, ok := args["openkore_name"]; ok {
			s := toString(v)
			openkore = &s
		}
		if err := db.UpdateActionMetadata(actionName, desc, openkore); err != nil {
			return nil, err
		}
		return map[string]any{
			"success":     true,
			"action_name": actionName,
			"message":     fmt.Sprintf("Updated action %q metadata", actionName),
		}, nil

	case "delete_action":
		actionName, err := reqStr(args, "action_name")
		if err != nil {
			return nil, err
		}
		if err := db.DeleteAction(actionName); err != nil {
			return nil, err
		}
		return map[string]any{
			"success":     true,
			"action_name": actionName,
			"message":     fmt.Sprintf("Deleted action %q", actionName),
		}, nil

	case "rename_action":
		oldName, err := reqStr(args, "old_name")
		if err != nil {
			return nil, err
		}
		newName, err := reqStr(args, "new_name")
		if err != nil {
			return nil, err
		}
		if err := db.RenameAction(oldName, newName); err != nil {
			return nil, err
		}
		return map[string]any{
			"success":  true,
			"old_name": oldName,
			"new_name": newName,
			"message":  fmt.Sprintf("Renamed action %q → %q", oldName, newName),
		}, nil

	case "add_implementation":
		actionName, err := reqStr(args, "action_name")
		if err != nil {
			return nil, err
		}
		packetID, err := reqStr(args, "packet_id")
		if err != nil {
			return nil, err
		}
		structName, err := reqStr(args, "struct_name")
		if err != nil {
			return nil, err
		}
		impl := semanticsdb.Implementation{
			PacketID:     packetID,
			StructName:   structName,
			PacketverMin: optInt(args, "packetver_min", 0),
			PacketverMax: optInt(args, "packetver_max", 0),
		}
		if err := db.AddImplementation(actionName, impl); err != nil {
			return nil, err
		}
		return map[string]any{
			"success":     true,
			"action_name": actionName,
			"packet_id":   impl.PacketID,
			"message":     fmt.Sprintf("Added implementation %s to action %q", impl.PacketID, actionName),
		}, nil

	case "update_implementation":
		actionName, err := reqStr(args, "action_name")
		if err != nil {
			return nil, err
		}
		packetID, err := reqStr(args, "packet_id")
		if err != nil {
			return nil, err
		}
		var structName *string
		var pvMin, pvMax *int
		if v, ok := args["struct_name"]; ok {
			s := toString(v)
			structName = &s
		}
		if v, ok := args["packetver_min"]; ok {
			n := toInt(v)
			pvMin = &n
		}
		if v, ok := args["packetver_max"]; ok {
			n := toInt(v)
			pvMax = &n
		}
		if err := db.UpdateImplementation(actionName, packetID, structName, pvMin, pvMax); err != nil {
			return nil, err
		}
		return map[string]any{
			"success":     true,
			"action_name": actionName,
			"packet_id":   packetID,
			"message":     fmt.Sprintf("Updated implementation %s on action %q", packetID, actionName),
		}, nil

	case "delete_implementation":
		actionName, err := reqStr(args, "action_name")
		if err != nil {
			return nil, err
		}
		packetID, err := reqStr(args, "packet_id")
		if err != nil {
			return nil, err
		}
		if err := db.DeleteImplementation(actionName, packetID); err != nil {
			return nil, err
		}
		return map[string]any{
			"success":     true,
			"action_name": actionName,
			"packet_id":   packetID,
			"message":     fmt.Sprintf("Deleted implementation %s from action %q", packetID, actionName),
		}, nil
	}

	return nil, fmt.Errorf("unknown tool: %s", name)
}

// --- JSON-friendly converters ---

// jsonableImpl is the JSON representation of an Implementation. Pointers
// for the packetver bounds so that null and 0 are distinguishable in the
// output (null = unbounded, 0 = explicit zero).
type jsonableImpl struct {
	PacketID     string            `json:"packet_id"`
	StructName   string            `json:"struct_name"`
	PacketverMin *int              `json:"packetver_min"`
	PacketverMax *int              `json:"packetver_max"`
	FieldMapping map[string]string `json:"field_mapping,omitempty"`
}

type jsonableAction struct {
	Name            string         `json:"name"`
	Description     string         `json:"description"`
	OpenkoreName    string         `json:"openkore_name"`
	Implementations []jsonableImpl `json:"implementations"`
}

func toJSONableAction(a semanticsdb.Action) map[string]any {
	return map[string]any{
		"name":                 a.Name,
		"description":          a.Description,
		"openkore_name":        a.OpenkoreName,
		"implementations":      toJSONableImpls(a.Implementations),
		"implementation_count": len(a.Implementations),
	}
}

func toJSONableImpls(impls []semanticsdb.Implementation) []jsonableImpl {
	out := make([]jsonableImpl, 0, len(impls))
	for _, impl := range impls {
		out = append(out, toJSONableImpl(impl))
	}
	return out
}

func toJSONableImpl(impl semanticsdb.Implementation) jsonableImpl {
	out := jsonableImpl{
		PacketID:     impl.PacketID,
		StructName:   impl.StructName,
		FieldMapping: impl.FieldMapping,
	}
	if impl.PacketverMin != 0 {
		v := impl.PacketverMin
		out.PacketverMin = &v
	}
	if impl.PacketverMax != 0 {
		v := impl.PacketverMax
		out.PacketverMax = &v
	}
	return out
}

// --- arg-extraction helpers ---

func reqStr(args map[string]any, key string) (string, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return "", fmt.Errorf("missing required argument %q", key)
	}
	return toString(v), nil
}

func optStr(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok || v == nil {
		return ""
	}
	return toString(v)
}

func optInt(args map[string]any, key string, def int) int {
	v, ok := args[key]
	if !ok || v == nil {
		return def
	}
	return toInt(v)
}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		// JSON numbers come through as float64. For a packet_id like
		// "0x00FB" the user must pass a string; this branch only fires
		// for genuinely numeric fields that were given as JSON numbers.
		return fmt.Sprintf("%g", x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func toInt(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case string:
		var n int
		_, _ = fmt.Sscanf(x, "%d", &n)
		return n
	}
	return 0
}
