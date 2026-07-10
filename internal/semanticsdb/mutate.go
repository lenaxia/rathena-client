package semanticsdb

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Mutation errors.
var (
	ErrActionExists        = errors.New("semantic action already exists")
	ErrActionNotFound      = errors.New("semantic action not found")
	ErrImplExists          = errors.New("implementation already exists for this action")
	ErrImplNotFound        = errors.New("implementation not found for this action")
	ErrEmptyActionName     = errors.New("action name must not be empty")
	ErrEmptyPacketID       = errors.New("packet_id must not be empty")
	ErrEmptyStructName     = errors.New("struct_name must not be empty")
)

// CreateAction adds a new empty action with the given metadata. Returns
// ErrActionExists if an action with this name is already present.
//
// The new action is appended after the last existing action in document order,
// matching the pattern of past additions (e.g. zc_party_join_req was inserted
// alphabetically before zc_pc_purchase_itemlist_frommc — see git d76a1d9).
// We don't enforce alphabetical position; we append.
func (d *DB) CreateAction(name, description, openkoreName string) error {
	if strings.TrimSpace(name) == "" {
		return ErrEmptyActionName
	}
	if _, ok := d.action[name]; ok {
		return fmt.Errorf("%w: %s", ErrActionExists, name)
	}

	node := buildActionNode(name, description, openkoreName, nil)
	appendActionsMappingEntry(d.root, name, node)
	d.action[name] = node
	return nil
}

// DeleteAction removes the named action entirely. Returns ErrActionNotFound
// if absent.
func (d *DB) DeleteAction(name string) error {
	node, ok := d.action[name]
	if !ok {
		return fmt.Errorf("%w: %s", ErrActionNotFound, name)
	}
	if err := removeActionsMappingEntry(d.root, name, node); err != nil {
		return err
	}
	delete(d.action, name)
	return nil
}

// UpdateActionMetadata edits the description and/or openkore_name of an
// existing action. Either argument may be empty (no change) — pass a pointer
// to a string to set, including the empty string. Pass nil to leave it
// untouched.
func (d *DB) UpdateActionMetadata(name string, description, openkoreName *string) error {
	node, ok := d.action[name]
	if !ok {
		return fmt.Errorf("%w: %s", ErrActionNotFound, name)
	}
	ensureMappingNode(node)
	if description != nil {
		setMappingField(node, "description", *description)
	}
	if openkoreName != nil {
		setMappingField(node, "openkore_name", *openkoreName)
	}
	return nil
}

// AddImplementation adds a packet-ID implementation to an existing action.
// Returns ErrActionNotFound if the action is missing, ErrImplExists if an
// implementation with the same packet_id is already present.
func (d *DB) AddImplementation(actionName string, impl Implementation) error {
	if strings.TrimSpace(impl.PacketID) == "" {
		return ErrEmptyPacketID
	}
	if strings.TrimSpace(impl.StructName) == "" {
		return ErrEmptyStructName
	}
	node, ok := d.action[actionName]
	if !ok {
		return fmt.Errorf("%w: %s", ErrActionNotFound, actionName)
	}
	impl.PacketID = normPacketID(impl.PacketID)

	// Reject duplicates.
	existing := parseImplsNode(getMappingValue(node, "implementations"))
	for _, e := range existing {
		if e.PacketID == impl.PacketID {
			return fmt.Errorf("%w: %s on action %s", ErrImplExists, impl.PacketID, actionName)
		}
	}

	implNode := buildImplNode(impl)
	appendImplToAction(node, implNode)
	return nil
}

// UpdateImplementation replaces the metadata of one implementation (matched
// by packet_id) within an action. Returns ErrActionNotFound or
// ErrImplNotFound on missing targets.
//
// structName, packetverMin, packetverMax are pointers; pass nil to leave
// each field untouched.
func (d *DB) UpdateImplementation(actionName, packetID string, structName *string, packetverMin, packetverMax *int) error {
	node, ok := d.action[actionName]
	if !ok {
		return fmt.Errorf("%w: %s", ErrActionNotFound, actionName)
	}
	want := normPacketID(packetID)
	idx, found := findImplIndex(node, want)
	if !found {
		return fmt.Errorf("%w: %s on action %s", ErrImplNotFound, want, actionName)
	}
	implNode := getImplsSeq(node).Content[idx]
	ensureMappingNode(implNode)
	if structName != nil {
		setMappingField(implNode, "struct_name", *structName)
	}
	if packetverMin != nil || packetverMax != nil {
		newMin, newMax := currentRange(implNode)
		if packetverMin != nil {
			newMin = *packetverMin
		}
		if packetverMax != nil {
			newMax = *packetverMax
		}
		replaceImplRange(implNode, newMin, newMax)
	}
	return nil
}

// DeleteImplementation removes one packet-ID implementation from an action.
func (d *DB) DeleteImplementation(actionName, packetID string) error {
	node, ok := d.action[actionName]
	if !ok {
		return fmt.Errorf("%w: %s", ErrActionNotFound, actionName)
	}
	want := normPacketID(packetID)
	seq := getImplsSeq(node)
	idx, found := findImplIndex(node, want)
	if !found {
		return fmt.Errorf("%w: %s on action %s", ErrImplNotFound, want, actionName)
	}
	seq.Content = append(seq.Content[:idx], seq.Content[idx+1:]...)
	return nil
}

// Save serializes the current node tree back to d.path. The output preserves
// the source formatting of unchanged content (yaml.v3 round-trips node trees
// for parsed input) and emits consistent formatting for newly-added nodes
// via the buildXxxNode helpers in this file.
//
// The file is written atomically: a temp file in the same directory is
// written, then renamed over the original.
func (d *DB) Save() error {
	if d.path == "" {
		return errors.New("Save: DB has no path (use SaveTo)")
	}
	return d.SaveTo(d.path)
}

// SaveTo writes the DB to outPath, atomically.
func (d *DB) SaveTo(outPath string) error {
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(4)
	if err := enc.Encode(d.root); err != nil {
		return fmt.Errorf("encode %s: %w", outPath, err)
	}
	enc.Close()
	// yaml.v3 Encoder emits a trailing "---" document separator on Encode
	// only when there is more than one document; our single-doc tree
	// produces a single trailing newline. Append a final newline if missing.
	out := buf.String()
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}

	dir := dirOf(outPath)
	tmp, err := os.CreateTemp(dir, ".semantics-*.yaml")
	if err != nil {
		return fmt.Errorf("SaveTo %s: create temp: %w", outPath, err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	if _, err := tmp.WriteString(out); err != nil {
		tmp.Close()
		return fmt.Errorf("SaveTo %s: write temp: %w", outPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("SaveTo %s: close temp: %w", outPath, err)
	}
	if err := os.Rename(tmpName, outPath); err != nil {
		return fmt.Errorf("SaveTo %s: rename: %w", outPath, err)
	}
	return nil
}

// --- node-building helpers ---

// buildActionNode constructs the YAML node tree for one new action.
//
// Output layout (matching mappings.yaml style):
//
//	    <actionName>:
//	        name: <actionName>
//	        description: <description>
//	        openkore_name: <openkoreName>
//	        canonical_params: []
//	        implementations: []
func buildActionNode(name, description, openkoreName string, impls []Implementation) *yaml.Node {
	mapping := &yaml.Node{Kind: yaml.MappingNode}
	addMappingKV(mapping, "name", name)
	addMappingKV(mapping, "description", description)
	addMappingKV(mapping, "openkore_name", openkoreName)
	// canonical_params is part of the existing schema; preserve it as [].
	addMappingKey(mapping, "canonical_params", &yaml.Node{
		Kind:    yaml.SequenceNode,
		Tag:     "!!seq",
		Content: []*yaml.Node{},
	})
	if len(impls) == 0 {
		addMappingKey(mapping, "implementations", &yaml.Node{
			Kind:    yaml.SequenceNode,
			Tag:     "!!seq",
			Content: []*yaml.Node{},
		})
	} else {
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, impl := range impls {
			seq.Content = append(seq.Content, buildImplNode(impl))
		}
		addMappingKey(mapping, "implementations", seq)
	}
	return mapping
}

// buildImplNode constructs the YAML node tree for one implementation.
//
// Output layout (matching mappings.yaml style):
//
//	    - packet_id: "0x00FB"
//	      packetver_range:
//	        - null
//	        - null
//	      struct_name: PACKET_ZC_GROUP_LIST
//	      field_mapping: {}
func buildImplNode(impl Implementation) *yaml.Node {
	mapping := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	// First key is packet_id (list-item key, on the same line as "- ").
	// The packet_id value is emitted with DoubleQuotedStyle so the output
	// is "0x00FB" rather than the bare 0x00FB yaml.v3 would otherwise
	// produce — matches the quoting convention used throughout the
	// existing mappings.yaml and keeps diffs minimal.
	addMappingQuotedKV(mapping, "packet_id", impl.PacketID)

	// packetver_range as a 2-element sequence (null for unbounded bounds).
	rangeSeq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	rangeSeq.Content = []*yaml.Node{
		rangeValueNode(impl.PacketverMin),
		rangeValueNode(impl.PacketverMax),
	}
	addMappingKey(mapping, "packetver_range", rangeSeq)

	addMappingKV(mapping, "struct_name", impl.StructName)

	// field_mapping — empty inline mapping {} when no field mapping, else a
	// populated mapping node.
	fmNode := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	keys := make([]string, 0, len(impl.FieldMapping))
	for k := range impl.FieldMapping {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		addMappingKV(fmNode, k, impl.FieldMapping[k])
	}
	addMappingKey(mapping, "field_mapping", fmNode)

	return mapping
}

// rangeValueNode produces a scalar node for one packetver_range entry.
// 0 → null; otherwise a bare decimal int.
func rangeValueNode(v int) *yaml.Node {
	if v == 0 {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
	}
	return &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!int",
		Value: fmt.Sprintf("%d", v),
	}
}

// addMappingKV appends a (key, scalar-string-value) pair to a mapping node.
func addMappingKV(m *yaml.Node, key, value string) {
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}

// addMappingQuotedKV appends a (key, double-quoted-string-value) pair. Used
// for packet_id so the output matches the existing mappings.yaml convention
// of writing packet IDs as "0xABCD" rather than bare scalars (which yaml.v3
// would otherwise emit unquoted).
func addMappingQuotedKV(m *yaml.Node, key, value string) {
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value, Style: yaml.DoubleQuotedStyle},
	)
}

// addMappingKey appends a (key, value-node) pair where the value can be any
// kind (sequence, mapping, etc.).
func addMappingKey(m *yaml.Node, key string, value *yaml.Node) {
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		value,
	)
}

// ensureMappingNode guarantees that node is a mapping; if it was a null
// scalar (e.g. an empty action body), it is promoted in place.
func ensureMappingNode(node *yaml.Node) {
	if node.Kind != yaml.MappingNode {
		node.Kind = yaml.MappingNode
		node.Tag = "!!map"
		node.Value = ""
		node.Content = nil
	}
}

// getMappingValue returns the value node for key within mapping m, or nil.
func getMappingValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// setMappingField sets a string-valued key on a mapping node, replacing any
// existing value.
func setMappingField(m *yaml.Node, key, value string) {
	ensureMappingNode(m)
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = &yaml.Node{
				Kind: yaml.ScalarNode, Tag: "!!str", Value: value,
			}
			return
		}
	}
	addMappingKV(m, key, value)
}

// getImplsSeq returns the implementations sequence node on an action node,
// creating an empty one if missing.
func getImplsSeq(actionNode *yaml.Node) *yaml.Node {
	v := getMappingValue(actionNode, "implementations")
	if v == nil {
		v = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		setMappingFieldNode(actionNode, "implementations", v)
	}
	if v.Kind != yaml.SequenceNode {
		v.Kind = yaml.SequenceNode
		v.Tag = "!!seq"
		v.Content = nil
	}
	// Reset any flow style inherited from parsing an empty `[]` literal —
	// we always emit implementations in block style to match the rest of
	// mappings.yaml.
	v.Style = 0
	return v
}

// setMappingFieldNode sets a non-scalar value on a mapping key.
func setMappingFieldNode(m *yaml.Node, key string, value *yaml.Node) {
	ensureMappingNode(m)
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = value
			return
		}
	}
	addMappingKey(m, key, value)
}

// appendImplToAction appends one implementation node to an action's
// implementations: sequence, creating the sequence if absent.
//
// yaml.v3 round-trips the Style flag for parsed nodes: an empty `[]` parses
// as a FlowStyle sequence, and appending a child to it keeps the flow style
// (single-line `[...]` output). The rest of mappings.yaml uses block style
// for the implementations list, so we forcibly reset the style here to keep
// the diff minimal when growing a previously-empty list.
func appendImplToAction(actionNode, implNode *yaml.Node) {
	seq := getImplsSeq(actionNode)
	seq.Style = 0
	seq.Content = append(seq.Content, implNode)
}

// findImplIndex returns the (sequence index, found) for an implementation
// with the given packet_id within an action.
func findImplIndex(actionNode *yaml.Node, packetID string) (int, bool) {
	want := normPacketID(packetID)
	seq := getImplsSeq(actionNode)
	for i, item := range seq.Content {
		pidNode := getMappingValue(item, "packet_id")
		if pidNode == nil {
			continue
		}
		if normPacketID(pidNode.Value) == want {
			return i, true
		}
	}
	return 0, false
}

// currentRange returns the (min, max) currently stored on an impl node.
func currentRange(implNode *yaml.Node) (int, int) {
	rng := getMappingValue(implNode, "packetver_range")
	if rng == nil || rng.Kind != yaml.SequenceNode || len(rng.Content) != 2 {
		return 0, 0
	}
	min, _ := parseRangeItem(rng.Content[0])
	max, _ := parseRangeItem(rng.Content[1])
	return min, max
}

// replaceImplRange overwrites the packetver_range sequence on an impl node.
func replaceImplRange(implNode *yaml.Node, min, max int) {
	newSeq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	newSeq.Content = []*yaml.Node{
		rangeValueNode(min),
		rangeValueNode(max),
	}
	setMappingFieldNode(implNode, "packetver_range", newSeq)
}

// parseRangeItem reads one packetver_range scalar node to an int.
// Returns (value, true) on success.
func parseRangeItem(node *yaml.Node) (int, bool) {
	if node == nil || node.Tag == "!!null" {
		return 0, true
	}
	var n int
	if err := node.Decode(&n); err == nil {
		return n, true
	}
	var s string
	if err := node.Decode(&s); err != nil {
		return 0, false
	}
	var parsed int
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &parsed); err == nil {
		return parsed, true
	}
	return 0, false
}

// appendActionsMappingEntry inserts a new (name → node) pair into the
// top-level `semantic_actions:` mapping of the document.
func appendActionsMappingEntry(root *yaml.Node, name string, node *yaml.Node) {
	actionsMap := mustFindActionsMap(root)
	actionsMap.Content = append(actionsMap.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name},
		node,
	)
}

// removeActionsMappingEntry deletes the (name → node) pair from the
// top-level `semantic_actions:` mapping.
func removeActionsMappingEntry(root *yaml.Node, name string, _ *yaml.Node) error {
	actionsMap := mustFindActionsMap(root)
	for i := 0; i+1 < len(actionsMap.Content); i += 2 {
		if actionsMap.Content[i].Value == name {
			actionsMap.Content = append(actionsMap.Content[:i], actionsMap.Content[i+2:]...)
			return nil
		}
	}
	return fmt.Errorf("removeActionsMappingEntry: key %q not found", name)
}

// mustFindActionsMap returns the *semantic_actions mapping node, panicking
// if the document shape is unexpected. Callers can assume Load already
// validated the shape.
func mustFindActionsMap(root *yaml.Node) *yaml.Node {
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		panic("semanticsdb: root is not a document node")
	}
	topMap := root.Content[0]
	if topMap.Kind != yaml.MappingNode {
		panic("semanticsdb: top-level node is not a mapping")
	}
	for i := 0; i+1 < len(topMap.Content); i += 2 {
		if topMap.Content[i].Value == "semantic_actions" {
			v := topMap.Content[i+1]
			if v.Kind != yaml.MappingNode {
				panic("semanticsdb: semantic_actions is not a mapping")
			}
			return v
		}
	}
	panic("semanticsdb: semantic_actions key not found")
}

// dirOf returns the directory component of path, or "." if path has none.
func dirOf(path string) string {
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[:idx]
	}
	return "."
}
