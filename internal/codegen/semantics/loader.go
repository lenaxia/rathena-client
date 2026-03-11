// Package semantics loads semantics/mappings.yaml into Go structs.
// The file contains only a semantic_actions section — groupings of packet IDs
// under named actions with their rAthena struct names. All field derivation is
// done directly from the rAthena VersionTable; there are no canonical_params or
// field_mapping expressions.
package semantics

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Implementation is one packet ID variant for a semantic action.
type Implementation struct {
	PacketID     string // e.g. "0x009F"
	PacketverMin int    // e.g. 20030000
	PacketverMax int    // 0 = no upper bound
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
//
// Expected indentation:
//
//	0  spaces — "semantic_actions:"
//	4  spaces — action name key  (e.g. "    actor_exists:")
//	8  spaces — "implementations:"
//	12 spaces — list item        (e.g. "            - packet_id: ...")
//	14 spaces — continuation     (struct_name, packetver_min, packetver_max)
func parse(lines []string) (*DB, error) {
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

	var curAction *Action
	var curImpl *Implementation
	inImpls := false
	inPacketverRange := false
	packetverRangeIdx := 0

	for i := start; i < len(lines); i++ {
		raw := lines[i]

		// Stop at next top-level key (no leading space).
		if len(raw) > 0 && raw[0] != ' ' && raw[0] != '\t' {
			break
		}

		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := countIndent(raw)

		// Handle packetver_range list items at indent 16 (new MCP format).
		// Format:
		//   packetver_range:   (indent 14)
		//     - null           (indent 16, item 0 = min)
		//     - null           (indent 16, item 1 = max)
		if inPacketverRange && indent == 16 && strings.HasPrefix(trimmed, "- ") && curImpl != nil {
			val := strings.TrimPrefix(trimmed, "- ")
			val = strings.TrimSpace(val)
			if val != "null" && val != "" {
				switch packetverRangeIdx {
				case 0:
					parseIntInto(val, &curImpl.PacketverMin)
				case 1:
					parseIntInto(val, &curImpl.PacketverMax)
				}
			}
			packetverRangeIdx++
			continue
		}

		// Leaving the packetver_range block when indent drops back to 14 or below.
		if inPacketverRange && indent <= 14 {
			inPacketverRange = false
			// Apply default packetver_min if still zero (null in YAML).
			if curImpl != nil && curImpl.PacketverMin == 0 {
				curImpl.PacketverMin = 20030000
			}
		}

		switch {
		case indent == 4 && strings.HasSuffix(trimmed, ":"):
			// New action name.
			name := unquote(strings.TrimSuffix(trimmed, ":"))
			curAction = &Action{Name: name}
			db.Actions[name] = curAction
			inImpls = false
			curImpl = nil

		case indent == 8:
			if trimmed == "implementations:" {
				inImpls = true
			}

		case indent == 12 && inImpls && strings.HasPrefix(trimmed, "- "):
			// Start of a new implementation entry.
			curImpl = &Implementation{}
			if curAction != nil {
				curAction.Implementations = append(curAction.Implementations, *curImpl)
				curImpl = &curAction.Implementations[len(curAction.Implementations)-1]
			}
			rest := strings.TrimPrefix(trimmed, "- ")
			k, v := splitKV(rest)
			setImplField(curImpl, k, v)

		case indent == 14 && curImpl != nil:
			k, v := splitKV(trimmed)
			if k == "packetver_range" {
				inPacketverRange = true
				packetverRangeIdx = 0
			} else {
				setImplField(curImpl, k, v)
			}
		}
	}

	// Apply default packetver_min for any trailing impl that ended in a null range.
	if inPacketverRange && curImpl != nil && curImpl.PacketverMin == 0 {
		curImpl.PacketverMin = 20030000
	}

	return db, nil
}

// PacketMapping holds the minimal data needed for the S→C length join pass:
// packet ID, direction (derived from struct name), and rAthena struct name.
type PacketMapping struct {
	PacketID      string // e.g. "0x00B0" (normalised uppercase hex)
	Direction     string // "send" or "receive"
	RathenaStruct string // e.g. "PACKET_ZC_PAR_CHANGE"
}

// LoadMappings derives a flat list of PacketMapping from the semantic_actions
// section. Direction is inferred from the struct name prefix:
//   - ZC_ / HC_ / AC_ / SC_ / TC_ prefixes → "receive" (server-to-client)
//   - Everything else                        → "send"    (client-to-server)
//
// SYNTH_ZC_* and SYNTH_HC_* are also treated as receive. packet_* lowercase
// structs represent receive packets by convention.
func LoadMappings(path string) ([]PacketMapping, error) {
	db, err := LoadFile(path)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool) // deduplicate packet_id
	var results []PacketMapping

	for _, action := range db.Actions {
		for _, impl := range action.Implementations {
			if impl.PacketID == "" || impl.StructName == "" {
				continue
			}
			key := impl.PacketID
			if seen[key] {
				continue
			}
			seen[key] = true
			results = append(results, PacketMapping{
				PacketID:      normPacketID(impl.PacketID),
				Direction:     inferDirection(impl.StructName),
				RathenaStruct: impl.StructName,
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
	// lowercase packet_* structs (packet_idle_unit, packet_unit_walking, etc.)
	if strings.HasPrefix(structName, "packet_") {
		return "receive"
	}
	return "send"
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

// unquote strips surrounding single or double quotes.
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

func setImplField(impl *Implementation, k, v string) {
	switch k {
	case "packet_id":
		lower := strings.ToLower(v)
		if strings.HasPrefix(lower, "0x") {
			impl.PacketID = "0x" + strings.ToUpper(lower[2:])
		} else {
			impl.PacketID = "0x" + strings.ToUpper(lower)
		}
	case "struct_name":
		impl.StructName = v
	case "packetver_min":
		parseIntInto(v, &impl.PacketverMin)
	case "packetver_max":
		parseIntInto(v, &impl.PacketverMax)
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

// normPacketID normalises a packet ID to "0xABCD" form (uppercase hex digits).
func normPacketID(s string) string {
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "0x") {
		return "0x" + strings.ToUpper(lower[2:])
	}
	return "0x" + strings.ToUpper(lower)
}
