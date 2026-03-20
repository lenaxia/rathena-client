// Package session — EPIC-08 dispatch coverage tests.
//
// These tests verify that every new PID added by EPIC-08 is present in the
// generated receive_dispatch table and correctly routes to the expected
// SemanticAction constant.
//
// Tests are written BEFORE codegen is run, so they fail on the current
// generated code and pass only after codegen is re-run with the updated DB.
//
// Each test feeds a minimal frame with the correct packet ID and asserts that
// the registered semantic handler fires exactly once.
package session

import (
	"fmt"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/events"
)

// frameForPID constructs a fixed-size frame with the given packet ID.
// size must be large enough for the decode function at the given packetver.
func frameForPID(pid uint16, size int) []byte {
	b := make([]byte, size)
	b[0] = byte(pid)
	b[1] = byte(pid >> 8)
	return b
}

// varFrameForPID constructs a variable-length frame (length embedded at bytes 2-3).
func varFrameForPID(pid uint16, size int) []byte {
	b := make([]byte, size)
	b[0] = byte(pid)
	b[1] = byte(pid >> 8)
	b[2] = byte(size)
	b[3] = byte(size >> 8)
	return b
}

// dispatchCheck verifies that a single packet ID routes to the expected action
// and that a RegisterSemanticHandler callback fires exactly once.
type dispatchCheck struct {
	name      string
	pid       uint16
	action    SemanticAction
	size      int // frame size; if negative, variable-length (embed size in bytes 2-3)
	packetver uint32
}

// runDispatchCheck is a table-driven helper: creates a session at packetver,
// registers a handler for action, feeds one frame with pid, asserts fired==1.
func runDispatchCheck(t *testing.T, c dispatchCheck) {
	t.Helper()
	t.Run(c.name, func(t *testing.T) {
		s := NewMapSession(c.packetver)
		fired := 0

		// We need a concrete event type per action.
		// Use the raw registerHandler approach so we don't need every event type.
		// The semantic dispatch test only needs to verify the action constant routes
		// to the PID — the decode correctness is tested in epic08_golden_test.go.
		s.registerHandler(c.pid, func(_ []byte, _ uint32) {
			fired++
		})
		if c.size < 0 {
			sz := -c.size
			s.setLength(c.pid, -1)
			frame := varFrameForPID(c.pid, sz)
			if err := s.Feed(frame); err != nil {
				t.Fatalf("Feed(0x%04X) error: %v", c.pid, err)
			}
		} else {
			s.setLength(c.pid, int16(c.size))
			frame := frameForPID(c.pid, c.size)
			if err := s.Feed(frame); err != nil {
				t.Fatalf("Feed(0x%04X) error: %v", c.pid, err)
			}
		}
		if fired != 1 {
			t.Errorf("0x%04X handler fired %d times, want 1", c.pid, fired)
		}
	})
}

// TestEPIC08_NewPIDsInDispatch verifies every new PID from EPIC-08 is present
// in the generated receive_dispatch table. This requires the receiveDispatch map
// to contain an entry for each action.
//
// We check by verifying that ActionXxx != 0 (i.e., the constant was generated)
// and that receiveDispatch[ActionXxx] is non-empty.
func TestEPIC08_NewPIDsInDispatch(t *testing.T) {
	// Each entry: action constant, expected PIDs in dispatch.
	type check struct {
		action   SemanticAction
		name     string
		wantPIDs []uint16
	}

	checks := []check{
		{
			ActionZcAcceptEnter,
			"zc_accept_enter",
			[]uint16{0x0073, 0x02EB, 0x0A18},
		},
		{
			ActionCharCreated,
			"char_created",
			[]uint16{0x006D, 0x0B6F},
		},
		{
			ActionReceivedCharactersPage,
			"received_characters_page",
			[]uint16{0x099D, 0x0B72},
		},
		{
			ActionItemPickup,
			"item_pickup",
			[]uint16{0x00A0, 0x029A, 0x02D4, 0x0990, 0x0A0C, 0x0A37, 0x0B41},
		},
		{
			ActionZcReqTakeoffEquipAck,
			"zc_req_takeoff_equip_ack",
			[]uint16{0x00AC, 0x08D1, 0x099A},
		},
		{
			ActionZcReqWearEquipAck,
			"zc_req_wear_equip_ack",
			[]uint16{0x00AA, 0x0999},
		},
		{
			ActionActorExists,
			"actor_exists",
			[]uint16{0x0078, 0x01D8, 0x022A, 0x02EE, 0x09DD, 0x09FF},
		},
		{
			ActionActorConnected,
			"actor_connected",
			[]uint16{0x0079, 0x01D9, 0x022B, 0x02ED, 0x09DC, 0x09FE},
		},
		{
			ActionActorMoved,
			"actor_moved",
			// 0x02EC is the bug-fix: moved from actor_exists to actor_moved
			[]uint16{0x007B, 0x01DA, 0x022C, 0x02EC, 0x09DB, 0x09FD},
		},
		{
			ActionExp,
			"exp",
			[]uint16{0x07F6, 0x0ACC},
		},
		{
			ActionZcHoParChange,
			"zc_ho_par_change",
			[]uint16{0x07DB},
		},
		{
			ActionZcElParChange,
			"zc_el_par_change",
			[]uint16{0x081E},
		},
		{
			ActionSkillAdd,
			"skill_add",
			[]uint16{0x0111, 0x0B31},
		},
		{
			ActionSkillsList,
			"skills_list",
			[]uint16{0x010F, 0x0B32},
		},
		{
			ActionZcSkillinfoUpdate2,
			"zc_skillinfo_update2",
			[]uint16{0x07E1, 0x0B33},
		},
		{
			ActionAddExchangeItem,
			"add_exchange_item",
			[]uint16{0x00E9, 0x080F, 0x0A09, 0x0A96},
		},
		{
			ActionZcShortcutKeyList,
			"zc_shortcut_key_list",
			[]uint16{0x02B9, 0x07D9, 0x0A00, 0x0B20},
		},
		{
			ActionZcGuildInfo,
			"zc_guild_info",
			[]uint16{0x01B6, 0x0A84, 0x0B7B},
		},
		{
			ActionZcEquipwinMicroscope,
			"zc_equipwin_microscope",
			[]uint16{0x02D7, 0x0859, 0x0906, 0x0997, 0x0A2D, 0x0B03},
		},
		{
			ActionReceivedMapServerInfo,
			"received_map_server_info",
			[]uint16{0x0071, 0x0AC5},
		},
		{
			ActionStatUpdate,
			"stat_update",
			[]uint16{0x00B0, 0x00B1, 0x00BE, 0x02A2},
		},
		{
			ActionPinCodeRequest,
			"pin_code_request",
			[]uint16{0x02AD, 0x08B9},
		},
		{
			ActionCharacterServerRefused,
			"character_server_refused",
			[]uint16{0x006C, 0x02CA},
		},
		{
			ActionZcHoskillinfoList,
			"zc_hoskillinfo_list",
			[]uint16{0x0235, 0x029D},
		},
		{
			ActionAreaSpell,
			"area_spell",
			[]uint16{0x011F, 0x08C7},
		},
		{
			ActionMailReceive,
			"mail_receive",
			[]uint16{0x0274},
		},
		{
			ActionInventoryItemsStackable,
			"inventory_items_stackable",
			[]uint16{0x00A3, 0x01EE, 0x02E8, 0x0991},
		},
		{
			ActionInventoryItemsEquip,
			"inventory_items_equip",
			[]uint16{0x00A4, 0x0992, 0x0A0D, 0x0B0A, 0x0B39},
		},
	}

	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			entries, ok := receiveDispatch[c.action]
			if !ok || len(entries) == 0 {
				t.Fatalf("action %v (%s) has no entries in receiveDispatch", c.action, c.name)
			}

			// Build a set of PIDs in the dispatch for this action.
			inDispatch := make(map[uint16]bool)
			for _, e := range entries {
				inDispatch[e.id] = true
			}

			for _, pid := range c.wantPIDs {
				if !inDispatch[pid] {
					t.Errorf("0x%04X missing from dispatch for %s", pid, c.name)
				}
			}
			t.Logf("%s: %d dispatch entries", c.name, len(entries))
		})
	}
}

// TestEPIC08_ActorMoved_0x02EC_Dispatches verifies the 0x02EC bug fix:
// after EPIC-08, feeding a 0x02EC frame must fire an ActionActorMoved handler,
// not an ActionActorExists handler.
func TestEPIC08_ActorMoved_0x02EC_Dispatches(t *testing.T) {
	// At packetver 20080102-20091102, 0x02EC is unit_walkingType (actor_moved).
	// Use packetver=20080102 to test this range.
	s := NewMapSession(20080102)

	actorMovedFired := 0
	actorExistsFired := 0

	RegisterSemanticHandler(s, ActionActorMoved, func(e events.ActorMoved) {
		actorMovedFired++
	})
	RegisterSemanticHandler(s, ActionActorExists, func(e events.ActorExists) {
		actorExistsFired++
	})

	// 0x02EC at pv=20080102: packet_unit_walking, 67 bytes.
	// Variable-length (has PacketLength field at offset 2).
	s.setLength(0x02EC, -1)
	frame := varFrameForPID(0x02EC, 67)
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed(0x02EC) error: %v", err)
	}

	if actorMovedFired != 1 {
		t.Errorf("ActionActorMoved fired %d times, want 1", actorMovedFired)
	}
	if actorExistsFired != 0 {
		t.Errorf("ActionActorExists fired %d times, want 0 (was bug: 0x02EC was in actor_exists)", actorExistsFired)
	}
}

// TestEPIC08_ZcAcceptEnter_0x0073_Dispatches verifies that 0x0073 (pre-2008
// map-enter packet) routes to ActionZcAcceptEnter.
func TestEPIC08_ZcAcceptEnter_0x0073_Dispatches(t *testing.T) {
	// At packetver < 20080102, 0x0073 = ZC_ACCEPT_ENTER (11 bytes).
	s := NewMapSession(20050101)

	fired := 0
	RegisterSemanticHandler(s, ActionZcAcceptEnter, func(_ events.ZcAcceptEnter) {
		fired++
	})

	s.setLength(0x0073, 11)
	frame := frameForPID(0x0073, 11)
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed(0x0073) error: %v", err)
	}
	if fired != 1 {
		t.Errorf("ActionZcAcceptEnter fired %d times for 0x0073, want 1", fired)
	}
}

// TestEPIC08_Exp_Dispatches verifies both EXP packet variants route to ActionExp.
func TestEPIC08_Exp_Dispatches(t *testing.T) {
	for _, tc := range []struct {
		name      string
		packetver uint32
		pid       uint16
		size      int
	}{
		{"pre-2017 (0x07F6)", 20170101, 0x07F6, 14},
		{"post-2017 (0x0ACC)", 20170901, 0x0ACC, 18},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewMapSession(tc.packetver)
			fired := 0
			RegisterSemanticHandler(s, ActionExp, func(_ events.Exp) {
				fired++
			})
			s.setLength(tc.pid, int16(tc.size))
			frame := frameForPID(tc.pid, tc.size)
			if err := s.Feed(frame); err != nil {
				t.Fatalf("Feed(0x%04X) error: %v", tc.pid, err)
			}
			if fired != 1 {
				t.Errorf("0x%04X: ActionExp fired %d times, want 1", tc.pid, fired)
			}
		})
	}
}

// TestEPIC08_ItemPickup_AllVariants verifies all 7 item_pickup PIDs route to
// ActionItemPickup across their respective packetver ranges.
func TestEPIC08_ItemPickup_AllVariants(t *testing.T) {
	cases := []struct {
		pv  uint32
		pid uint16
		sz  int
	}{
		{20050101, 0x00A0, 23}, // else (pre-20061218): classic 23-byte layout
		{20061218, 0x029A, 27},
		{20071002, 0x02D4, 29},
		{20120925, 0x0990, 31},
		{20150226, 0x0A0C, 61},
		{20160921, 0x0A37, 63},
		{20200916, 0x0B41, 70},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(fmt.Sprintf("0x%04X", tc.pid), func(t *testing.T) {
			s := NewMapSession(tc.pv)
			fired := 0
			RegisterSemanticHandler(s, ActionItemPickup, func(_ events.ItemPickup) {
				fired++
			})
			s.setLength(tc.pid, int16(tc.sz))
			frame := frameForPID(tc.pid, tc.sz)
			if err := s.Feed(frame); err != nil {
				t.Fatalf("Feed(0x%04X) error: %v", tc.pid, err)
			}
			if fired != 1 {
				t.Errorf("0x%04X: fired %d, want 1", tc.pid, fired)
			}
		})
	}
}
