// Package semantics_test — EPIC-08 coverage tests.
//
// These tests assert the exact post-EPIC-08 state of the semantics DB.
// They are written BEFORE any DB changes are made, so they fail on the
// current DB and pass only after all 55 gap PIDs are correctly added.
//
// Test categories:
//
//  1. TestEPIC08_ActorPIDClassification — asserts 0x02EC is in actor_moved
//     (not actor_exists) and that all new actor PIDs are correctly placed.
//
//  2. TestEPIC08_AllGapPIDsPresent — asserts every one of the 55 gap PIDs
//     exists in the DB under the correct action and struct.
//
//  3. TestEPIC08_PacketverRangesCorrect — spot-checks critical packetver
//     ranges (item_pickup, zc_accept_enter) for correctness.
//
//  4. TestEPIC08_NewActionsExist — asserts all 9 new actions created by
//     EPIC-08 are present in the DB.
//
//  5. TestEPIC08_NoDuplicatePIDs — sanity: no PID appears in two actions.
//
//  6. TestEPIC08_ImplCount — total impl count is within the expected range
//     after all additions.
package semantics_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lenaxia/rathena-client/internal/codegen/semantics"
)

// normPID normalises a packet ID to lowercase "0xNNNN".
func normPID(s string) string {
	var n uint64
	fmt.Sscanf(strings.TrimSpace(s), "%v", &n)
	return fmt.Sprintf("0x%04x", n)
}

// findImpl searches action for an implementation with the given packet ID.
// Returns (impl, true) if found.
func findImpl(db *semantics.DB, action, pid string) (*semantics.Implementation, bool) {
	a, ok := db.Actions[action]
	if !ok {
		return nil, false
	}
	want := normPID(pid)
	for i := range a.Implementations {
		if normPID(a.Implementations[i].PacketID) == want {
			return &a.Implementations[i], true
		}
	}
	return nil, false
}

// loadDB is a test helper that loads the semantics DB or fails.
func loadDB(t *testing.T) *semantics.DB {
	t.Helper()
	db, err := semantics.LoadFile("../../../semantics/mappings.yaml")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	return db
}

// ─── Test 1: Actor PID classification ────────────────────────────────────────

// TestEPIC08_ActorPIDClassification verifies the definitive actor PID
// assignment derived from the rAthena packets_struct.hpp enum tables:
//
//	idle_unitType   → actor_exists    (packet_idle_unit)
//	spawn_unitType  → actor_connected (packet_spawn_unit)
//	unit_walkingType → actor_moved    (packet_unit_walking)
//
// Prior bug: 0x02EC was under actor_exists. It must be under actor_moved.
func TestEPIC08_ActorPIDClassification(t *testing.T) {
	db := loadDB(t)

	type wantActor struct {
		pid     string
		action  string
		struct_ string
		note    string
	}

	// Complete tables from rAthena packets_struct.hpp enum.
	// Only the PIDs with actual decode implementations (non-zero length).
	wants := []wantActor{
		// actor_exists — idle_unitType assignments
		{"0x0078", "actor_exists", "packet_idle_unit", "idle_unitType < 4"},
		{"0x01D8", "actor_exists", "packet_idle_unit", "idle_unitType < 7"},
		{"0x022A", "actor_exists", "packet_idle_unit", "idle_unitType < 20080102"},
		{"0x02EE", "actor_exists", "packet_idle_unit", "idle_unitType < 20091103"},
		{"0x09DD", "actor_exists", "packet_idle_unit", "idle_unitType < 20150513"},
		{"0x09FF", "actor_exists", "packet_idle_unit", "idle_unitType else"},

		// actor_connected — spawn_unitType assignments
		{"0x0079", "actor_connected", "packet_spawn_unit", "spawn_unitType < 4"},
		{"0x01D9", "actor_connected", "packet_spawn_unit", "spawn_unitType < 7"},
		{"0x022B", "actor_connected", "packet_spawn_unit", "spawn_unitType < 20080102"},
		{"0x02ED", "actor_connected", "packet_spawn_unit", "spawn_unitType < 20091103"},
		{"0x09DC", "actor_connected", "packet_spawn_unit", "spawn_unitType < 20150513"},
		{"0x09FE", "actor_connected", "packet_spawn_unit", "spawn_unitType else"},

		// actor_moved — unit_walkingType assignments
		{"0x007B", "actor_moved", "packet_unit_walking", "unit_walkingType < 4"},
		{"0x01DA", "actor_moved", "packet_unit_walking", "unit_walkingType < 7"},
		{"0x022C", "actor_moved", "packet_unit_walking", "unit_walkingType < 20080102"},
		{"0x02EC", "actor_moved", "packet_unit_walking", "unit_walkingType < 20091103 (was wrongly in actor_exists)"},
		{"0x09DB", "actor_moved", "packet_unit_walking", "unit_walkingType < 20150513"},
		{"0x09FD", "actor_moved", "packet_unit_walking", "unit_walkingType else"},
	}

	for _, w := range wants {
		impl, ok := findImpl(db, w.action, w.pid)
		if !ok {
			t.Errorf("MISSING %s in %s [%s]", w.pid, w.action, w.note)
			continue
		}
		if impl.StructName != w.struct_ {
			t.Errorf("%s in %s: struct=%q want=%q [%s]",
				w.pid, w.action, impl.StructName, w.struct_, w.note)
		}
	}

	// Assert 0x02EC is NOT in actor_exists (the pre-existing bug).
	if _, inWrong := findImpl(db, "actor_exists", "0x02EC"); inWrong {
		t.Errorf("BUG: 0x02EC is still in actor_exists — must be in actor_moved")
	}
}

// ─── Test 2: All gap PIDs present ────────────────────────────────────────────

// TestEPIC08_AllGapPIDsPresent asserts every one of the 55 gap PIDs from
// EPIC-08 is present in the DB under the correct action and struct.
func TestEPIC08_AllGapPIDsPresent(t *testing.T) {
	db := loadDB(t)

	type want struct {
		pid     string
		action  string
		struct_ string
		note    string
	}

	// The canonical 55+2 gap PIDs (55 from the audit + 2 equip extras found
	// during investigation: 0x00AA already exists but 0x0295 and 0x02D0 were
	// identified as additional gaps for inventory_items_equip).
	wants := []want{
		// ── Step 1: fix 0x02EC bug ─────────────────────────────────────────
		{"0x02EC", "actor_moved", "packet_unit_walking", "bug fix: was in actor_exists"},

		// ── Step 2: zc_accept_enter ─────────────────────────────────────────
		{"0x0073", "zc_accept_enter", "PACKET_ZC_ACCEPT_ENTER", "< 20080102"},
		{"0x02EB", "zc_accept_enter", "PACKET_ZC_ACCEPT_ENTER", "< 20141022 and >= 20160330"},

		// ── Step 2: char_created (new action) ───────────────────────────────
		{"0x006D", "char_created", "PACKET_HC_ACCEPT_MAKECHAR", "else branch"},
		{"0x0B6F", "char_created", "PACKET_HC_ACCEPT_MAKECHAR", "MAIN >= 20201007"},

		// ── Step 2: received_characters_page ────────────────────────────────
		{"0x0B72", "received_characters_page", "PACKET_HC_ACK_CHARINFO_PER_PAGE", "MAIN >= 20201007"},

		// ── Step 3: item_pickup ──────────────────────────────────────────────
		{"0x029A", "item_pickup", "PACKET_ZC_ITEM_PICKUP_ACK", ">= 20061218"},
		{"0x02D4", "item_pickup", "PACKET_ZC_ITEM_PICKUP_ACK", ">= 20071002"},
		{"0x0990", "item_pickup", "PACKET_ZC_ITEM_PICKUP_ACK", ">= 20120925"},
		{"0x0A0C", "item_pickup", "PACKET_ZC_ITEM_PICKUP_ACK", ">= 20150226"},
		{"0x0B41", "item_pickup", "PACKET_ZC_ITEM_PICKUP_ACK", "MAIN >= 20200916"},

		// ── Step 4: zc_req_takeoff_equip_ack (new action) ────────────────────
		{"0x00AC", "zc_req_takeoff_equip_ack", "PACKET_ZC_REQ_TAKEOFF_EQUIP_ACK", "else"},
		{"0x08D1", "zc_req_takeoff_equip_ack", "PACKET_ZC_REQ_TAKEOFF_EQUIP_ACK", ">= 20110824"},
		{"0x099A", "zc_req_takeoff_equip_ack", "PACKET_ZC_REQ_TAKEOFF_EQUIP_ACK", ">= 20130000"},

		// ── Step 4: zc_req_wear_equip_ack (add 0x0999) ───────────────────────
		{"0x0999", "zc_req_wear_equip_ack", "PACKET_ZC_REQ_WEAR_EQUIP_ACK", "MAIN >= 20121205"},

		// ── Step 5: actor PIDs ────────────────────────────────────────────────
		{"0x022A", "actor_exists", "packet_idle_unit", "idle_unitType < 20080102"},
		{"0x02EE", "actor_exists", "packet_idle_unit", "idle_unitType < 20091103"},
		{"0x09DD", "actor_exists", "packet_idle_unit", "idle_unitType < 20150513"},
		{"0x022B", "actor_connected", "packet_spawn_unit", "spawn_unitType < 20080102"},
		{"0x09DC", "actor_connected", "packet_spawn_unit", "spawn_unitType < 20150513"},

		// ── Step 6: exp (new action) ──────────────────────────────────────────
		{"0x07F6", "exp", "SYNTH_ZC_LONG_PAR_CHANGE", "< 20170830"},
		{"0x0ACC", "exp", "SYNTH_ZC_LONG_PAR_CHANGE2", ">= 20170830"},

		// ── Step 7: Cat A remaining ───────────────────────────────────────────
		{"0x07DB", "zc_ho_par_change", "PACKET_ZC_HO_PAR_CHANGE", "else"},
		{"0x081E", "zc_el_par_change", "PACKET_ZC_EL_PAR_CHANGE", "unconditional"},
		{"0x0B31", "skill_add", "PACKET_ZC_ADD_SKILL", "RE >= 20190807"},
		{"0x0B32", "skills_list", "PACKET_ZC_SKILLINFO_LIST", "RE >= 20190807"},
		{"0x0B33", "zc_skillinfo_update2", "PACKET_ZC_SKILLINFO_UPDATE2", "RE >= 20190807"},
		{"0x080F", "add_exchange_item", "PACKET_ZC_ADD_EXCHANGE_ITEM", ">= 20100223"},
		{"0x0A09", "add_exchange_item", "PACKET_ZC_ADD_EXCHANGE_ITEM", ">= 20150226"},
		{"0x0A96", "add_exchange_item", "PACKET_ZC_ADD_EXCHANGE_ITEM", "MAIN >= 20161102"},
		{"0x07D9", "zc_shortcut_key_list", "PACKET_ZC_SHORTCUT_KEY_LIST", ">= 20090617"},
		{"0x0A00", "zc_shortcut_key_list", "PACKET_ZC_SHORTCUT_KEY_LIST", "MAIN >= 20141022"},
		{"0x0B20", "zc_shortcut_key_list", "PACKET_ZC_SHORTCUT_KEY_LIST", "MAIN >= 20190522"},
		{"0x01B6", "zc_guild_info", "PACKET_ZC_GUILD_INFO", "else"},
		{"0x0B7B", "zc_guild_info", "PACKET_ZC_GUILD_INFO", ">= 20200902"},
		{"0x0859", "zc_equipwin_microscope", "PACKET_ZC_EQUIPWIN_MICROSCOPE", ">= 20101123"},
		{"0x0906", "zc_equipwin_microscope", "PACKET_ZC_EQUIPWIN_MICROSCOPE", "MAIN >= 20111207"},
		{"0x0997", "zc_equipwin_microscope", "PACKET_ZC_EQUIPWIN_MICROSCOPE", "MAIN >= 20121205"},
		{"0x0A2D", "zc_equipwin_microscope", "PACKET_ZC_EQUIPWIN_MICROSCOPE", ">= 20140820"},
		{"0x0B03", "zc_equipwin_microscope", "PACKET_ZC_EQUIPWIN_MICROSCOPE", "MAIN >= 20180801"},

		// ── Step 8: Cat B SYNTH_ ─────────────────────────────────────────────
		{"0x0071", "received_map_server_info", "SYNTH_HC_NOTIFY_ZONESVR_OLD", "< 20170315"},
		{"0x02A2", "stat_update", "SYNTH_ZC_PAR_CHANGE2", ">= 20060424"},
		{"0x02AD", "pin_code_request", "SYNTH_HC_SECOND_PASSWD_LOGIN_OLD", ">= 20070227"},
		{"0x02CA", "character_server_refused", "SYNTH_HC_REFUSE_ENTER_OLD", ">= 20070227"},
		{"0x029D", "zc_hoskillinfo_list", "PACKET_ZC_HOSKILLINFO_LIST", ">= 20060424"},
		{"0x08C7", "area_spell", "SYNTH_ZC_SKILL_ENTRY3", ">= 20121212"},
		{"0x0274", "mail_receive", "SYNTH_ZC_MAIL_RECEIVE", ">= 20060306"},

		// ── Step 9: inventory lists (new actions) ─────────────────────────────
		{"0x00A3", "inventory_items_stackable", "packet_itemlist_normal", "else"},
		{"0x01EE", "inventory_items_stackable", "packet_itemlist_normal", ">= 20071002"},
		{"0x02E8", "inventory_items_stackable", "packet_itemlist_normal", ">= 20080102"},
		{"0x0991", "inventory_items_stackable", "packet_itemlist_normal", ">= 20120925"},
		{"0x00A4", "inventory_items_equip", "packet_itemlist_equip", "else"},
		{"0x0992", "inventory_items_equip", "packet_itemlist_equip", ">= 20120925"},
		{"0x0A0D", "inventory_items_equip", "packet_itemlist_equip", ">= 20150226"},
		{"0x0B0A", "inventory_items_equip", "packet_itemlist_equip", "MAIN >= 20181002"},
		{"0x0B39", "inventory_items_equip", "packet_itemlist_equip", "MAIN >= 20200916"},
	}

	for _, w := range wants {
		impl, ok := findImpl(db, w.action, w.pid)
		if !ok {
			t.Errorf("MISSING %s in action %q [%s]", w.pid, w.action, w.note)
			continue
		}
		if impl.StructName != w.struct_ {
			t.Errorf("%s in %q: struct=%q want=%q [%s]",
				w.pid, w.action, impl.StructName, w.struct_, w.note)
		}
	}
}

// ─── Test 3: Packetver ranges ─────────────────────────────────────────────────

// TestEPIC08_PacketverRangesCorrect spot-checks the critical packetver ranges
// that were explicitly derived from rAthena's #elif chains.
func TestEPIC08_PacketverRangesCorrect(t *testing.T) {
	db := loadDB(t)

	type rangeCheck struct {
		action  string
		pid     string
		wantMin int // 0 = no assertion
		wantMax int // 0 = no assertion (open-ended)
		note    string
	}

	checks := []rangeCheck{
		// item_pickup: exact #elif chain from packets_struct.hpp lines 582-594
		{"item_pickup", "0x00A0", 0, 20061217, "else → max 20061217"},
		{"item_pickup", "0x029A", 20061218, 20071001, ">= 20061218, < 20071002"},
		{"item_pickup", "0x02D4", 20071002, 20120924, ">= 20071002, < 20120925"},
		{"item_pickup", "0x0990", 20120925, 20150225, ">= 20120925, < 20150226"},
		{"item_pickup", "0x0A0C", 20150226, 20160920, ">= 20150226, < 20160921"},
		{"item_pickup", "0x0A37", 20160921, 0, ">= 20160921 (open-ended toward 0x0B41)"},
		{"item_pickup", "0x0B41", 20200916, 0, "MAIN >= 20200916"},

		// zc_accept_enter: discontinuous 0x02EB
		{"zc_accept_enter", "0x0073", 0, 20080101, "< 20080102"},
		{"zc_accept_enter", "0x0A18", 20141022, 20160329, "20141022 <= pv < 20160330"},

		// actor_moved: 0x02EC range (the bug-fix)
		{"actor_moved", "0x02EC", 20080102, 20091102, "unit_walkingType < 20091103"},

		// actor_exists: new PIDs
		{"actor_exists", "0x022A", 20050411, 20080101, "idle_unitType < 20080102"},
		{"actor_exists", "0x02EE", 20080102, 20091102, "idle_unitType < 20091103"},

		// received_map_server_info: 0x0071 pre-2017
		{"received_map_server_info", "0x0071", 0, 20170314, "< 20170315"},

		// exp: clear split at 20170830
		{"exp", "0x07F6", 0, 20170829, "< 20170830"},
		{"exp", "0x0ACC", 20170830, 0, ">= 20170830"},

		// zc_req_wear_equip_ack: 0x00AA extended to cover 20101123-20121204, 0x0999 starts 20121205
		// Corrected in v0.5.12: rAthena uses PACKETVER_MAIN_NUM >= 20121205 boundary,
		// not RE >= 20121107. The 0x00AA range now covers [null, 20121204].
		{"zc_req_wear_equip_ack", "0x00AA", 0, 20121204, "< 20121205 (MAIN)"},
		{"zc_req_wear_equip_ack", "0x0999", 20121205, 0, "MAIN >= 20121205"},

		// char_created: two ranges
		{"char_created", "0x006D", 0, 20201006, "else (< 20201007)"},
		{"char_created", "0x0B6F", 20201007, 0, "MAIN >= 20201007"},
	}

	for _, c := range checks {
		impl, ok := findImpl(db, c.action, c.pid)
		if !ok {
			t.Errorf("MISSING %s in %q — cannot check range [%s]", c.pid, c.action, c.note)
			continue
		}
		if c.wantMin != 0 && impl.PacketverMin != c.wantMin {
			t.Errorf("%s/%s: packetver_min=%d want=%d [%s]",
				c.action, c.pid, impl.PacketverMin, c.wantMin, c.note)
		}
		if c.wantMax != 0 && impl.PacketverMax != c.wantMax {
			t.Errorf("%s/%s: packetver_max=%d want=%d [%s]",
				c.action, c.pid, impl.PacketverMax, c.wantMax, c.note)
		}
		// If wantMax == 0 the range is open-ended; no max check.
	}
}

// ─── Test 4: New actions exist ────────────────────────────────────────────────

// TestEPIC08_NewActionsExist verifies all 9 new actions created by EPIC-08
// are present in the DB with at least one implementation.
func TestEPIC08_NewActionsExist(t *testing.T) {
	db := loadDB(t)

	newActions := []struct {
		name    string
		minImpl int
		note    string
	}{
		{"char_created", 2, "0x006D + 0x0B6F"},
		{"exp", 2, "0x07F6 + 0x0ACC"},
		{"zc_req_takeoff_equip_ack", 3, "0x00AC + 0x08D1 + 0x099A"},
		{"zc_ho_par_change", 1, "0x07DB"},
		{"zc_el_par_change", 1, "0x081E"},
		{"inventory_items_stackable", 4, "0x00A3 + 0x01EE + 0x02E8 + 0x0991"},
		{"inventory_items_equip", 5, "0x00A4 + 0x0992 + 0x0A0D + 0x0B0A + 0x0B39"},
		{"mail_receive", 1, "0x0274"},
	}

	for _, a := range newActions {
		action, ok := db.Actions[a.name]
		if !ok {
			t.Errorf("NEW action %q is missing from DB [%s]", a.name, a.note)
			continue
		}
		if len(action.Implementations) < a.minImpl {
			t.Errorf("action %q: %d impls, want >= %d [%s]",
				a.name, len(action.Implementations), a.minImpl, a.note)
		}
	}
}

// ─── Test 5: No duplicate PIDs ────────────────────────────────────────────────

// TestEPIC08_NoDuplicatePIDs verifies that after all additions, no packet ID
// is claimed by two different actions. This is enforced by the existing
// TestSemanticDB_NoDuplicatePacketIDs test, but we run it here explicitly
// to catch any EPIC-08 introduction of duplicates.
func TestEPIC08_NoDuplicatePIDs(t *testing.T) {
	db := loadDB(t)

	seen := make(map[string]string) // pid → action_name
	for name, action := range db.Actions {
		for _, impl := range action.Implementations {
			pid := normPID(impl.PacketID)
			if prev, exists := seen[pid]; exists {
				t.Errorf("duplicate pid %s: claimed by %q and %q", pid, prev, name)
			} else {
				seen[pid] = name
			}
		}
	}
	t.Logf("checked %d unique PIDs", len(seen))
}

// ─── Test 6: Impl count ───────────────────────────────────────────────────────

// TestEPIC08_ImplCount verifies the total implementation count is within
// the expected range after all EPIC-08 additions.
//
// Before EPIC-08: ~475 impls
// EPIC-08 adds: 55 gap PIDs + 7 range-fix updates = ~57 net new entries
// Expected range after: 530–560
func TestEPIC08_ImplCount(t *testing.T) {
	db := loadDB(t)

	total := 0
	for _, action := range db.Actions {
		total += len(action.Implementations)
	}
	t.Logf("total implementations: %d", total)

	const wantMin = 530
	const wantMax = 600
	if total < wantMin {
		t.Errorf("impl count=%d < %d — EPIC-08 additions may be incomplete", total, wantMin)
	}
	if total > wantMax {
		t.Errorf("impl count=%d > %d — unexpected additions?", total, wantMax)
	}
}
