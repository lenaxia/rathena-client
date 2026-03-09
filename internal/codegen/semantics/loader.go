// Package semantics loads the semantic_actions section of semantics/mappings.yaml
// into Go structs. It uses a minimal hand-written YAML parser that handles the
// specific subset of YAML used in mappings.yaml — no external dependencies.
package semantics

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// CanonicalParam is one entry in an action's canonical_params list.
type CanonicalParam struct {
	Name     string
	Type     string
	Semantic string
}

// Implementation is one packet ID variant for a semantic action.
type Implementation struct {
	PacketID     string            // e.g. "0x009F"
	PacketverMin int               // e.g. 20030000
	PacketverMax int               // 0 = no upper bound
	StructName   string            // rAthena struct name, e.g. PACKET_ZC_NOTIFY_VANISH
	FieldMapping map[string]string // canonical param → Go expression
}

// Action is a single semantic action from the semantic_actions section.
type Action struct {
	Name            string
	Description     string
	OpenkoreName    string
	CanonicalParams []CanonicalParam
	Implementations []Implementation
}

// DB holds all semantic actions keyed by action name.
type DB struct {
	Actions map[string]*Action
}

// LoadFile reads the mappings.yaml file at path and returns the semantic DB.
func LoadFile(path string) (*DB, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}
	return parse(lines)
}

// parse extracts the semantic_actions section and builds a DB.
func parse(lines []string) (*DB, error) {
	// Find the start of the semantic_actions section.
	start := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == "semantic_actions:" {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return nil, fmt.Errorf("semantic_actions: section not found")
	}

	db := &DB{Actions: make(map[string]*Action)}

	// State machine: we iterate line by line within the semantic_actions block.
	// Indentation levels:
	//   4 spaces  — action name key (e.g. "    ac_accept_login:")
	//   8 spaces  — action field (name, description, canonical_params, implementations)
	//   12 spaces — list item start "- " or map value
	//   16 spaces — param/impl field
	//   20 spaces — field_mapping key
	//
	// We detect blocks by indent level.

	type state int
	const (
		sTop      state = iota // inside semantic_actions, looking for action name
		sAction                // inside an action, looking for action fields
		sParams                // inside canonical_params list
		sParam                 // inside a single param
		sImpls                 // inside implementations list
		sImpl                  // inside a single implementation
		sFieldMap              // inside field_mapping
	)

	cur := sTop
	var action *Action
	var param *CanonicalParam
	var impl *Implementation

	for i := start; i < len(lines); i++ {
		raw := lines[i]

		// Stop at top-level keys that follow semantic_actions (e.g. "version:")
		if len(raw) > 0 && raw[0] != ' ' && raw[0] != '\t' {
			break
		}

		// Skip blank lines and comment lines.
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := countIndent(raw)

		switch cur {
		case sTop:
			// Expect "    <action_name>:" at indent 4.
			if indent == 4 && strings.HasSuffix(trimmed, ":") {
				name := strings.TrimSuffix(trimmed, ":")
				name = unquote(name)
				action = &Action{Name: name}
				db.Actions[name] = action
				cur = sAction
			}

		case sAction:
			if indent == 4 {
				// New top-level action (shouldn't happen without colon, but guard).
				if strings.HasSuffix(trimmed, ":") {
					name := strings.TrimSuffix(trimmed, ":")
					name = unquote(name)
					action = &Action{Name: name}
					db.Actions[name] = action
				}
				continue
			}
			if indent < 8 {
				// Stepped out of action scope.
				cur = sTop
				i-- // re-process this line
				continue
			}
			if indent == 8 {
				k, v := splitKV(trimmed)
				switch k {
				case "name":
					action.Name = unquote(v)
				case "description":
					action.Description = unquote(v)
				case "openkore_name":
					action.OpenkoreName = unquote(v)
				case "canonical_params":
					// value is empty (block sequence follows)
					cur = sParams
				case "implementations":
					cur = sImpls
				}
			}

		case sParams:
			if indent < 8 {
				cur = sTop
				i--
				continue
			}
			if indent == 8 {
				// Stepped out of canonical_params to another action field.
				k, _ := splitKV(trimmed)
				switch k {
				case "implementations":
					cur = sImpls
				default:
					// Some other field (shouldn't happen normally).
					cur = sAction
					i--
				}
				continue
			}
			// indent == 12: list items "- name: ..."
			if indent == 12 && strings.HasPrefix(trimmed, "- ") {
				// Start of a new param entry.
				param = &CanonicalParam{}
				action.CanonicalParams = append(action.CanonicalParams, *param)
				// The param pointer must point to the slice element.
				param = &action.CanonicalParams[len(action.CanonicalParams)-1]
				cur = sParam
				// Parse the inline key-value after "- ".
				rest := strings.TrimPrefix(trimmed, "- ")
				k, v := splitKV(rest)
				setParamField(param, k, v)
				continue
			}
			// indent == 14: continuation fields of a param.
			if indent == 14 {
				if param != nil {
					k, v := splitKV(trimmed)
					setParamField(param, k, v)
				}
				continue
			}

		case sParam:
			if indent < 12 {
				// Left the param list entirely.
				cur = sAction
				i--
				continue
			}
			if indent == 12 {
				if strings.HasPrefix(trimmed, "- ") {
					// Next param.
					param = &CanonicalParam{}
					action.CanonicalParams = append(action.CanonicalParams, *param)
					param = &action.CanonicalParams[len(action.CanonicalParams)-1]
					rest := strings.TrimPrefix(trimmed, "- ")
					k, v := splitKV(rest)
					setParamField(param, k, v)
				} else {
					// Back to sParams level without a list item — transition.
					cur = sParams
					i--
				}
				continue
			}
			if indent >= 14 {
				if param != nil {
					k, v := splitKV(trimmed)
					setParamField(param, k, v)
				}
				continue
			}

		case sImpls:
			if indent < 8 {
				cur = sTop
				i--
				continue
			}
			if indent == 8 {
				// Another action-level field after implementations.
				cur = sAction
				i--
				continue
			}
			// indent == 12: list items "- packet_id: ..."
			if indent == 12 && strings.HasPrefix(trimmed, "- ") {
				impl = &Implementation{FieldMapping: make(map[string]string)}
				action.Implementations = append(action.Implementations, *impl)
				impl = &action.Implementations[len(action.Implementations)-1]
				cur = sImpl
				rest := strings.TrimPrefix(trimmed, "- ")
				k, v := splitKV(rest)
				setImplField(impl, k, v)
				continue
			}

		case sImpl:
			if indent < 12 {
				cur = sAction
				i--
				continue
			}
			if indent == 12 {
				if strings.HasPrefix(trimmed, "- ") {
					// Next implementation.
					impl = &Implementation{FieldMapping: make(map[string]string)}
					action.Implementations = append(action.Implementations, *impl)
					impl = &action.Implementations[len(action.Implementations)-1]
					rest := strings.TrimPrefix(trimmed, "- ")
					k, v := splitKV(rest)
					setImplField(impl, k, v)
				} else {
					cur = sImpls
					i--
				}
				continue
			}
			// indent == 14: continuation fields of a list item (aligned to content after "- ").
			if indent == 14 {
				k, v := splitKV(trimmed)
				switch k {
				case "field_mapping":
					cur = sFieldMap
				case "packetver_range":
					// list items follow at indent 16; skip the key line, handled below.
				default:
					setImplField(impl, k, v)
				}
				continue
			}
			// indent == 16: packetver_range list items.
			if indent == 16 {
				if strings.HasPrefix(trimmed, "- ") {
					val := strings.TrimPrefix(trimmed, "- ")
					val = strings.TrimSpace(val)
					if val != "null" && val != "" {
						if impl.PacketverMin == 0 {
							parseIntInto(val, &impl.PacketverMin)
						} else {
							parseIntInto(val, &impl.PacketverMax)
						}
					}
				}
				continue
			}

		case sFieldMap:
			if indent < 12 {
				// Stepped out of the implementations list entirely.
				cur = sAction
				i--
				continue
			}
			if indent == 12 {
				if strings.HasPrefix(trimmed, "- ") {
					// Next implementation in the list.
					impl = &Implementation{FieldMapping: make(map[string]string)}
					action.Implementations = append(action.Implementations, *impl)
					impl = &action.Implementations[len(action.Implementations)-1]
					cur = sImpl
					rest := strings.TrimPrefix(trimmed, "- ")
					k, v := splitKV(rest)
					setImplField(impl, k, v)
				} else {
					// Non-list line at indent 12 — back to sImpls.
					cur = sImpls
					i--
				}
				continue
			}
			if indent == 14 {
				// End of field_mapping — back to impl continuation fields.
				cur = sImpl
				i--
				continue
			}
			if indent == 16 {
				k, v := splitKV(trimmed)
				if impl != nil && k != "" {
					impl.FieldMapping[k] = v
				}
				continue
			}
		}
	}

	return db, nil
}

// --- helpers ---

func countIndent(s string) int {
	n := 0
	for _, c := range s {
		if c == ' ' {
			n++
		} else if c == '\t' {
			n += 4
		} else {
			break
		}
	}
	return n
}

// splitKV splits "key: value" → ("key", "value").
// value may be empty (block scalar follows).
func splitKV(s string) (key, val string) {
	idx := strings.Index(s, ":")
	if idx < 0 {
		return strings.TrimSpace(s), ""
	}
	key = strings.TrimSpace(s[:idx])
	val = strings.TrimSpace(s[idx+1:])
	key = unquote(key)
	val = unquote(val)
	return
}

// unquote strips surrounding single or double quotes and unescapes \\n, \\.
func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '\'' && s[len(s)-1] == '\'') ||
			(s[0] == '"' && s[len(s)-1] == '"') {
			s = s[1 : len(s)-1]
		}
	}
	return s
}

func setParamField(p *CanonicalParam, k, v string) {
	switch k {
	case "name":
		p.Name = v
	case "type":
		p.Type = v
	case "semantic":
		p.Semantic = v
	}
}

func setImplField(impl *Implementation, k, v string) {
	switch k {
	case "packet_id":
		// Normalise to "0x009F" form (lower-case 0x, upper-case hex digits).
		lower := strings.ToLower(v)
		if strings.HasPrefix(lower, "0x") {
			impl.PacketID = "0x" + strings.ToUpper(lower[2:])
		} else {
			impl.PacketID = "0x" + strings.ToUpper(lower)
		}
	case "struct_name":
		impl.StructName = v
	}
}

func parseIntInto(s string, dst *int) {
	s = strings.TrimSpace(s)
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	*dst = n
}
