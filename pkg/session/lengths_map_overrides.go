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
	// 0x009E ZC_ITEM_FALL_ENTRY — packet_dropflooritem
	// packets_struct.hpp:605: `type` field added at PACKETVER >= 20130000 (+2 bytes → 19 total).
	// packets_struct.hpp:132: dropflooritemType switches from 0x009E to 0x084B at PACKETVER > 20130000.
	// Overlap: at exactly PACKETVER == 20130000, the server sends 0x009E with a 19-byte body.
	// clif_packetdb.hpp line 49 hardcodes packet(0x009e, 17) unconditionally.
	// GCC verified: pv=20120925 → 17 bytes; pv=20130000 → 19 bytes (0x009E); pv=20130001 → 19 bytes (0x084B).
	if pv == 20130000 {
		t[0x009E] = 19
	}

	// 0x08C7 (skill_entryType pv 20110718–20121211): packet_skill_entry at this version
	// is 19 bytes. clif_packetdb.hpp hardcodes 20 (off by one).
	// GCC verified: pv=20110718 → 19 bytes; pv=20121212 → 22 bytes (separate ID 0x099F).
	if pv >= 20110718 && pv < 20121212 {
		t[0x08C7] = 19
	}

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

	// 0x099B ZC_MAPPROPERTY_R2 — clif_map_property (clif.cpp:6871-6903).
	// rAthena sends 0x099B with an 8-byte buffer for PACKETVER >= 20121010
	// (clif.cpp:6873-6875: cmd + property + flags), but clif_packetdb.hpp registers
	// packet(0x099b,8) only under #if PACKETVER >= 20130320 (clif_packetdb.hpp:1600,1642).
	// The generated lengths_map.go copies that guard, so the [20121010, 20130320)
	// window has lengths[0x099B] = 0 and the framer would treat 0x099B as unknown.
	// This override closes the rAthena-side packet_db/codegen grouping gap.
	if pv >= 20121010 && pv < 20130320 {
		t[0x099B] = 8
	}
}
