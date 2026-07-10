// Package session — dispatch coverage test for PACKET_ZC_GROUP_LIST
// (ActionZcGroupList), issue #13.
//
// Reproduces issue #13: PACKET_ZC_GROUP_LIST is the only way a client learns
// the full party roster when joining an existing party (rAthena
// src/map/party.cpp:676 calls clif_party_info which emits this packet to the
// joining player). Before this fix the three wire IDs (0x00FB / 0x0A44 /
// 0x0AE5) were framed correctly (lengths_map.go had all three registered as
// variable-length) but no decoder existed, so incoming packets were silently
// dropped as "unknown" with no semantic event fired.
//
// Three wire IDs cover three PACKETVER-conditional SUB layouts:
//
//	0x00FB  pv < 20170524                  — SUB size 46
//	0x0A44  20170524 ≤ pv < 20171207       — SUB size 50
//	0x0AE5  pv ≥ 20171207 (production)     — SUB size 54
//
// rAthena reference: src/map/packets_struct.hpp:271-283 (partyinfo enum),
// :2071-2091 (PACKET_ZC_GROUP_LIST_SUB + PACKET_ZC_GROUP_LIST structs),
// src/map/clif.cpp:7853 (clif_party_info emitter),
// src/map/party.cpp:676 (party_member_added caller).
package session

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/events"
)

// build0x0AE5GroupListFrame builds a 0x0AE5 (production layout) frame with
// one roster member, matching rAthena's clif_party_info() wire layout at
// PACKETVER >= 20171207. SUB size = 54 bytes.
func build0x0AE5GroupListFrame(partyName string, memberAID, memberGID uint32, memberName, mapName string) []byte {
	const memberSize = 54
	totalLen := 28 + memberSize
	b := make([]byte, totalLen)
	binary.LittleEndian.PutUint16(b[0:], 0x0AE5) // cmd
	binary.LittleEndian.PutUint16(b[2:], uint16(totalLen))
	copy(b[4:28], partyName)
	// Member: AID(4) + GID(4) + name[24] + map[16] + leader(1) + offline(1) + class(2) + baseLevel(2)
	off := 28
	binary.LittleEndian.PutUint32(b[off:], memberAID)
	off += 4
	binary.LittleEndian.PutUint32(b[off:], memberGID)
	off += 4
	copy(b[off:off+24], memberName)
	off += 24
	copy(b[off:off+16], mapName)
	off += 16
	b[off] = 0 // leader byte (0 = leader)
	off += 1
	b[off] = 0 // offline byte (0 = online)
	off += 1
	binary.LittleEndian.PutUint16(b[off:], 14) // class = 14 (merchant)
	off += 2
	binary.LittleEndian.PutUint16(b[off:], 99) // baseLevel = 99
	return b
}

// TestZcGroupList_Dispatch_HasAllThreeVariants verifies
// ActionZcGroupList dispatches all three packet IDs (0x00FB, 0x0A44, 0x0AE5).
func TestZcGroupList_Dispatch_HasAllThreeVariants(t *testing.T) {
	entries, ok := receiveDispatch[ActionZcGroupList]
	if !ok || len(entries) == 0 {
		t.Fatalf("ActionZcGroupList has no entries in receiveDispatch")
	}
	want := map[uint16]bool{0x00FB: true, 0x0A44: true, 0x0AE5: true}
	seen := map[uint16]bool{}
	for _, e := range entries {
		seen[e.id] = true
	}
	for pid := range want {
		if !seen[pid] {
			t.Errorf("0x%04X missing from ActionZcGroupList dispatch (have %v)", pid, seen)
		}
	}
	if len(entries) != 3 {
		t.Errorf("ActionZcGroupList dispatch entry count: got %d want 3", len(entries))
	}
}

// TestZcGroupList_0x0AE5_FiresAt_20200401 is the end-to-end reproduction of
// issue #13: at packetver 20200401, feeding a 0x0AE5 frame fires
// ActionZcGroupList exactly once with the roster decoded, and does NOT
// increment UnhandledPackets (the regression symptom: the packet used to be
// silently dropped).
func TestZcGroupList_0x0AE5_FiresAt_20200401(t *testing.T) {
	pv := uint32(20200401)
	s := NewMapSession(pv)

	// Prerequisite: the framer must already know 0x0AE5 = -1 (variable) at
	// this pv. From lengths_map.go:1981 (t[0x0AE5] = -1 if pv >= 20171207).
	if got := s.core.lengths[0x0AE5]; got != -1 {
		t.Fatalf("prerequisite failed: lengths[0x0AE5] = %d at pv=%d, want -1 (lengths_map.go gap?)", got, pv)
	}

	var fired int
	var gotEvent events.ZcGroupList
	RegisterSemanticHandler(s, ActionZcGroupList, func(e events.ZcGroupList) {
		fired++
		gotEvent = e
	})

	frame := build0x0AE5GroupListFrame("FarmParty", 1001, 2001, "Alice", "prontera")
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed(0x0AE5) error: %v", err)
	}

	if fired != 1 {
		t.Fatalf("ActionZcGroupList fired %d times, want 1 (issue #13 regression)", fired)
	}
	if unhandled := s.UnhandledPackets(); unhandled != 0 {
		t.Errorf("UnhandledPackets = %d, want 0 (handler should have fired, not dropped)", unhandled)
	}

	// Verify decode correctness through the dispatch path.
	if gotEvent.PartyName != "FarmParty" {
		t.Errorf("PartyName: got %q want \"FarmParty\"", gotEvent.PartyName)
	}
	if len(gotEvent.Members) != 1 {
		t.Fatalf("len(Members) = %d, want 1", len(gotEvent.Members))
	}
	m := gotEvent.Members[0]
	if m.AID != 1001 {
		t.Errorf("m.AID = %d, want 1001", m.AID)
	}
	if m.GID != 2001 {
		t.Errorf("m.GID = %d, want 2001 (GID present at pv >= 20171207)", m.GID)
	}
	if m.Name != "Alice" {
		t.Errorf("m.Name = %q, want Alice", m.Name)
	}
	if m.MapName != "prontera" {
		t.Errorf("m.MapName = %q, want prontera", m.MapName)
	}
	if !m.Leader {
		t.Error("m.Leader = false, want true (leader byte 0 = leader)")
	}
	if m.Offline {
		t.Error("m.Offline = true, want false (offline byte 0 = online)")
	}
	if m.Class != 14 {
		t.Errorf("m.Class = %d, want 14", m.Class)
	}
	if m.BaseLevel != 99 {
		t.Errorf("m.BaseLevel = %d, want 99", m.BaseLevel)
	}
}

// TestZcGroupList_0x00FB_FiresAt_LegacyPacketver exercises the oldest layout
// (no GID, no class/baseLevel fields) at a packetver that emits 0x00FB.
func TestZcGroupList_0x00FB_FiresAt_LegacyPacketver(t *testing.T) {
	pv := uint32(20160101)
	s := NewMapSession(pv)

	if got := s.core.lengths[0x00FB]; got != -1 {
		t.Fatalf("prerequisite: lengths[0x00FB] = %d at pv=%d, want -1", got, pv)
	}

	var fired int
	RegisterSemanticHandler(s, ActionZcGroupList, func(e events.ZcGroupList) {
		fired++
	})

	// 0x00FB frame with one 46-byte member.
	const memberSize = 46
	totalLen := 28 + memberSize
	b := make([]byte, totalLen)
	binary.LittleEndian.PutUint16(b[0:], 0x00FB)
	binary.LittleEndian.PutUint16(b[2:], uint16(totalLen))
	copy(b[4:28], "LegacyParty")
	// Member body (oldest layout — no GID, no class/baseLevel).
	off := 28
	binary.LittleEndian.PutUint32(b[off:], 4242) // AID
	off += 4
	copy(b[off:off+24], "Bob")
	off += 24
	copy(b[off:off+16], "izlude.gat")
	off += 16
	b[off] = 1 // leader byte (1 = normal)
	off += 1
	b[off] = 1 // offline byte (1 = offline)

	if err := s.Feed(b); err != nil {
		t.Fatalf("Feed(0x00FB) error: %v", err)
	}
	if fired != 1 {
		t.Fatalf("ActionZcGroupList fired %d times for 0x00FB, want 1", fired)
	}
}

// TestZcGroupList_ActionConstant verifies the constant exists and has the
// expected value. Compile-time regression guard for issue #13's reproduction
// ("ActionZcGroupList doesn't exist" was the original bug).
func TestZcGroupList_ActionConstant(t *testing.T) {
	var a SemanticAction = ActionZcGroupList
	if a != 464 {
		t.Errorf("ActionZcGroupList = %d, want 464", a)
	}
	if got := ActionZcGroupList.String(); got != "ActionZcGroupList" {
		t.Errorf("ActionZcGroupList.String() = %q, want \"ActionZcGroupList\"", got)
	}
}
