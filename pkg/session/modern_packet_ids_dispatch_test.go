// Dispatch tests for the modern packet IDs added in worklog 0092. Each
// test feeds a wire-valid frame at pv=20200401 and asserts the correct
// semantic action fires. These are companion tests to the golden-byte
// decode tests in pkg/decode/modern_packet_ids_test.go.

package session

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/events"
)

func TestPrivateMessage_0x09DE_FiresAtPv20200401(t *testing.T) {
	pv := uint32(20200401)
	s := NewMapSession(pv)

	if got := s.core.lengths[0x09DE]; got != -1 {
		t.Fatalf("prerequisite: lengths[0x09DE] = %d at pv=%d, want -1 (variable-length)", got, pv)
	}

	fired := 0
	var gotEvent events.PrivateMessage
	RegisterSemanticHandler(s, ActionPrivateMessage, func(e events.PrivateMessage) {
		fired++
		gotEvent = e
	})

	const totalLen = 50
	buf := make([]byte, totalLen)
	binary.LittleEndian.PutUint16(buf[0:2], 0x09DE)
	binary.LittleEndian.PutUint16(buf[2:4], totalLen)
	binary.LittleEndian.PutUint32(buf[4:8], 0xDEADBEEF)
	copy(buf[8:32], "Alice")
	buf[32] = 0 // isAdmin
	copy(buf[33:], "Hello")

	if err := s.Feed(buf); err != nil {
		t.Fatalf("Feed(0x09DE) error: %v", err)
	}
	if fired != 1 {
		t.Errorf("ActionPrivateMessage fired %d times for 0x09DE, want 1", fired)
	}
	if gotEvent.SenderGID != 0xDEADBEEF {
		t.Errorf("SenderGID: got 0x%08X, want 0xDEADBEEF", gotEvent.SenderGID)
	}
	if gotEvent.Sender != "Alice" {
		t.Errorf("Sender: got %q, want Alice", gotEvent.Sender)
	}
}

func TestZcAckReqnameall_0x0A30_FiresAtPv20200401(t *testing.T) {
	pv := uint32(20200401)
	s := NewMapSession(pv)

	if got := s.core.lengths[0x0A30]; got != 106 {
		t.Fatalf("prerequisite: lengths[0x0A30] = %d at pv=%d, want 106", got, pv)
	}

	fired := 0
	var gotEvent events.ZcAckReqnameall
	RegisterSemanticHandler(s, ActionZcAckReqnameall, func(e events.ZcAckReqnameall) {
		fired++
		gotEvent = e
	})

	buf := make([]byte, 106)
	binary.LittleEndian.PutUint16(buf[0:2], 0x0A30)
	binary.LittleEndian.PutUint32(buf[2:6], 100001)
	copy(buf[6:30], "Alice")
	copy(buf[30:54], "MyParty")
	copy(buf[54:78], "MyGuild")
	copy(buf[78:102], "Member")
	binary.LittleEndian.PutUint32(buf[102:106], 42)

	if err := s.Feed(buf); err != nil {
		t.Fatalf("Feed(0x0A30) error: %v", err)
	}
	if fired != 1 {
		t.Errorf("ActionZcAckReqnameall fired %d times for 0x0A30, want 1", fired)
	}
	if gotEvent.Gid != 100001 {
		t.Errorf("Gid: got %d, want 100001", gotEvent.Gid)
	}
	if gotEvent.Title_id != 42 {
		t.Errorf("Title_id: got %d, want 42", gotEvent.Title_id)
	}
}

func TestZcChangeGuild_0x0B47_FiresAtPv20200401(t *testing.T) {
	pv := uint32(20200401)
	s := NewMapSession(pv)

	if got := s.core.lengths[0x0B47]; got != 14 {
		t.Fatalf("prerequisite: lengths[0x0B47] = %d at pv=%d, want 14", got, pv)
	}

	fired := 0
	var gotEvent events.ZcChangeGuild
	RegisterSemanticHandler(s, ActionZcChangeGuild, func(e events.ZcChangeGuild) {
		fired++
		gotEvent = e
	})

	buf := make([]byte, 14)
	binary.LittleEndian.PutUint16(buf[0:2], 0x0B47)
	binary.LittleEndian.PutUint32(buf[2:6], 500)
	binary.LittleEndian.PutUint32(buf[6:10], 42)
	binary.LittleEndian.PutUint32(buf[10:14], 2000001)

	if err := s.Feed(buf); err != nil {
		t.Fatalf("Feed(0x0B47) error: %v", err)
	}
	if fired != 1 {
		t.Errorf("ActionZcChangeGuild fired %d times for 0x0B47, want 1", fired)
	}
	if gotEvent.Guild_id != 500 {
		t.Errorf("Guild_id: got %d, want 500", gotEvent.Guild_id)
	}
	if gotEvent.AID != 2000001 {
		t.Errorf("AID: got %d, want 2000001", gotEvent.AID)
	}
}

// Boundary tests: verify each packet ID is registered at the correct pv
// transition. rAthena source pins the transitions at specific dates; the
// codegen's length table must respect them.

func TestModernPacketIDs_TransitionBoundaries(t *testing.T) {
	cases := []struct {
		name      string
		id        uint16
		pvBefore  uint32
		pvAt      uint32
		lenBefore int16
		lenAt     int16
	}{
		// 0x09DE: rAthena packets_struct.hpp @ pv >= 20131204.
		// Before that transition rAthena binds ZC_WHISPER to 0x0097 instead.
		{"0x09DE whisper", 0x09DE, 20131203, 20131204, 0, -1},

		// 0x0A30: rAthena @ pv >= 20150225.
		{"0x0A30 reqnameall", 0x0A30, 20150224, 20150225, 0, 106},

		// 0x0B1F: rAthena @ pv 20190703..20190806 (narrow window,
		// per the source comment "20190619 main exists in first versions,
		// then removed").
		{"0x0B1F change_guild", 0x0B1F, 20190702, 20190703, 0, 14},

		// 0x0B47: rAthena @ pv >= 20190807. Codegen should register at
		// exactly this pv even though the underlying struct's VT range
		// starts earlier at 20190703 — see internal/codegen/main.go
		// buildMapStocJoinPass ver = max(vr.MinVer, mapping.PacketverMin).
		{"0x0B47 change_guild", 0x0B47, 20190806, 20190807, 0, 14},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewMapSession(tc.pvBefore)
			if got := s.core.lengths[tc.id]; got != tc.lenBefore {
				t.Errorf("lengths[0x%04X] at pv=%d: got %d, want %d",
					tc.id, tc.pvBefore, got, tc.lenBefore)
			}
			s2 := NewMapSession(tc.pvAt)
			if got := s2.core.lengths[tc.id]; got != tc.lenAt {
				t.Errorf("lengths[0x%04X] at pv=%d: got %d, want %d",
					tc.id, tc.pvAt, got, tc.lenAt)
			}
		})
	}
}
