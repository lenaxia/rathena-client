// Package semanticsdb is the editor layer over semantics/mappings.yaml.
//
// It exists separately from internal/codegen/semantics (which is the read-only
// loader used by the code generator) because the editor has different needs:
//
//   - It must round-trip the file format, preserving unrelated sections
//     (the flat `mappings:` list, `metadata:`, trailing `version:`) and every
//     unchanged action's exact formatting — comments, quoting style, etc.
//   - It must mutate the `semantic_actions:` section in place: add/remove
//     actions, add/remove/replace implementations, edit metadata.
//   - It is exposed via the cmd/semantics-tool CLI and MCP server.
//
// Implementation strategy: load the file once into a tree of yaml.Node values
// (which preserve source formatting — comments, quote style, key order), then
// walk the tree to expose a typed Go API. Mutations edit the node tree
// directly; Save serializes the whole tree back out. yaml.v3's encoder
// preserves formatting for nodes that came from the parser and emits
// consistently-formatted YAML for newly-built nodes.
//
// This package is in internal/ (not pkg/) so it has no zero-deps obligation.
// It imports gopkg.in/yaml.v3 per README Rule 5's scoping.
package semanticsdb

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Action is a semantic action: a named grouping of one or more packet-ID
// implementations, each pointing at a rAthena struct and bounded by an
// optional packetver range.
type Action struct {
	Name            string
	Description     string
	OpenkoreName    string
	Implementations []Implementation
}

// Implementation is one packet-ID variant for an action.
//
// PacketverMin == 0 means "no lower bound" (the on-disk encoding is `null`).
// PacketverMax == 0 means "no upper bound" (the on-disk encoding is `null`).
// A mapping with both bounds null covers all packetvers.
type Implementation struct {
	PacketID       string // normalised "0xABCD" (uppercase hex)
	StructName     string // rAthena struct name, e.g. PACKET_ZC_GROUP_LIST
	PacketverMin   int    // 0 = null (no lower bound)
	PacketverMax   int    // 0 = null (no upper bound)
	FieldMapping   map[string]string
}

// DB is a mutable view of semantics/mappings.yaml. Methods that mutate the DB
// do NOT write to disk; call Save to flush.
type DB struct {
	path   string
	root   *yaml.Node            // full document tree (preserves formatting)
	action map[string]*yaml.Node // per-action mapping node (inside actionsMap)
}

const (
	// Indentation matches the existing mappings.yaml formatting. Changing
	// these constants would reformat the entire semantic_actions section on
	// the first write — keep them stable.
	indentUnit    = "    " // 4 spaces
	listItemIndent = indentUnit + "- " // for implementation list items
)

// Load reads mappings.yaml from path into a mutable DB.
//
// The file must have a top-level `semantic_actions:` mapping. Other sections
// (`mappings:`, `metadata:`, `version:`) are preserved verbatim through the
// yaml.Node tree and rewritten unchanged on Save.
func Load(path string) (*DB, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return parse(data, path)
}

// parse is split out of Load for testing (tests pass in-memory YAML).
func parse(data []byte, path string) (*DB, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return nil, fmt.Errorf("decode %s: empty or non-document YAML", path)
	}
	topMap := root.Content[0]
	if topMap.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("decode %s: expected top-level mapping, got %s", path, kindName(topMap.Kind))
	}

	actionsMap, err := findMapKey(topMap, "semantic_actions")
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	if actionsMap == nil {
		return nil, fmt.Errorf("decode %s: no `semantic_actions:` top-level key", path)
	}
	if actionsMap.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("decode %s: `semantic_actions:` is not a mapping (got %s)", path, kindName(actionsMap.Kind))
	}

	// Build an index from action name → its mapping node for O(1) lookups.
	// We don't pre-parse Implementations; that happens lazily in GetAction.
	action := make(map[string]*yaml.Node, len(actionsMap.Content)/2)
	for i := 0; i+1 < len(actionsMap.Content); i += 2 {
		k := actionsMap.Content[i]
		if k.Kind == yaml.ScalarNode {
			action[k.Value] = actionsMap.Content[i+1]
		}
	}

	return &DB{path: path, root: &root, action: action}, nil
}

// ListActions returns the names of all semantic actions, sorted alphabetically.
func (d *DB) ListActions() []string {
	names := make([]string, 0, len(d.action))
	for name := range d.action {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetAction returns the named action and whether it existed.
func (d *DB) GetAction(name string) (Action, bool) {
	node, ok := d.action[name]
	if !ok {
		return Action{}, false
	}
	return parseActionNode(name, node), true
}

// ActionExists reports whether the named action is present.
func (d *DB) ActionExists(name string) bool {
	_, ok := d.action[name]
	return ok
}

// ImplementationCount returns the number of implementations for the action,
// or 0 if the action does not exist.
func (d *DB) ImplementationCount(actionName string) int {
	a, ok := d.GetAction(actionName)
	if !ok {
		return 0
	}
	return len(a.Implementations)
}

// GetImplementation returns the implementation for (action, packetID) and
// whether it existed.
func (d *DB) GetImplementation(actionName, packetID string) (Implementation, bool) {
	a, ok := d.GetAction(actionName)
	if !ok {
		return Implementation{}, false
	}
	want := normPacketID(packetID)
	for _, impl := range a.Implementations {
		if impl.PacketID == want {
			return impl, true
		}
	}
	return Implementation{}, false
}

// Stats holds high-level DB statistics.
type Stats struct {
	ActionCount       int
	ImplementationCount int
	ActionsWithImpls  int
}

// Statistics returns aggregate counts over the DB.
func (d *DB) Statistics() Stats {
	var s Stats
	for _, name := range d.ListActions() {
		a, _ := d.GetAction(name)
		s.ActionCount++
		s.ImplementationCount += len(a.Implementations)
		if len(a.Implementations) > 0 {
			s.ActionsWithImpls++
		}
	}
	return s
}

// --- helpers ---

// findMapKey returns the *value* node child of m whose key is name, or nil if
// the key is absent. Returns an error if m is not a mapping.
func findMapKey(m *yaml.Node, name string) (*yaml.Node, error) {
	if m.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected mapping, got %s", kindName(m.Kind))
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == name {
			return m.Content[i+1], nil
		}
	}
	return nil, nil
}

func kindName(k yaml.Kind) string {
	switch k {
	case yaml.DocumentNode:
		return "document"
	case yaml.SequenceNode:
		return "sequence"
	case yaml.MappingNode:
		return "mapping"
	case yaml.ScalarNode:
		return "scalar"
	case yaml.AliasNode:
		return "alias"
	default:
		return fmt.Sprintf("kind(%d)", k)
	}
}

// parseActionNode walks one action's mapping node and produces a typed Action.
func parseActionNode(name string, node *yaml.Node) Action {
	a := Action{Name: name}
	if node.Kind == yaml.ScalarNode && node.Tag == "!!null" {
		return a // empty action (rare; usually a mapping)
	}
	if node.Kind != yaml.MappingNode {
		return a
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i].Value
		v := node.Content[i+1]
		switch k {
		case "name":
			a.Name = scalarString(v)
		case "description":
			a.Description = scalarString(v)
		case "openkore_name":
			a.OpenkoreName = scalarString(v)
		case "implementations":
			a.Implementations = parseImplsNode(v)
		}
	}
	if a.Name == "" {
		a.Name = name
	}
	return a
}

// parseImplsNode walks an implementations: sequence node.
func parseImplsNode(node *yaml.Node) []Implementation {
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil
	}
	out := make([]Implementation, 0, len(node.Content))
	for _, item := range node.Content {
		out = append(out, parseImplNode(item))
	}
	return out
}

// parseImplNode walks one implementation mapping node.
func parseImplNode(node *yaml.Node) Implementation {
	var impl Implementation
	if node.Kind != yaml.MappingNode {
		return impl
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i].Value
		v := node.Content[i+1]
		switch k {
		case "packet_id":
			impl.PacketID = normPacketID(scalarString(v))
		case "struct_name":
			impl.StructName = scalarString(v)
		case "packetver_range":
			min, max := parsePacketverRange(v)
			impl.PacketverMin = min
			impl.PacketverMax = max
		case "field_mapping":
			impl.FieldMapping = parseFieldMapping(v)
		}
	}
	return impl
}

// parsePacketverRange reads a packetver_range: sequence node → (min, max).
// null entries become 0 (meaning "no bound").
func parsePacketverRange(node *yaml.Node) (min, max int) {
	if node == nil || node.Kind != yaml.SequenceNode {
		return 0, 0
	}
	vals := make([]int, 0, 2)
	for _, item := range node.Content {
		if item.Tag == "!!null" {
			vals = append(vals, 0)
			continue
		}
		var n int
		if err := item.Decode(&n); err == nil {
			vals = append(vals, n)
			continue
		}
		var s string
		if err := item.Decode(&s); err != nil {
			continue
		}
		var parsed int
		if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &parsed); err == nil {
			vals = append(vals, parsed)
		}
	}
	if len(vals) >= 1 {
		min = vals[0]
	}
	if len(vals) >= 2 {
		max = vals[1]
	}
	return min, max
}

func parseFieldMapping(node *yaml.Node) map[string]string {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	out := make(map[string]string, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := scalarString(node.Content[i])
		v := scalarString(node.Content[i+1])
		out[k] = v
	}
	return out
}

// scalarString returns the string value of a scalar node, stripping YAML
// quoting. Returns "" for null/non-scalar nodes.
func scalarString(node *yaml.Node) string {
	if node == nil {
		return ""
	}
	if node.Kind == yaml.ScalarNode {
		if node.Tag == "!!null" {
			return ""
		}
		return node.Value
	}
	return ""
}

// normPacketID normalises a packet ID to "0xABCD" form (uppercase hex digits).
func normPacketID(s string) string {
	lower := strings.ToLower(strings.TrimSpace(s))
	if lower == "" {
		return ""
	}
	if strings.HasPrefix(lower, "0x") {
		return "0x" + strings.ToUpper(lower[2:])
	}
	return "0x" + strings.ToUpper(lower)
}
