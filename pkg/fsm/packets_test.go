package fsm

// Phase 1 C→S packet builder golden tests.
//
// The generated pkg/encode/*.go functions are empty stubs. All real C→S packet
// construction for the auth sequence lives in pkg/fsm/packets.go. These tests
// verify byte-level correctness of each builder against GCC-verified struct layouts.
//
// Sources:
//
//	CA_LOGIN (0x0064):       common/packets.hpp PACKET_CA_LOGIN = 55 bytes
//	CH_ENTER (0x0065):       char_clif.cpp:820  raw 17-byte layout
//	CH_SELECT_CHAR (0x0066): common/packets.hpp PACKET_CH_SELECT_CHAR = 3 bytes
//	CH_CHARLIST_REQ (0x09A1):common/packets.hpp PACKET_CH_CHARLIST_REQ = 2 bytes
//	CZ_ENTER2 (0x0436):      clif.cpp:10641     raw 19-byte layout
//	CZ_NOTIFY_ACTORINIT (0x007D): clif.cpp:10742  = 2 bytes
//	CZ_REQUEST_TIME (0x007E):     clif.cpp:11196  = 6 bytes
//	CZ_REQUEST_TIME2 (0x0360):    clif.cpp:11197  = 6 bytes

import (
	"encoding/binary"
	"strings"
	"testing"
)

// ── buildLoginPacket ─────────────────────────────────────────────────────────
//
// struct PACKET_CA_LOGIN at PACKETVER=20180307 (GCC-verified):
//
//	int16  packetType   (0x0064)        offset 0, size 2
//	uint32 version      (packetver)     offset 2, size 4
//	char   username[24]                 offset 6, size 24
//	char   password[24]                 offset 30, size 24
//	uint8  clienttype   (always 0)      offset 54, size 1
//	total = 55 bytes

func TestBuildLoginPacket_FieldLayout(t *testing.T) {
	pkt := buildLoginPacket(20180307, "botijo1", "Melon.77")
	if len(pkt) != 55 {
		t.Fatalf("len=%d want 55", len(pkt))
	}
	if id := binary.LittleEndian.Uint16(pkt[0:2]); id != 0x0064 {
		t.Errorf("packetType=%#04x want 0x0064", id)
	}
	if ver := binary.LittleEndian.Uint32(pkt[2:6]); ver != 20180307 {
		t.Errorf("version=%d want 20180307", ver)
	}
	// username[24] at offset 6: null-terminated "botijo1"
	user := strings.TrimRight(string(pkt[6:30]), "\x00")
	if user != "botijo1" {
		t.Errorf("username=%q want botijo1", user)
	}
	// password[24] at offset 30: null-terminated "Melon.77"
	pass := strings.TrimRight(string(pkt[30:54]), "\x00")
	if pass != "Melon.77" {
		t.Errorf("password=%q want Melon.77", pass)
	}
	// clienttype at offset 54
	if pkt[54] != 0 {
		t.Errorf("clienttype=%d want 0", pkt[54])
	}
}

func TestBuildLoginPacket_VersionField(t *testing.T) {
	// Verify version field encodes different packetvers correctly.
	cases := []uint32{20030000, 20120000, 20170315, 20200401}
	for _, pv := range cases {
		pkt := buildLoginPacket(pv, "u", "p")
		got := binary.LittleEndian.Uint32(pkt[2:6])
		if got != pv {
			t.Errorf("pv=%d: version field=%d want %d", pv, got, pv)
		}
	}
}

func TestBuildLoginPacket_UsernameNullPad(t *testing.T) {
	// Short username must zero-fill remainder of username[24] field.
	pkt := buildLoginPacket(20180307, "a", "b")
	for i := 7; i < 30; i++ {
		if pkt[i] != 0 {
			t.Errorf("username[%d]=%#02x want 0x00 (null padding)", i-6, pkt[i])
		}
	}
	for i := 31; i < 54; i++ {
		if pkt[i] != 0 {
			t.Errorf("password[%d]=%#02x want 0x00 (null padding)", i-30, pkt[i])
		}
	}
}

func TestBuildLoginPacket_UsernameExact24(t *testing.T) {
	// Username exactly 23 chars (fits with null terminator).
	user23 := "123456789012345678901234" // 24 chars — fills the field exactly
	pkt := buildLoginPacket(20180307, user23[:23], "p")
	got := strings.TrimRight(string(pkt[6:30]), "\x00")
	if got != user23[:23] {
		t.Errorf("username=%q want %q", got, user23[:23])
	}
}

// ── buildCharEnterPacket ─────────────────────────────────────────────────────
//
// CH_ENTER (0x0065) raw layout from char_clif.cpp:820:
//
//	int16  packetType   (0x0065)   offset 0, size 2
//	uint32 accountID               offset 2, size 4
//	uint32 sessionID1              offset 6, size 4
//	uint32 sessionID2              offset 10, size 4
//	uint16 ???                     offset 14, size 2   (always 0)
//	uint8  sex                     offset 16, size 1
//	total = 17 bytes

func TestBuildCharEnterPacket_FieldLayout(t *testing.T) {
	const aid = uint32(2000003)
	const sid1 = uint32(0xDEADBEEF)
	const sid2 = uint32(0xCAFEBABE)
	const sex = uint8(1)

	pkt := buildCharEnterPacket(aid, sid1, sid2, sex)
	if len(pkt) != 17 {
		t.Fatalf("len=%d want 17", len(pkt))
	}
	if id := binary.LittleEndian.Uint16(pkt[0:2]); id != 0x0065 {
		t.Errorf("packetType=%#04x want 0x0065", id)
	}
	if got := binary.LittleEndian.Uint32(pkt[2:6]); got != aid {
		t.Errorf("accountID=%d want %d", got, aid)
	}
	if got := binary.LittleEndian.Uint32(pkt[6:10]); got != sid1 {
		t.Errorf("sessionID1=%#x want %#x", got, sid1)
	}
	if got := binary.LittleEndian.Uint32(pkt[10:14]); got != sid2 {
		t.Errorf("sessionID2=%#x want %#x", got, sid2)
	}
	if got := binary.LittleEndian.Uint16(pkt[14:16]); got != 0 {
		t.Errorf("???=%d want 0", got)
	}
	if pkt[16] != sex {
		t.Errorf("sex=%d want %d", pkt[16], sex)
	}
}

func TestBuildCharEnterPacket_FemaleChar(t *testing.T) {
	pkt := buildCharEnterPacket(1, 2, 3, 0) // sex=0 (female)
	if pkt[16] != 0 {
		t.Errorf("sex=%d want 0 (female)", pkt[16])
	}
}

// ── buildSelectCharPacket ────────────────────────────────────────────────────
//
// struct PACKET_CH_SELECT_CHAR at PACKETVER=20180307 (GCC-verified):
//
//	int16 packetType  (0x0066)   offset 0, size 2
//	uint8 slot                   offset 2, size 1
//	total = 3 bytes

func TestBuildSelectCharPacket_Slot0(t *testing.T) {
	pkt := buildSelectCharPacket(0)
	if len(pkt) != 3 {
		t.Fatalf("len=%d want 3", len(pkt))
	}
	if id := binary.LittleEndian.Uint16(pkt[0:2]); id != 0x0066 {
		t.Errorf("packetType=%#04x want 0x0066", id)
	}
	if pkt[2] != 0 {
		t.Errorf("slot=%d want 0", pkt[2])
	}
}

func TestBuildSelectCharPacket_AllSlots(t *testing.T) {
	// rAthena supports char slots 0-8.
	for slot := uint8(0); slot <= 8; slot++ {
		pkt := buildSelectCharPacket(slot)
		if pkt[2] != slot {
			t.Errorf("slot=%d: pkt[2]=%d want %d", slot, pkt[2], slot)
		}
	}
}

// ── buildCharlistReq ─────────────────────────────────────────────────────────
//
// struct PACKET_CH_CHARLIST_REQ at PACKETVER=20180307 (GCC-verified):
//
//	int16 packetType  (0x09A1)   offset 0, size 2
//	total = 2 bytes

func TestBuildCharlistReq_FieldLayout(t *testing.T) {
	pkt := buildCharlistReq()
	if len(pkt) != 2 {
		t.Fatalf("len=%d want 2", len(pkt))
	}
	if id := binary.LittleEndian.Uint16(pkt[0:2]); id != 0x09A1 {
		t.Errorf("packetType=%#04x want 0x09A1", id)
	}
}

// ── buildMapEnterPacket ──────────────────────────────────────────────────────
//
// CZ_ENTER2 (0x0436) raw layout from clif.cpp:10641:
//
//	int16  packetType   (0x0436)   offset 0,  size 2
//	uint32 accountID               offset 2,  size 4
//	uint32 charID                  offset 6,  size 4
//	uint32 sessionID1 (auth code)  offset 10, size 4
//	uint32 clientTick  (0 at connect) offset 14, size 4
//	uint8  sex                     offset 18, size 1
//	total = 19 bytes

func TestBuildMapEnterPacket_FieldLayout(t *testing.T) {
	const aid = uint32(2000003)
	const cid = uint32(150001)
	const sid1 = uint32(0xDEADBEEF)
	const sex = uint8(1)

	pkt := buildMapEnterPacket(aid, cid, sid1, sex)
	if len(pkt) != 19 {
		t.Fatalf("len=%d want 19", len(pkt))
	}
	if id := binary.LittleEndian.Uint16(pkt[0:2]); id != 0x0436 {
		t.Errorf("packetType=%#04x want 0x0436", id)
	}
	if got := binary.LittleEndian.Uint32(pkt[2:6]); got != aid {
		t.Errorf("accountID=%d want %d", got, aid)
	}
	if got := binary.LittleEndian.Uint32(pkt[6:10]); got != cid {
		t.Errorf("charID=%d want %d", got, cid)
	}
	if got := binary.LittleEndian.Uint32(pkt[10:14]); got != sid1 {
		t.Errorf("sessionID1=%#x want %#x", got, sid1)
	}
	// clientTick must be 0 at connect time
	if got := binary.LittleEndian.Uint32(pkt[14:18]); got != 0 {
		t.Errorf("clientTick=%d want 0", got)
	}
	if pkt[18] != sex {
		t.Errorf("sex=%d want %d", pkt[18], sex)
	}
}

func TestBuildMapEnterPacket_ZeroCharID(t *testing.T) {
	// charID may be 0 before the char server echoes the real value.
	pkt := buildMapEnterPacket(1001, 0, 0xABCD, 0)
	if got := binary.LittleEndian.Uint32(pkt[6:10]); got != 0 {
		t.Errorf("charID=%d want 0", got)
	}
	if got := binary.LittleEndian.Uint32(pkt[10:14]); got != 0xABCD {
		t.Errorf("sessionID1=%#x want 0xABCD", got)
	}
}

// ── buildMapLoadedPacket ─────────────────────────────────────────────────────
//
// CZ_NOTIFY_ACTORINIT (0x007D): int16 only = 2 bytes
// Source: clif.cpp:10742

func TestBuildMapLoadedPacket_FieldLayout(t *testing.T) {
	pkt := buildMapLoadedPacket()
	if len(pkt) != 2 {
		t.Fatalf("len=%d want 2", len(pkt))
	}
	if id := binary.LittleEndian.Uint16(pkt[0:2]); id != 0x007D {
		t.Errorf("packetType=%#04x want 0x007D", id)
	}
}

// ── buildTickSyncPacket ──────────────────────────────────────────────────────
//
// CZ_REQUEST_TIME  (0x007E): int16 + uint32 = 6 bytes  (packetver < 20080102)
// CZ_REQUEST_TIME2 (0x0360): int16 + uint32 = 6 bytes  (packetver >= 20080102)
// Source: clif.cpp:11196-11197
//
// The client tick must be 0 at connect time (server ignores it; but the value
// must be present to produce a valid 6-byte packet).

func TestBuildTickSyncPacket_Pre20080102(t *testing.T) {
	// packetver 20070521 → 0x007E
	id, pkt := buildTickSyncPacket(20070521)
	if len(pkt) != 6 {
		t.Fatalf("len=%d want 6", len(pkt))
	}
	if id != 0x007E {
		t.Errorf("returned id=%#04x want 0x007E", id)
	}
	if embeddedID := binary.LittleEndian.Uint16(pkt[0:2]); embeddedID != 0x007E {
		t.Errorf("pkt[0:2]=%#04x want 0x007E", embeddedID)
	}
	if tick := binary.LittleEndian.Uint32(pkt[2:6]); tick != 0 {
		t.Errorf("clientTick=%d want 0", tick)
	}
}

func TestBuildTickSyncPacket_Post20080102(t *testing.T) {
	// packetver 20080102 → 0x0360
	id, pkt := buildTickSyncPacket(20080102)
	if len(pkt) != 6 {
		t.Fatalf("len=%d want 6", len(pkt))
	}
	if id != 0x0360 {
		t.Errorf("returned id=%#04x want 0x0360", id)
	}
	if embeddedID := binary.LittleEndian.Uint16(pkt[0:2]); embeddedID != 0x0360 {
		t.Errorf("pkt[0:2]=%#04x want 0x0360", embeddedID)
	}
}

func TestBuildTickSyncPacket_BoundaryExact(t *testing.T) {
	// Boundary: 20080101 → 0x007E; 20080102 → 0x0360
	id1, _ := buildTickSyncPacket(20080101)
	if id1 != 0x007E {
		t.Errorf("pv=20080101: id=%#04x want 0x007E", id1)
	}
	id2, _ := buildTickSyncPacket(20080102)
	if id2 != 0x0360 {
		t.Errorf("pv=20080102: id=%#04x want 0x0360", id2)
	}
}

func TestBuildTickSyncPacket_Modern(t *testing.T) {
	// packetver 20200401 (live server) → 0x0360
	id, pkt := buildTickSyncPacket(20200401)
	if id != 0x0360 {
		t.Errorf("pv=20200401: id=%#04x want 0x0360", id)
	}
	if len(pkt) != 6 {
		t.Fatalf("len=%d want 6", len(pkt))
	}
}

// ── copyStr ──────────────────────────────────────────────────────────────────

func TestCopyStr_NullTermination(t *testing.T) {
	dst := make([]byte, 8)
	copyStr(dst, "abc")
	if dst[0] != 'a' || dst[1] != 'b' || dst[2] != 'c' {
		t.Errorf("first 3 bytes: got %v want [97 98 99]", dst[:3])
	}
	// All bytes after 'c' must be zero.
	for i := 3; i < 8; i++ {
		if dst[i] != 0 {
			t.Errorf("dst[%d]=%d want 0", i, dst[i])
		}
	}
}

func TestCopyStr_Truncation(t *testing.T) {
	dst := make([]byte, 3)
	copyStr(dst, "abcdef") // truncated to 3 bytes
	if string(dst) != "abc" {
		t.Errorf("got %q want abc", string(dst))
	}
}

func TestCopyStr_Empty(t *testing.T) {
	dst := []byte{0xFF, 0xFF, 0xFF}
	copyStr(dst, "")
	for i, b := range dst {
		if b != 0 {
			t.Errorf("dst[%d]=%#02x want 0x00 for empty string", i, b)
		}
	}
}
