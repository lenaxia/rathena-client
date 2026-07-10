// Package session — dispatch coverage test for the 0x0B1A variant of
// ZC_USESKILL_ACK (ActionSkillCast).
//
// Validates facts #3 and #4 from the gap report:
//   - Fact #3: lengths_map.go already sets t[0x0B1A] = 29 at pv >= 20191120, so
//     the framer CAN frame it.
//   - Fact #4 (before fix): feeding a 29-byte 0x0B1A frame at packetver 20200401
//     results in unhandled=1 (no semantic handler fires).
//
// After the fix, 0x0B1A routes to ActionSkillCast (alongside 0x07FB) and the
// handler fires exactly once with AttackMT decoded from offset 25.
package session

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/events"
)

// build0x0B1ASkillCastFrame builds a 29-byte 0x0B1A frame with sentinel field
// values, used to verify dispatch + decode correctness end-to-end.
func build0x0B1ASkillCastFrame() []byte {
	b := make([]byte, 29)
	binary.LittleEndian.PutUint16(b[0:], 0x0B1A)
	binary.LittleEndian.PutUint32(b[2:], 0xAABBCCDD)  // srcId
	binary.LittleEndian.PutUint32(b[6:], 0x11223344)  // dstId
	binary.LittleEndian.PutUint16(b[10:], 100)        // x
	binary.LittleEndian.PutUint16(b[12:], 200)        // y
	binary.LittleEndian.PutUint16(b[14:], 42)         // skillId
	binary.LittleEndian.PutUint32(b[16:], 0x55667788) // element
	binary.LittleEndian.PutUint32(b[20:], 1500)       // delayTime
	b[24] = 1                                         // disposable
	binary.LittleEndian.PutUint32(b[25:], 0xCAFEBABE) // attackMT
	return b
}

// TestSkillCast_Dispatch_HasBothVariants verifies ActionSkillCast dispatches
// BOTH the 0x07FB and 0x0B1A packet IDs (the fix is additive).
func TestSkillCast_Dispatch_HasBothVariants(t *testing.T) {
	entries, ok := receiveDispatch[ActionSkillCast]
	if !ok || len(entries) == 0 {
		t.Fatalf("ActionSkillCast has no entries in receiveDispatch")
	}

	want := map[uint16]bool{0x07FB: true, 0x0B1A: true}
	seen := map[uint16]bool{}
	for _, e := range entries {
		seen[e.id] = true
	}
	for pid := range want {
		if !seen[pid] {
			t.Errorf("0x%04X missing from ActionSkillCast dispatch (have %v)", pid, seen)
		}
	}
	if len(entries) != 2 {
		t.Errorf("ActionSkillCast dispatch entry count: got %d want 2", len(entries))
	}
}

// TestSkillCast_0x0B1A_FiresAt_20200401 verifies the end-to-end fix:
// at packetver 20200401 (goKore's version), feeding a 29-byte 0x0B1A frame
// fires ActionSkillCast with all fields — including AttackMT from offset 25 —
// and does NOT increment UnhandledPackets.
func TestSkillCast_0x0B1A_FiresAt_20200401(t *testing.T) {
	pv := uint32(20200401)
	s := NewMapSession(pv)

	// Fact #3 sanity: the framer must already know 0x0B1A = 29 at this pv.
	if got := s.core.lengths[0x0B1A]; got != 29 {
		t.Fatalf("prerequisite failed: lengths[0x0B1A] = %d at pv=%d, want 29 (lengths_map.go gap?)", got, pv)
	}

	var fired int
	var gotEvent events.SkillCast
	RegisterSemanticHandler(s, ActionSkillCast, func(e events.SkillCast) {
		fired++
		gotEvent = e
	})

	frame := build0x0B1ASkillCastFrame()
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed(0x0B1A) error: %v", err)
	}

	if fired != 1 {
		t.Fatalf("ActionSkillCast fired %d times, want 1", fired)
	}
	if unhandled := s.UnhandledPackets(); unhandled != 0 {
		t.Errorf("UnhandledPackets = %d, want 0 (handler should have fired)", unhandled)
	}

	// Verify decode correctness through the dispatch path.
	if gotEvent.SrcId != 0xAABBCCDD {
		t.Errorf("SrcId: got %#x want 0xAABBCCDD", gotEvent.SrcId)
	}
	if gotEvent.DstId != 0x11223344 {
		t.Errorf("DstId: got %#x want 0x11223344", gotEvent.DstId)
	}
	if gotEvent.X != 100 || gotEvent.Y != 200 {
		t.Errorf("X,Y: got %d,%d want 100,200", gotEvent.X, gotEvent.Y)
	}
	if gotEvent.SkillId != 42 {
		t.Errorf("SkillId: got %d want 42", gotEvent.SkillId)
	}
	if gotEvent.Element != 0x55667788 {
		t.Errorf("Element: got %#x want 0x55667788", gotEvent.Element)
	}
	if gotEvent.DelayTime != 1500 {
		t.Errorf("DelayTime: got %d want 1500", gotEvent.DelayTime)
	}
	if gotEvent.Disposable != 1 {
		t.Errorf("Disposable: got %d want 1", gotEvent.Disposable)
	}
	if gotEvent.AttackMT != 0xCAFEBABE {
		t.Errorf("AttackMT: got %#x want 0xCAFEBABE (0x0B1A trailing field at offset 25)", gotEvent.AttackMT)
	}
}

// TestSkillCast_0x07FB_StillFires verifies the 0x07FB variant dispatches to
// ActionSkillCast at packetvers where rAthena actually sends 0x07FB — that is,
// 20091124 ≤ pv < 20181212 (see rAthena src/map/packets_struct.hpp:3965-3977).
// At pv >= 20181212 rAthena binds ZC_USESKILL_ACK to 0x0B1A instead, so this
// test uses a pv in 0x07FB's active range.
func TestSkillCast_0x07FB_StillFires(t *testing.T) {
	pv := uint32(20180101) // 0x07FB is on the wire at this pv (25 bytes)
	s := NewMapSession(pv)

	if got := s.core.lengths[0x07FB]; got != 25 {
		t.Fatalf("prerequisite: lengths[0x07FB] = %d at pv=%d, want 25", got, pv)
	}

	fired := 0
	RegisterSemanticHandler(s, ActionSkillCast, func(e events.SkillCast) {
		fired++
	})

	// 25-byte 0x07FB frame (9 fields, no attackMT — added only in 0x0B1A).
	frame := make([]byte, 25)
	binary.LittleEndian.PutUint16(frame[0:], 0x07FB)
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed(0x07FB) error: %v", err)
	}
	if fired != 1 {
		t.Errorf("ActionSkillCast fired %d times for 0x07FB, want 1", fired)
	}
}
