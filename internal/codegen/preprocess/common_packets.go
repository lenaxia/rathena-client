package preprocess

import (
	"regexp"
	"strconv"
	"strings"
)

// CommonPacketEntry describes one packet from common/packets.hpp.
type CommonPacketEntry struct {
	// Name is the PACKET_* struct name (e.g. "PACKET_CA_LOGIN").
	Name string
	// ID is the packet type ID (e.g. 0x0064).
	ID uint16
	// Length is the fixed byte size, or -1 for variable-length packets.
	// Variable-length means the struct contains a flex array (TYPE name[]) or
	// a packetLength field — the frame's bytes [2:4] hold the total length.
	Length int16
	// Prefix is the first two letters of the packet name component
	// (e.g. "CA", "AC", "CH", "HC", "SC", "CT", "TC", "PING").
	Prefix string
}

// reHeaderConst matches the preprocessed form of DEFINE_PACKET_HEADER:
//
//	const int16 HEADER_CA_LOGIN = 0x64;
var reHeaderConst = regexp.MustCompile(`const\s+int16\s+HEADER_(\w+)\s*=\s*(0x[0-9A-Fa-f]+|\d+)\s*;`)

// reFlexArray matches a C flexible array member declaration, e.g.:
//
//	char token[];
//	PACKET_AC_ACCEPT_LOGIN_sub char_servers[];
var reFlexArray = regexp.MustCompile(`\w+\s+\w+\[\s*\]`)

// hasFlexArray returns true if the struct body contains a flex array member.
// A flex array is an array with no specified size: TYPE name[].
func hasFlexArray(body string) bool {
	return reFlexArray.MatchString(body)
}

// packetPrefix extracts the prefix from a packet name like "CA_LOGIN" → "CA",
// "PING" → "PING", "SC_NOTIFY_BAN" → "SC".
func packetPrefix(name string) string {
	idx := strings.Index(name, "_")
	if idx < 0 {
		return name
	}
	return name[:idx]
}

// reNestedScalar matches a scalar field whose type is an identifier not in typeSizes,
// i.e. a nested struct field: "CHARACTER_INFO character;" or "PACKET_X_sub sub;".
// It does NOT match array fields (those have '['), flex arrays are caught separately.
var reNestedScalar = regexp.MustCompile(`^(\w+)\s+(\w+)$`)

// ParseCommonPacketHeaders parses the preprocessed output of common/packets.hpp
// and returns one entry per HEADER_* constant found. For each entry it:
//   - Resolves the corresponding PACKET_* struct size from structDB
//   - Adds sizes for nested struct fields (e.g. CHARACTER_INFO character)
//   - Marks variable-length packets (-1) if the struct has a flex array
//   - Computes the Prefix from the packet name
//
// structDB must be the result of ExtractStructs on the same preprocessed content.
func ParseCommonPacketHeaders(preprocessed string, structDB StructDB) []CommonPacketEntry {
	// First, detect which struct names have flex arrays by re-scanning raw text.
	flexStructs := detectFlexArrayStructs(preprocessed)

	// Build a map of nested struct sizes so we can resolve fields like
	// "CHARACTER_INFO character" that ParseStructBody skips.
	// We use the fully-parsed StructDB to get each struct's TotalSize.
	// For structs that themselves contain nested unknowns (rare in common/packets.hpp),
	// we only go one level deep — sufficient for the known cases.
	nestedSizes := computeNestedStructSizes(preprocessed, structDB)

	var entries []CommonPacketEntry
	for _, line := range strings.Split(preprocessed, "\n") {
		m := reHeaderConst.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		packetName := m[1] // e.g. "CA_LOGIN"
		idStr := m[2]

		id64, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(idStr), "0x"), 16, 16)
		if err != nil {
			continue
		}
		id := uint16(id64)
		structName := "PACKET_" + packetName

		var length int16
		if flexStructs[structName] {
			length = -1
		} else if layout, ok := structDB[structName]; ok && layout != nil && layout.Available {
			// Start with the size of known-type fields (ParseStructBody result).
			total := layout.TotalSize
			// Add any nested struct contributions for this struct.
			if extra, ok := nestedSizes[structName]; ok {
				total += extra
			}
			length = int16(total)
		} else {
			// Unknown or unavailable struct — skip (will be 0 = unknown in table).
			continue
		}

		entries = append(entries, CommonPacketEntry{
			Name:   structName,
			ID:     id,
			Length: length,
			Prefix: packetPrefix(packetName),
		})
	}
	return entries
}

// computeNestedStructSizes scans the preprocessed text body of each PACKET_* struct
// for fields whose type is not a primitive (i.e. a nested struct like CHARACTER_INFO)
// and returns a map of PACKET_name → total extra bytes contributed by nested struct fields.
//
// Only scalar nested struct fields are counted (arrays are handled by flex-array detection).
// The size of the nested struct itself is resolved from structDB or nestedSizes recursively.
func computeNestedStructSizes(preprocessed string, structDB StructDB) map[string]int {
	// Build a map of all struct names → their fully-resolved sizes,
	// including any nested-struct contributions (one level of recursion).
	// We iterate twice: first to collect primitive-only sizes, then resolve nested.

	// Pass 1: collect all struct body lines.
	structBodies := extractAllStructBodies(preprocessed)

	// Pass 2: for each struct, sum sizes of unresolved (nested struct) scalar fields.
	// We resolve against structDB first, then against already-computed sizes.
	result := make(map[string]int)
	for name, bodyLines := range structBodies {
		extra := 0
		for _, rawLine := range bodyLines {
			line := strings.TrimSpace(rawLine)
			line = strings.TrimSuffix(line, ";")
			if line == "" || strings.Contains(line, "[") {
				continue // skip arrays and flex arrays
			}
			m := reNestedScalar.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			typ, _ := m[1], m[2]
			// If the type is already a known primitive, ParseStructBody handles it.
			if _, isPrimitive := typeSizes[typ]; isPrimitive {
				continue
			}
			// Unknown type — check if it's a struct we know about.
			if layout, ok := structDB[typ]; ok && layout != nil && layout.Available {
				extra += layout.TotalSize
				// Recursively add any nested-struct contribution inside this struct.
				if innerExtra, ok := result[typ]; ok {
					extra += innerExtra
				}
			}
		}
		if extra > 0 {
			result[name] = extra
		}
	}
	return result
}

// extractAllStructBodies returns a map of struct name → body lines (between { and })
// for every struct in the preprocessed output.
func extractAllStructBodies(preprocessed string) map[string][]string {
	result := make(map[string][]string)
	lines := strings.Split(preprocessed, "\n")
	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		m := reStructStart.FindStringSubmatch(line)
		if m == nil {
			i++
			continue
		}
		structName := m[1]
		depth := 0
		start := i
		var bodyLines []string
		for i < len(lines) {
			l := lines[i]
			depth += strings.Count(l, "{") - strings.Count(l, "}")
			if i > start {
				bodyLines = append(bodyLines, l)
			}
			if depth == 0 && i > start {
				break
			}
			i++
		}
		i++
		// Trim closing '}' and attribute lines.
		for len(bodyLines) > 0 {
			last := strings.TrimSpace(bodyLines[len(bodyLines)-1])
			if last == "" || strings.HasPrefix(last, "}") || strings.HasPrefix(last, "__attribute__") {
				bodyLines = bodyLines[:len(bodyLines)-1]
			} else {
				break
			}
		}
		result[structName] = bodyLines
	}
	return result
}

// detectFlexArrayStructs scans preprocessed text and returns the set of struct
// names whose body contains at least one flex array member (TYPE name[]).
// This is used to mark variable-length packets in ParseCommonPacketHeaders.
func detectFlexArrayStructs(preprocessed string) map[string]bool {
	result := make(map[string]bool)
	lines := strings.Split(preprocessed, "\n")

	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		m := reStructStart.FindStringSubmatch(line)
		if m == nil {
			i++
			continue
		}
		structName := m[1]

		// Collect body lines until depth returns to 0.
		depth := 0
		start := i
		isFlexible := false
		for i < len(lines) {
			l := lines[i]
			depth += strings.Count(l, "{") - strings.Count(l, "}")
			if i > start && reFlexArray.MatchString(strings.TrimSpace(l)) {
				isFlexible = true
			}
			if depth == 0 && i > start {
				break
			}
			i++
		}
		i++ // advance past closing '}'

		if isFlexible {
			result[structName] = true
		}
	}
	return result
}
