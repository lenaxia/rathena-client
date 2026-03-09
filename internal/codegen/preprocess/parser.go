package preprocess

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// typeSizes maps rAthena C types to their byte sizes in __attribute__((packed)) structs.
var typeSizes = map[string]int{
	"int8":     1,
	"uint8":    1,
	"char":     1,
	"bool":     1,
	"int16":    2,
	"uint16":   2,
	"int32":    4,
	"uint32":   4,
	"float":    4,
	"int64":    8,
	"uint64":   8,
	"double":   8,
	"int":      4, // rAthena uses int (32-bit platform assumption)
	"short":    2,
	"long":     4,
	"int8_t":   1,
	"uint8_t":  1,
	"int16_t":  2,
	"uint16_t": 2,
	"int32_t":  4,
	"uint32_t": 4,
	"int64_t":  8,
	"uint64_t": 8,
}

// specialNote maps specific field names to their packing annotation.
var specialNote = map[string]string{
	"PosDir":   "packing=WBUFPOS",
	"MoveData": "packing=WBUFPOS2",
	"posDir":   "packing=WBUFPOS",
	"moveData": "packing=WBUFPOS2",
}

// arrayField matches: TYPE NAME[EXPR]
// EXPR may be a C constant expression like (23 + 1) or a plain integer.
var reArrayField = regexp.MustCompile(`^(\w+)\s+(\w+)\[([^\]]+)\]$`)

// flexArrayField matches: TYPE NAME[]  (C flexible array member, no size)
var reFlexArrayField = regexp.MustCompile(`^(\w+)\s+(\w+)\[\]$`)

// scalarField matches: TYPE NAME
var reScalarField = regexp.MustCompile(`^(\w+)\s+(\w+)$`)

// nestedStructField matches: struct TYPENAME NAME
var reNestedStructField = regexp.MustCompile(`^struct\s+(\w+)\s+(\w+)$`)

// nestedStructArrayField matches: struct TYPENAME NAME[EXPR]
var reNestedStructArrayField = regexp.MustCompile(`^struct\s+(\w+)\s+(\w+)\[([^\]]+)\]$`)

// nestedStructFlexArrayField matches: struct TYPENAME NAME[]
var reNestedStructFlexArrayField = regexp.MustCompile(`^struct\s+(\w+)\s+(\w+)\[\]$`)

// evalExpr evaluates a simple integer constant expression (the kind GCC produces
// after preprocessing macro substitutions). It handles sums like (23 + 1).
// Returns 0 on any parse error.
func evalExpr(s string) int {
	s = strings.TrimSpace(s)
	// Remove outer parentheses.
	for strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	// Try direct integer parse first.
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	// Simple addition: A + B (only form GCC emits in array sizes after preprocessing).
	if idx := strings.LastIndex(s, "+"); idx != -1 {
		a := evalExpr(s[:idx])
		b := evalExpr(s[idx+1:])
		return a + b
	}
	// Multiplication
	if idx := strings.LastIndex(s, "*"); idx != -1 {
		a := evalExpr(s[:idx])
		b := evalExpr(s[idx+1:])
		return a * b
	}
	return 0
}

// ParseStructBody parses the flat preprocessed body text of a single struct
// (the text between { and }, with all PACKETVER conditionals already resolved).
// Returns a slice of Fields and the total byte size.
// Returns an error if UNAVAILABLE_STRUCT is present in the body.
//
// knownStructs may be nil; if provided, it is used to resolve nested struct
// field sizes (e.g. "struct EQUIPSLOTINFO slot").
func ParseStructBody(body string, structName string, packetver uint32, knownStructs ...StructDB) (*StructLayout, error) {
	if strings.Contains(body, "UNAVAILABLE_STRUCT") {
		return &StructLayout{
			Name:      structName,
			Packetver: packetver,
			Available: false,
		}, nil
	}

	var db StructDB
	if len(knownStructs) > 0 {
		db = knownStructs[0]
	}

	layout := &StructLayout{
		Name:      structName,
		Packetver: packetver,
		Available: true,
	}

	offset := 0
	for _, rawLine := range strings.Split(body, "\n") {
		line := strings.TrimSpace(rawLine)
		line = strings.TrimSuffix(line, ";")
		if line == "" {
			continue
		}

		// Nested struct flex array: struct TYPENAME NAME[]
		if m := reNestedStructFlexArrayField.FindStringSubmatch(line); m != nil {
			typName, name := m[1], m[2]
			layout.Fields = append(layout.Fields, Field{
				Name:        name,
				Type:        fmt.Sprintf("struct %s[]", typName),
				BaseType:    typName,
				Offset:      offset,
				Size:        0,
				IsArray:     true,
				IsFlexArray: true,
			})
			// Flex arrays add no size — they extend to packet end.
			continue
		}

		// Nested struct fixed array: struct TYPENAME NAME[EXPR]
		if m := reNestedStructArrayField.FindStringSubmatch(line); m != nil {
			typName, name, expr := m[1], m[2], m[3]
			count := evalExpr(expr)
			elemSize := 0
			if db != nil {
				if nested, ok := db[typName]; ok && nested != nil && nested.Available {
					elemSize = nested.TotalSize
				}
			}
			if count == 0 || elemSize == 0 {
				// count == 0: unevaluated macro (e.g. MAX_ITEM_OPTIONS) — treat as flex array.
				// elemSize == 0: unknown struct type — skip.
				if count == 0 && elemSize > 0 {
					layout.Fields = append(layout.Fields, Field{
						Name:        name,
						Type:        fmt.Sprintf("struct %s[]", typName),
						BaseType:    typName,
						Offset:      offset,
						Size:        0,
						IsArray:     true,
						IsFlexArray: true,
					})
				}
				// else: unknown struct with fixed count — skip
				continue
			}
			sz := elemSize * count
			layout.Fields = append(layout.Fields, Field{
				Name:     name,
				Type:     fmt.Sprintf("struct %s[%d]", typName, count),
				BaseType: typName,
				Offset:   offset,
				Size:     sz,
				IsArray:  true,
				ArrayLen: count,
			})
			offset += sz
			continue
		}

		// Nested struct scalar: struct TYPENAME NAME
		if m := reNestedStructField.FindStringSubmatch(line); m != nil {
			typName, name := m[1], m[2]
			elemSize := 0
			if db != nil {
				if nested, ok := db[typName]; ok && nested != nil && nested.Available {
					elemSize = nested.TotalSize
				}
			}
			if elemSize > 0 {
				layout.Fields = append(layout.Fields, Field{
					Name:     name,
					Type:     fmt.Sprintf("struct %s", typName),
					BaseType: typName,
					Offset:   offset,
					Size:     elemSize,
				})
				offset += elemSize
			}
			// If elemSize == 0 (unknown struct), skip the field.
			continue
		}

		// Flex array: TYPE NAME[]
		if m := reFlexArrayField.FindStringSubmatch(line); m != nil {
			typ, name := m[1], m[2]
			baseSize := typeSizes[typ] // may be 0 for unknown types — that's fine for flex
			_ = baseSize
			layout.Fields = append(layout.Fields, Field{
				Name:        name,
				Type:        fmt.Sprintf("%s[]", typ),
				BaseType:    typ,
				Offset:      offset,
				Size:        0,
				IsArray:     true,
				IsFlexArray: true,
			})
			// Flex arrays add no size.
			continue
		}

		// Array field: TYPE NAME[EXPR]
		if m := reArrayField.FindStringSubmatch(line); m != nil {
			typ, name, expr := m[1], m[2], m[3]
			count := evalExpr(expr)
			baseSize, known := typeSizes[typ]
			if !known {
				// Unknown type — skip (nested struct declarations, etc.)
				continue
			}
			sz := baseSize * count
			note := specialNote[name]
			layout.Fields = append(layout.Fields, Field{
				Name:     name,
				Type:     fmt.Sprintf("%s[%d]", typ, count),
				BaseType: typ,
				Offset:   offset,
				Size:     sz,
				IsArray:  true,
				ArrayLen: count,
				Note:     note,
			})
			offset += sz
			continue
		}

		// Scalar field: TYPE NAME
		if m := reScalarField.FindStringSubmatch(line); m != nil {
			typ, name := m[1], m[2]
			sz, known := typeSizes[typ]
			if !known {
				continue
			}
			note := specialNote[name]
			layout.Fields = append(layout.Fields, Field{
				Name:     name,
				Type:     typ,
				BaseType: typ,
				Offset:   offset,
				Size:     sz,
				Note:     note,
			})
			offset += sz
			continue
		}
		// Lines we cannot parse (nested struct/union decls, attributes, etc.) — skip.
	}

	layout.TotalSize = offset
	return layout, nil
}
