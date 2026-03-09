// Package preprocess runs the GCC preprocessor on rAthena C++ headers at each
// PACKETVER breakpoint and parses the flat output into structured field tables.
package preprocess

// Field represents a single field in a packed rAthena struct.
// All sizes are in bytes; offsets are byte offsets from struct start.
type Field struct {
	Name        string // C field name (e.g. "AID", "speed", "PosDir")
	Type        string // C type string (e.g. "uint32", "int16", "uint8[3]")
	BaseType    string // C base type without array suffix (e.g. "uint8")
	Offset      int    // byte offset from struct start
	Size        int    // byte size of this field (0 for flex arrays)
	IsArray     bool
	ArrayLen    int
	IsFlexArray bool   // true for C flexible array member: TYPE name[] (zero-size, variable trailing data)
	Note        string // e.g. "packing=WBUFPOS" or "packing=WBUFPOS2"
}

// StructLayout is the parsed field list for one struct at one PACKETVER.
type StructLayout struct {
	Name      string  // struct name (e.g. "packet_idle_unit")
	Packetver uint32  // PACKETVER this layout was parsed at
	Fields    []Field // in order, zero-indexed
	TotalSize int     // sum of all field sizes
	Available bool    // false if UNAVAILABLE_STRUCT tombstone present
}

// StructDB maps struct names to their layouts for a specific PACKETVER.
type StructDB map[string]*StructLayout

// VersionedLayout pairs a PACKETVER range with a struct layout.
// Range is [MinVer, MaxVer). MaxVer==0 means "no upper bound".
type VersionedLayout struct {
	MinVer uint32
	MaxVer uint32        // 0 = infinity
	Layout *StructLayout // nil if struct is UNAVAILABLE or absent in this range
}

// VersionTable maps struct names to their complete version history.
// Each entry is a sorted (by MinVer) slice of non-overlapping ranges.
type VersionTable map[string][]VersionedLayout

// Breakpoint is a single PACKETVER date where at least one struct changes.
type Breakpoint struct {
	Ver  uint32 // e.g. 20180307
	Prev uint32 // the date just before this breakpoint (Ver-1 unless there is a prior breakpoint)
}
