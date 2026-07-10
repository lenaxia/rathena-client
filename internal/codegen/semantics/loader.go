// Package semantics loads semantics/mappings.yaml into Go structs.
//
// The file has a top-level `semantic_actions:` section that groups packet IDs
// under named actions, each with its rAthena struct name and optional
// packetver bounds. All field derivation is done directly from the rAthena
// VersionTable; there are no canonical_params or field_mapping expressions in
// the consumed schema.
//
// This loader uses gopkg.in/yaml.v3. The previous hand-rolled parser (327
// lines of indent-counting bufio.Scanner logic) was deleted in the Rule 5
// scope change that permits yaml.v3 in internal/ developer tooling.
package semantics

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Implementation is one packet ID variant for a semantic action.
type Implementation struct {
	PacketID     string // e.g. "0x009F" (normalised uppercase hex)
	PacketverMin int    // e.g. 20030000; 0 means "no lower bound"
	PacketverMax int    // 0 means "no upper bound"
	StructName   string // rAthena struct name, e.g. PACKET_ZC_NOTIFY_VANISH
}

// Action is a single semantic action from the semantic_actions section.
type Action struct {
	Name            string
	Implementations []Implementation
}

// DB holds all semantic actions keyed by action name.
type DB struct {
	Actions map[string]*Action
}

// rawAction / rawImpl mirror the on-disk YAML shape so yaml.v3 can decode
// directly. The decoding layer translates these into the exported API types
// and applies the same normalisation rules the hand-parser used:
//   - packet_id is normalised to "0xABCD" (uppercase hex)
//   - packetver_range[0] (min) of null/absent defaults to 20030000
//   - packetver_range[1] (max) of null/absent stays 0 (= no upper bound)
//
// tolerantRange is a custom slice unmarshaler that preserves null positions
// (yaml.v3 skips null items when decoding into []int with a per-item
// UnmarshalYAML hook) and accepts both bare integers (20030000) and quoted
// strings ("20121009") — mappings.yaml is inconsistent on this and the
// previous hand-parser silently accepted both by extracting digits from any
// token.
type tolerantRange []int

func (r *tolerantRange) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.SequenceNode {
		return fmt.Errorf("packetver_range: expected sequence, got %s", value.Tag)
	}
	out := make([]int, 0, len(value.Content))
	for _, item := range value.Content {
		if item.Tag == "!!null" {
			out = append(out, 0)
			continue
		}
		var n int
		if err := item.Decode(&n); err == nil {
			out = append(out, n)
			continue
		}
		var s string
		if err := item.Decode(&s); err != nil {
			return fmt.Errorf("packetver_range entry: %w", err)
		}
		var parsed int
		if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &parsed); err != nil {
			return fmt.Errorf("packetver_range entry %q: %w", s, err)
		}
		out = append(out, parsed)
	}
	*r = out
	return nil
}

type rawImpl struct {
	PacketID       string        `yaml:"packet_id"`
	StructName     string        `yaml:"struct_name"`
	PacketverRange tolerantRange `yaml:"packetver_range"`
}

type rawAction struct {
	Name            string    `yaml:"name"`
	Description     string    `yaml:"description"`
	OpenkoreName    string    `yaml:"openkore_name"`
	Implementations []rawImpl `yaml:"implementations"`
}

type rawFile struct {
	SemanticActions map[string]rawAction `yaml:"semantic_actions"`
}

// LoadFile reads the mappings.yaml file at path and returns the semantic DB.
func LoadFile(path string) (*DB, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var raw rawFile
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(false)
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}

	db := &DB{Actions: make(map[string]*Action, len(raw.SemanticActions))}
	for name, ra := range raw.SemanticActions {
		action := &Action{Name: name}
		for _, ri := range ra.Implementations {
			impl := Implementation{
				PacketID:   normPacketID(ri.PacketID),
				StructName: ri.StructName,
			}
			if len(ri.PacketverRange) >= 1 {
				impl.PacketverMin = ri.PacketverRange[0]
			}
			if len(ri.PacketverRange) >= 2 {
				impl.PacketverMax = ri.PacketverRange[1]
			}
			if impl.PacketverMin == 0 {
				impl.PacketverMin = 20030000
			}
			action.Implementations = append(action.Implementations, impl)
		}
		db.Actions[name] = action
	}
	return db, nil
}

// PacketMapping holds the minimal data needed for the S→C length join pass:
// packet ID, direction (derived from struct name), rAthena struct name, and
// optional packetver bounds constraining which VersionTable ranges to emit.
type PacketMapping struct {
	PacketID      string // e.g. "0x00B0" (normalised uppercase hex)
	Direction     string // "send" or "receive"
	RathenaStruct string // e.g. "PACKET_ZC_PAR_CHANGE"
	PacketverMin  int    // 0 = no lower bound (treat as 20030000)
	PacketverMax  int    // 0 = no upper bound
}

// LoadMappings derives a flat list of PacketMapping from the semantic_actions
// section. Direction is inferred from the struct name prefix:
//   - ZC_ / HC_ / AC_ / SC_ / TC_ prefixes → "receive" (server-to-client)
//   - Everything else                        → "send"    (client-to-server)
//
// SYNTH_ZC_* and SYNTH_HC_* are also treated as receive. packet_* lowercase
// structs represent receive packets by convention.
//
// The same packet ID may appear multiple times if different actions define
// versioned implementations for it (e.g. 0x0092 pre- and post-20170315).
// The join pass in main.go is responsible for resolving conflicts.
func LoadMappings(path string) ([]PacketMapping, error) {
	db, err := LoadFile(path)
	if err != nil {
		return nil, err
	}

	type deduKey struct{ id, structName string }
	seen := make(map[deduKey]bool)
	var results []PacketMapping

	for _, action := range db.Actions {
		for _, impl := range action.Implementations {
			if impl.PacketID == "" || impl.StructName == "" {
				continue
			}
			key := deduKey{impl.PacketID, impl.StructName}
			if seen[key] {
				continue
			}
			seen[key] = true
			results = append(results, PacketMapping{
				PacketID:      impl.PacketID,
				Direction:     inferDirection(impl.StructName),
				RathenaStruct: impl.StructName,
				PacketverMin:  impl.PacketverMin,
				PacketverMax:  impl.PacketverMax,
			})
		}
	}
	return results, nil
}

// inferDirection returns "receive" for server→client structs, "send" otherwise.
func inferDirection(structName string) string {
	upper := strings.ToUpper(structName)
	for _, prefix := range []string{
		"PACKET_ZC_", "PACKET_HC_", "PACKET_AC_", "PACKET_SC_", "PACKET_TC_",
		"SYNTH_ZC_", "SYNTH_HC_",
	} {
		if strings.HasPrefix(upper, prefix) {
			return "receive"
		}
	}
	if strings.HasPrefix(structName, "packet_") {
		return "receive"
	}
	return "send"
}

// normPacketID normalises a packet ID to "0xABCD" form (uppercase hex digits).
func normPacketID(s string) string {
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "0x") {
		return "0x" + strings.ToUpper(lower[2:])
	}
	return "0x" + strings.ToUpper(lower)
}
