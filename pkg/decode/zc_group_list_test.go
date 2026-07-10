package decode

import (
	"testing"

	"github.com/lenaxia/rathena-client/pkg/events"
)

// GCC-verified wire layouts for PACKET_ZC_GROUP_LIST (rAthena
// src/map/packets_struct.hpp:2071-2091). Three packet IDs cover three
// PACKETVER-conditional SUB struct layouts:
//
//	pv < 20170524 (oldest):  packet_id 0x00FB
//	    SUB = AID(4) + playerName[24] + mapName[16] + leader(1) + offline(1)
//	    SUB size = 46 bytes
//
//	20170524 <= pv < 20171207:  packet_id 0x0A44
//	    SUB = AID(4) + playerName[24] + mapName[16] + leader(1) + offline(1)
//	          + class_(2) + baseLevel(2)
//	    SUB size = 50 bytes
//
//	pv >= 20171207 (newest, production):  packet_id 0x0AE5
//	    SUB = AID(4) + GID(4) + playerName[24] + mapName[16] + leader(1)
//	          + offline(1) + class_(2) + baseLevel(2)
//	    SUB size = 54 bytes
//
// The outer PACKET_ZC_GROUP_LIST header is constant across all eras:
//
//	packetType(2) + packetLen(2) + partyName[24] = 28 bytes before members[]
//
// Members begin at offset 28; each SUB occupies SUB_size bytes.

// memberWireSize returns the byte size of one PACKET_ZC_GROUP_LIST_SUB at the
// given packetver. Mirrors zcGroupListMemberSize in zc_group_list.go so tests
// can construct golden bytes without import cycles.
func memberWireSize(pv uint32) int {
	switch {
	case pv >= 20171207:
		return 54
	case pv >= 20170524:
		return 50
	default:
		return 46
	}
}

// encodeMemberForTest writes one member into b at offset off for the given
// packetver layout. Used by the tests below to construct golden bytes.
func encodeMemberForTest(b []byte, off int, pv uint32, aid, gid uint32, name, mapName string, leaderByte, offlineByte byte, class, baseLevel int16) {
	cur := off
	putU32LE(b, cur, aid)
	cur += 4
	if pv >= 20171207 {
		putU32LE(b, cur, gid)
		cur += 4
	}
	copy(b[cur:cur+24], name)
	if len(name) < 24 {
		b[cur+len(name)] = 0
	}
	cur += 24
	copy(b[cur:cur+16], mapName)
	if len(mapName) < 16 {
		b[cur+len(mapName)] = 0
	}
	cur += 16
	b[cur] = leaderByte
	cur += 1
	b[cur] = offlineByte
	cur += 1
	if pv >= 20170524 {
		putI16LE(b, cur, class)
		cur += 2
		putI16LE(b, cur, baseLevel)
		cur += 2
	}
}

// TestZcGroupList_0x00FB_OldestLayout covers the pre-20170524 layout (no GID,
// no class/baseLevel fields).
func TestZcGroupList_0x00FB_OldestLayout(t *testing.T) {
	const pv uint32 = 20160101
	memberSize := memberWireSize(pv) // 46
	members := 2
	totalLen := 28 + memberSize*members
	b := make([]byte, totalLen)

	putU16LE(b, 0, 0x00FB)
	putI16LE(b, 2, int16(totalLen))
	copy(b[4:28], "My Party")
	b[27] = 0 // NUL-terminate party name

	// Member 0: Alice (leader, online, no class fields populated at this pv).
	encodeMemberForTest(b, 28, pv,
		1001,              // AID
		0,                 // GID (absent)
		"Alice",           // name
		"prontera.gat",    // mapName
		0,                 // leader byte (0 = leader per clif.cpp:7892)
		0,                 // offline byte (0 = online per clif.cpp:7893)
		0, 0,              // class/baseLevel absent at pv < 20170524
	)
	// Member 1: Bob (normal member, offline).
	encodeMemberForTest(b, 28+memberSize, pv,
		1002,
		0,
		"Bob",
		"geffen.gat",
		1,    // leader byte (1 = normal member)
		1,    // offline byte (1 = offline)
		0, 0,
	)

	e := ZcGroupList_0x00FB(b, pv)
	if e.PartyName != "My Party" {
		t.Errorf("PartyName = %q, want %q", e.PartyName, "My Party")
	}
	if len(e.Members) != 2 {
		t.Fatalf("len(Members) = %d, want 2", len(e.Members))
	}
	m0 := e.Members[0]
	if m0.AID != 1001 {
		t.Errorf("m0.AID = %d, want 1001", m0.AID)
	}
	if m0.GID != 0 {
		t.Errorf("m0.GID = %d, want 0 (absent at pv < 20171207)", m0.GID)
	}
	if m0.Name != "Alice" {
		t.Errorf("m0.Name = %q, want Alice", m0.Name)
	}
	if m0.MapName != "prontera.gat" {
		t.Errorf("m0.MapName = %q, want prontera.gat", m0.MapName)
	}
	if !m0.Leader {
		t.Error("m0.Leader = false, want true (leader byte 0 = leader)")
	}
	if m0.Offline {
		t.Error("m0.Offline = true, want false (offline byte 0 = online)")
	}
	if m0.Class != 0 || m0.BaseLevel != 0 {
		t.Errorf("m0.Class=%d m0.BaseLevel=%d, want 0/0 (absent)", m0.Class, m0.BaseLevel)
	}

	m1 := e.Members[1]
	if m1.AID != 1002 || m1.Name != "Bob" || m1.MapName != "geffen.gat" {
		t.Errorf("m1 wrong: %+v", m1)
	}
	if m1.Leader {
		t.Error("m1.Leader = true, want false (leader byte 1 = normal)")
	}
	if !m1.Offline {
		t.Error("m1.Offline = false, want true (offline byte 1 = offline)")
	}
}

// TestZcGroupList_0x0A44_MidLayout covers the 20170524..20171206 layout
// (class/baseLevel fields added, no GID yet).
func TestZcGroupList_0x0A44_MidLayout(t *testing.T) {
	const pv uint32 = 20171001
	memberSize := memberWireSize(pv) // 50
	members := 1
	totalLen := 28 + memberSize*members
	b := make([]byte, totalLen)

	putU16LE(b, 0, 0x0A44)
	putI16LE(b, 2, int16(totalLen))
	copy(b[4:28], "Party")
	b[9] = 0

	encodeMemberForTest(b, 28, pv,
		42, 0, "Knight", "aldebaran.gat", 1, 0,
		7,   // class = 7 (knight)
		99,  // baseLevel = 99
	)

	e := ZcGroupList_0x0A44(b, pv)
	if len(e.Members) != 1 {
		t.Fatalf("len(Members) = %d, want 1", len(e.Members))
	}
	m := e.Members[0]
	if m.AID != 42 {
		t.Errorf("AID = %d, want 42", m.AID)
	}
	if m.GID != 0 {
		t.Errorf("GID = %d, want 0 (absent at pv < 20171207)", m.GID)
	}
	if m.Class != 7 {
		t.Errorf("Class = %d, want 7", m.Class)
	}
	if m.BaseLevel != 99 {
		t.Errorf("BaseLevel = %d, want 99", m.BaseLevel)
	}
	if m.Leader {
		t.Error("Leader = true, want false (leader byte 1 = normal member)")
	}
}

// TestZcGroupList_0x0AE5_NewestLayout covers the production-target layout
// (pv >= 20171207, includes GID). This is the wire ID at pv=20200401.
func TestZcGroupList_0x0AE5_NewestLayout(t *testing.T) {
	const pv uint32 = 20200401
	memberSize := memberWireSize(pv) // 54
	members := 3
	totalLen := 28 + memberSize*members
	b := make([]byte, totalLen)

	putU16LE(b, 0, 0x0AE5)
	putI16LE(b, 2, int16(totalLen))
	copy(b[4:28], "Farm Party")
	b[14] = 0

	// Three members: leader, normal-online, normal-offline.
	encodeMemberForTest(b, 28, pv,
		1001, 2001, "Leader", "prontera", 0, 0, 14, 150)
	encodeMemberForTest(b, 28+memberSize, pv,
		1002, 2002, "Tank", "prontera", 1, 0, 7, 99)
	encodeMemberForTest(b, 28+2*memberSize, pv,
		1003, 2003, "AfkAco", "", 1, 1, 8, 45)

	e := ZcGroupList_0x0AE5(b, pv)
	if len(e.Members) != 3 {
		t.Fatalf("len(Members) = %d, want 3", len(e.Members))
	}
	if e.PartyName != "Farm Party" {
		t.Errorf("PartyName = %q, want Farm Party", e.PartyName)
	}

	// All members should have non-zero GID (newest layout).
	for i, m := range e.Members {
		if m.GID == 0 {
			t.Errorf("m%d.GID = 0, want non-zero (newest layout)", i)
		}
	}

	if !e.Members[0].Leader {
		t.Error("m0 should be leader")
	}
	if e.Members[1].Leader {
		t.Error("m1 should not be leader")
	}
	if !e.Members[2].Offline {
		t.Error("m2 should be offline")
	}
	if e.Members[0].Class != 14 {
		t.Errorf("m0.Class = %d, want 14", e.Members[0].Class)
	}
	if e.Members[2].MapName != "" {
		t.Errorf("m2.MapName = %q, want empty", e.Members[2].MapName)
	}
}

// TestZcGroupList_EmptyRoster covers a packet with zero members. The packet
// is still legal (4 + 24 = 28 bytes).
func TestZcGroupList_EmptyRoster(t *testing.T) {
	const pv uint32 = 20200401
	totalLen := 28
	b := make([]byte, totalLen)
	putU16LE(b, 0, 0x0AE5)
	putI16LE(b, 2, int16(totalLen))
	copy(b[4:28], "Solo")
	b[8] = 0

	e := ZcGroupList_0x0AE5(b, pv)
	if e.PartyName != "Solo" {
		t.Errorf("PartyName = %q, want Solo", e.PartyName)
	}
	if len(e.Members) != 0 {
		t.Errorf("len(Members) = %d, want 0", len(e.Members))
	}
}

// TestZcGroupList_PacketLengthPropagated verifies the packetLen field is read.
func TestZcGroupList_PacketLengthPropagated(t *testing.T) {
	const pv uint32 = 20200401
	totalLen := 28 + 54 // one member
	b := make([]byte, totalLen)
	putU16LE(b, 0, 0x0AE5)
	putI16LE(b, 2, int16(totalLen))

	e := ZcGroupList_0x0AE5(b, pv)
	if e.PacketLength != int16(totalLen) {
		t.Errorf("PacketLength = %d, want %d", e.PacketLength, totalLen)
	}
}

// TestZcGroupList_TruncatedMemberSlice covers the edge case where the wire
// length doesn't divide evenly by member size. The decoder should drop the
// partial trailing member rather than panic.
func TestZcGroupList_TruncatedMemberSlice(t *testing.T) {
	const pv uint32 = 20200401
	memberSize := 54
	// 1 full member + 10 extra bytes (partial).
	totalLen := 28 + memberSize + 10
	b := make([]byte, totalLen)
	putU16LE(b, 0, 0x0AE5)
	putI16LE(b, 2, int16(totalLen))
	encodeMemberForTest(b, 28, pv, 100, 200, "X", "Y", 1, 0, 1, 1)

	e := ZcGroupList_0x0AE5(b, pv)
	if len(e.Members) != 1 {
		t.Errorf("len(Members) = %d, want 1 (partial trailing member dropped)", len(e.Members))
	}
}

// TestZcGroupList_TruncatedHeaderDoesNotPanic covers the robustness
// requirement that a malformed or malicious server packet must NEVER panic
// the decoder. The framer only guarantees len(data) >= 4 for variable-length
// packets, but this packet's fixed header is 28 bytes. A packet with
// embedded packetLen in [4, 28) must yield a zero-value event, not a
// runtime slice-bounds panic.
func TestZcGroupList_TruncatedHeaderDoesNotPanic(t *testing.T) {
	decoders := []struct {
		name string
		fn   func([]byte, uint32) events.ZcGroupList
	}{
		{"0x00FB", ZcGroupList_0x00FB},
		{"0x0A44", ZcGroupList_0x0A44},
		{"0x0AE5", ZcGroupList_0x0AE5},
	}
	for _, c := range decoders {
		t.Run(c.name+"/len=4", func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("decoder panicked on 4-byte truncated header: %v", r)
				}
			}()
			b := make([]byte, 4)
			putU16LE(b, 0, 0x0AE5)
			putI16LE(b, 2, 4) // embedded length matches actual
			e := c.fn(b, 20200401)
			if e.PartyName != "" || len(e.Members) != 0 {
				t.Errorf("expected zero-value event, got %+v", e)
			}
		})
		t.Run(c.name+"/len=10", func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("decoder panicked on 10-byte truncated header: %v", r)
				}
			}()
			b := make([]byte, 10)
			putU16LE(b, 0, 0x0AE5)
			putI16LE(b, 2, 10)
			e := c.fn(b, 20200401)
			if e.PartyName != "" || len(e.Members) != 0 {
				t.Errorf("expected zero-value event, got %+v", e)
			}
		})
		t.Run(c.name+"/len=27", func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("decoder panicked on 27-byte truncated header: %v", r)
				}
			}()
			b := make([]byte, 27)
			putU16LE(b, 0, 0x0AE5)
			putI16LE(b, 2, 27)
			e := c.fn(b, 20200401)
			if e.PartyName != "" || len(e.Members) != 0 {
				t.Errorf("expected zero-value event, got %+v", e)
			}
		})
	}
}

// BenchmarkZcGroupList_0x0AE5 covers the production-target hot path.
//
// Note: this benchmark reports 1 alloc/op — the unavoidable make([]Member, n)
// for the variable-length roster. This is a documented exception to the 0-alloc
// decode contract, matching the inventory list events (worklog 0066). The
// event struct itself does not escape.
func BenchmarkZcGroupList_0x0AE5(b *testing.B) {
	const pv uint32 = 20200401
	memberSize := 54
	members := 6
	totalLen := 28 + memberSize*members
	buf := make([]byte, totalLen)
	putU16LE(buf, 0, 0x0AE5)
	putI16LE(buf, 2, int16(totalLen))
	for i := 0; i < members; i++ {
		encodeMemberForTest(buf, 28+i*memberSize, pv,
			uint32(1000+i), uint32(2000+i), "Name", "map", 1, 0, 1, 1)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ZcGroupList_0x0AE5(buf, pv)
	}
}

// BenchmarkZcGroupList_0x00FB covers the oldest-layout hot path (smaller
// per-member size; same 1 alloc/op pattern).
func BenchmarkZcGroupList_0x00FB(b *testing.B) {
	const pv uint32 = 20160101
	memberSize := 46
	members := 6
	totalLen := 28 + memberSize*members
	buf := make([]byte, totalLen)
	putU16LE(buf, 0, 0x00FB)
	putI16LE(buf, 2, int16(totalLen))
	for i := 0; i < members; i++ {
		encodeMemberForTest(buf, 28+i*memberSize, pv,
			uint32(1000+i), 0, "Name", "map", 1, 0, 0, 0)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ZcGroupList_0x00FB(buf, pv)
	}
}
