package preprocess

import (
	"fmt"
	"sort"
)

// BreakpointVersions extracts all unique PACKETVER dates from preprocessed output
// across multiple sources. The caller should pass the union of all dates extracted
// from the three rAthena headers.
//
// The returned slice is sorted ascending and deduplicated.
func SortBreakpoints(vers []uint32) []uint32 {
	seen := make(map[uint32]bool)
	var out []uint32
	for _, v := range vers {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// layoutEqual returns true if two struct layouts have identical field lists.
// Checks name, type, offset, and size for each field.
func layoutEqual(a, b *StructLayout) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Available != b.Available {
		return false
	}
	if !a.Available && !b.Available {
		return true // both unavailable
	}
	if a.TotalSize != b.TotalSize || len(a.Fields) != len(b.Fields) {
		return false
	}
	for i := range a.Fields {
		af, bf := a.Fields[i], b.Fields[i]
		if af.Name != bf.Name || af.Type != bf.Type || af.Offset != bf.Offset || af.Size != bf.Size {
			return false
		}
	}
	return true
}

// BuildVersionTable constructs the complete version history for every struct
// by comparing struct layouts at successive PACKETVER breakpoints.
//
// versionedDBs is a slice of (packetver, StructDB) pairs sorted ascending by packetver.
// Each entry represents the struct layouts at one PACKETVER.
//
// The algorithm:
//  1. For each struct seen in any version, emit a VersionedLayout range.
//  2. A new range starts whenever the layout changes from the previous breakpoint.
//  3. MaxVer is set to the next breakpoint's MinVer (or 0 = infinity for the last).
func BuildVersionTable(versionedDBs []VersionedDB) (VersionTable, error) {
	if len(versionedDBs) == 0 {
		return nil, fmt.Errorf("no versioned DBs provided")
	}

	// Collect all known struct names.
	structNames := make(map[string]bool)
	for _, vdb := range versionedDBs {
		for name := range vdb.DB {
			structNames[name] = true
		}
	}

	table := make(VersionTable, len(structNames))

	for name := range structNames {
		var ranges []VersionedLayout
		var prev *StructLayout

		for idx, vdb := range versionedDBs {
			cur := vdb.DB[name] // nil if absent at this packetver

			if layoutEqual(prev, cur) {
				// No change — extend the current range (do nothing here; MaxVer updated later).
				continue
			}

			// Layout changed (or first entry). Start a new range.
			// Close the previous range first.
			if len(ranges) > 0 {
				ranges[len(ranges)-1].MaxVer = vdb.Ver
			}

			var layout *StructLayout
			if cur != nil {
				l := *cur // copy
				layout = &l
			}

			ranges = append(ranges, VersionedLayout{
				MinVer: vdb.Ver,
				MaxVer: 0, // will be filled in when next change is found
				Layout: layout,
			})

			prev = cur
			_ = idx
		}

		// The last range extends to infinity (MaxVer = 0).
		table[name] = ranges
	}

	return table, nil
}

// VersionedDB pairs a PACKETVER with the StructDB parsed at that version.
type VersionedDB struct {
	Ver uint32
	DB  StructDB
}

// LayoutAt returns the struct layout for the given struct name at the given packetver.
// Returns nil if the struct is absent at that version.
// Returns a layout with Available=false if the struct has an UNAVAILABLE_STRUCT tombstone.
func (vt VersionTable) LayoutAt(name string, packetver uint32) *StructLayout {
	ranges, ok := vt[name]
	if !ok {
		return nil
	}
	for i := len(ranges) - 1; i >= 0; i-- {
		r := ranges[i]
		if packetver >= r.MinVer && (r.MaxVer == 0 || packetver < r.MaxVer) {
			return r.Layout
		}
	}
	return nil
}
