// Manually implemented — regression test for goKore bug report 0807.
// ActionZcPartyJoinReq must exist and ZcPartyJoinReq_0x02C6 must decode correctly.

package decode_test

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/decode"
	"github.com/lenaxia/rathena-client/pkg/session"
)

// buildZcPartyJoinReq builds a PACKET_ZC_PARTY_JOIN_REQ test packet.
// GCC layout at pv=20200401 (packets_struct.hpp:5082):
//
//	int16  PacketType   offset 0  (2 bytes)
//	int    GRID         offset 2  (4 bytes) — party/group ID
//	char   groupName[24] offset 6  (24 bytes) — NUL-padded
//
// Total: 30 bytes.
func buildZcPartyJoinReq(pid uint16, grid uint32, groupName string) []byte {
	b := make([]byte, 30)
	binary.LittleEndian.PutUint16(b[0:], pid)
	binary.LittleEndian.PutUint32(b[2:], grid)
	copy(b[6:30], groupName)
	return b
}

// TestZcPartyJoinReq_0x02C6_Decode verifies the modern (pv>=20110718) packet ID.
// Source: packets_struct.hpp:5090 DEFINE_PACKET_HEADER(ZC_PARTY_JOIN_REQ, 0x02c6)
// Note: GRID is decoded as []byte (codegen maps C int → []byte); callers use
// binary.LittleEndian.Uint32(e.GRID[:4]).
func TestZcPartyJoinReq_0x02C6_Decode(t *testing.T) {
	data := buildZcPartyJoinReq(0x02C6, 99001, "WarriorParty")
	e := decode.ZcPartyJoinReq_0x02C6(data, 20200401)

	if len(e.GRID) < 4 {
		t.Fatalf("GRID too short: %d bytes, want >= 4", len(e.GRID))
	}
	if got := binary.LittleEndian.Uint32(e.GRID[:4]); got != 99001 {
		t.Errorf("GRID: got %d, want 99001", got)
	}
	if e.GroupName != "WarriorParty" {
		t.Errorf("GroupName: got %q, want %q", e.GroupName, "WarriorParty")
	}
}

// TestZcPartyJoinReq_0x00FE_Decode verifies the legacy (pv<20110718) packet ID.
// Source: packets_struct.hpp:5088 DEFINE_PACKET_HEADER(ZC_PARTY_JOIN_REQ, 0x00fe)
// Same struct layout — only the packet ID differs.
func TestZcPartyJoinReq_0x00FE_Decode(t *testing.T) {
	data := buildZcPartyJoinReq(0x00FE, 12345, "AlphaSquad")
	e := decode.ZcPartyJoinReq_0x00FE(data, 20030000)

	if len(e.GRID) < 4 {
		t.Fatalf("GRID too short: %d bytes, want >= 4", len(e.GRID))
	}
	if got := binary.LittleEndian.Uint32(e.GRID[:4]); got != 12345 {
		t.Errorf("GRID: got %d, want 12345", got)
	}
	if e.GroupName != "AlphaSquad" {
		t.Errorf("GroupName: got %q, want %q", e.GroupName, "AlphaSquad")
	}
}

// TestZcPartyJoinReq_NulPaddedName verifies NUL-padding in groupName is stripped.
func TestZcPartyJoinReq_NulPaddedName(t *testing.T) {
	data := buildZcPartyJoinReq(0x02C6, 1, "Hi") // "Hi" + 22 NUL bytes
	e := decode.ZcPartyJoinReq_0x02C6(data, 20200401)
	if e.GroupName != "Hi" {
		t.Errorf("GroupName: got %q, want %q", e.GroupName, "Hi")
	}
}

// TestActionZcPartyJoinReq_Exists verifies the semantic action constant exists.
// This is the direct regression test: if ActionZcPartyJoinReq is missing,
// this file will not compile and the test suite fails at build time.
func TestActionZcPartyJoinReq_Exists(t *testing.T) {
	_ = session.ActionZcPartyJoinReq
	if session.ActionZcPartyJoinReq == 0 {
		t.Fatal("ActionZcPartyJoinReq == 0 (ActionUnknown) — not assigned a real value")
	}
	if session.ActionZcPartyJoinReq.String() != "ActionZcPartyJoinReq" {
		t.Errorf("String() = %q, want ActionZcPartyJoinReq",
			session.ActionZcPartyJoinReq.String())
	}
}

func BenchmarkZcPartyJoinReq_0x02C6(b *testing.B) {
	data := buildZcPartyJoinReq(0x02C6, 99001, "WarriorParty")
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = decode.ZcPartyJoinReq_0x02C6(data, 20200401)
	}
}
