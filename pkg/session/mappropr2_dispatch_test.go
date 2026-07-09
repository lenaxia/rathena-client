// Package session — dispatch coverage test for the 0x099B variant of
// ZC_MAPPROPERTY_R2 (ActionZcNotifyMapproperty2).
//
// Reproduces issue #9: at PACKETVER >= 20121010 rAthena emits the map-property
// packet as 0x099B with an 8-byte layout (type + flags bitfield), not 0x01D6
// with the 4-byte layout. Before this fix the packet arrived on the wire, was
// framed correctly (lengths_map.go already registered t[0x099B] = 8 inside the
// pv >= 20121212 block), but no semantic handler fired because receiveDispatch
// only knew about 0x01D6.
//
// After the fix, 0x099B routes to ActionZcNotifyMapproperty2 (alongside
// 0x01D6) and the handler fires with Type decoded from offset 2 and Flags
// decoded from offset 4. The 0x01D6 variant still fires and leaves Flags at
// the zero value (the 4-byte layout has no flags bitfield).
//
// rAthena reference: src/map/clif.cpp:6871-6903 (clif_map_property).
package session

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/events"
)

// build0x099BMapPropertyFrame builds an 8-byte 0x099B frame with sentinel
// field values, matching rAthena's clif_map_property() wire layout at
// PACKETVER >= 20121010.
func build0x099BMapPropertyFrame() []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint16(b[0:], 0x099B)        // cmd
	binary.LittleEndian.PutUint16(b[2:], 1)             // type   = MAPPROPERTY_FREEPVPZONE
	binary.LittleEndian.PutUint32(b[4:], 0x00000467)    // flags  = PARTY|GUILD|SIEGE|COUNT_PK|BATTLEFIELD
	return b
}

// TestZcNotifyMapproperty2_Dispatch_HasBothVariants verifies
// ActionZcNotifyMapproperty2 dispatches BOTH the 0x01D6 (legacy, pv <
// 20121010) and 0x099B (modern, pv >= 20121010) packet IDs.
func TestZcNotifyMapproperty2_Dispatch_HasBothVariants(t *testing.T) {
	entries, ok := receiveDispatch[ActionZcNotifyMapproperty2]
	if !ok || len(entries) == 0 {
		t.Fatalf("ActionZcNotifyMapproperty2 has no entries in receiveDispatch")
	}

	want := map[uint16]bool{0x01D6: true, 0x099B: true}
	seen := map[uint16]bool{}
	for _, e := range entries {
		seen[e.id] = true
	}
	for pid := range want {
		if !seen[pid] {
			t.Errorf("0x%04X missing from ActionZcNotifyMapproperty2 dispatch (have %v)", pid, seen)
		}
	}
	if len(entries) != 2 {
		t.Errorf("ActionZcNotifyMapproperty2 dispatch entry count: got %d want 2", len(entries))
	}
}

// TestZcNotifyMapproperty2_0x099B_FiresAt_20200401 is the end-to-end
// reproduction of issue #9: at packetver 20200401, feeding an 8-byte 0x099B
// frame fires ActionZcNotifyMapproperty2 exactly once with Type and Flags
// decoded, and does NOT increment UnhandledPackets.
func TestZcNotifyMapproperty2_0x099B_FiresAt_20200401(t *testing.T) {
	pv := uint32(20200401)
	s := NewMapSession(pv)

	// Prerequisite: the framer must already know 0x099B = 8 at this pv.
	// This entry comes from clif_packetdb.hpp packet(0x099b,8).
	if got := s.core.lengths[0x099B]; got != 8 {
		t.Fatalf("prerequisite failed: lengths[0x099B] = %d at pv=%d, want 8 (lengths_map.go gap?)", got, pv)
	}

	var fired int
	var gotEvent events.ZcNotifyMapproperty2
	RegisterSemanticHandler(s, ActionZcNotifyMapproperty2, func(e events.ZcNotifyMapproperty2) {
		fired++
		gotEvent = e
	})

	frame := build0x099BMapPropertyFrame()
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed(0x099B) error: %v", err)
	}

	if fired != 1 {
		t.Fatalf("ActionZcNotifyMapproperty2 fired %d times, want 1 (issue #9 regression)", fired)
	}
	if unhandled := s.UnhandledPackets(); unhandled != 0 {
		t.Errorf("UnhandledPackets = %d, want 0 (handler should have fired)", unhandled)
	}

	// Verify decode correctness through the dispatch path.
	if gotEvent.Type != 1 {
		t.Errorf("Type: got %d want 1 (MAPPROPERTY_FREEPVPZONE)", gotEvent.Type)
	}
	if gotEvent.Flags != 0x00000467 {
		t.Errorf("Flags: got %#x want 0x00000467 (PARTY|GUILD|SIEGE|COUNT_PK|BATTLEFIELD)", gotEvent.Flags)
	}
}

// TestZcNotifyMapproperty2_0x01D6_StillFires verifies the legacy 0x01D6
// variant still dispatches to ActionZcNotifyMapproperty2 and leaves Flags at
// the zero value (the 4-byte layout has no flags bitfield).
func TestZcNotifyMapproperty2_0x01D6_StillFires(t *testing.T) {
	pv := uint32(20200401)
	s := NewMapSession(pv)

	if got := s.core.lengths[0x01D6]; got != 4 {
		t.Fatalf("prerequisite: lengths[0x01D6] = %d at pv=%d, want 4", got, pv)
	}

	var fired int
	var gotEvent events.ZcNotifyMapproperty2
	RegisterSemanticHandler(s, ActionZcNotifyMapproperty2, func(e events.ZcNotifyMapproperty2) {
		fired++
		gotEvent = e
	})

	// 4-byte 0x01D6 frame: cmd + type only.
	frame := make([]byte, 4)
	binary.LittleEndian.PutUint16(frame[0:], 0x01D6)
	binary.LittleEndian.PutUint16(frame[2:], 7) // arbitrary type value
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed(0x01D6) error: %v", err)
	}
	if fired != 1 {
		t.Errorf("ActionZcNotifyMapproperty2 fired %d times for 0x01D6, want 1", fired)
	}
	if gotEvent.Type != 7 {
		t.Errorf("Type: got %d want 7", gotEvent.Type)
	}
	if gotEvent.Flags != 0 {
		t.Errorf("Flags: got %#x want 0 (0x01D6 has no flags bitfield — must stay zero)", gotEvent.Flags)
	}
}
