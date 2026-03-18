// Hand-maintained overrides for the map server packet length table.
//
// This file exists because clif_packetdb.hpp hardcodes packet sizes as integer
// literals rather than sizeof() expressions. When a packet struct gains a
// PACKETVER-conditional field that changes its size, clif_packetdb.hpp is not
// updated — so the codegen's diff pass never detects the change and
// lengths_map.go remains wrong for the newer versions.
//
// Each entry here must cite:
//   - the rAthena struct and field that changed
//   - the PACKETVER conditional that governs the change
//   - the GCC-verified sizes at each breakpoint
//
// When the codegen cross-check pass (Part 5, see internal/codegen/main.go) is
// implemented and regenerated, entries here that are now correctly emitted by
// the codegen should be removed.
package session

// applyMapLengthOverrides applies manually-maintained packet length corrections
// on top of the generated populateMapLengths table.
// Called from NewMapSession immediately after populateMapLengths.
func applyMapLengthOverrides(pv uint32, t *[65536]int16) {
	// 0x0ADD ZC_ITEM_FALL_ENTRY5 — packet_dropflooritem
	// packets_struct.hpp:600: ITID is uint16 before PACKETVER_MAIN_NUM >= 20181121,
	// uint32 from that version onward (+2 bytes).
	// GCC verified: pv=20180418 → 22 bytes; pv=20181121 → 24 bytes.
	// clif_packetdb.hpp:1921 hardcodes packet(0x0ADD, 22) and is never updated.
	// Live confirmed: goKore on pay_dun00 PACKETVER=20200401, 24-byte frame parses
	// with zero remainder; old value of 22 left 0x0000 as the next packet ID.
	if pv >= 20181121 {
		t[0x0ADD] = 24
	}
}
