// gaps_test.go — concrete gap-proving tests.
//
// Each test constructs bytes from GCC-verified struct offsets (not from the
// generated code itself), calls the decode function, and asserts exact values.
// Tests are named TestGap_* so they can be run with -run TestGap.
//
// GCC source for all offsets:
//
//	./validation/struct_layout.sh dump map/packets_struct.hpp <struct> <packetver>
package decode

import (
	"encoding/binary"
	"testing"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

func gapPutU16(b []byte, off int, v uint16) { binary.LittleEndian.PutUint16(b[off:], v) }
func gapPutU32(b []byte, off int, v uint32) { binary.LittleEndian.PutUint32(b[off:], v) }
func gapPutI32(b []byte, off int, v int32)  { binary.LittleEndian.PutUint32(b[off:], uint32(v)) }

// makeIdleUnit_20181121 builds a 108-byte packet_idle_unit buffer at PACKETVER=20181121.
// Layout verified by:
//
//	./validation/struct_layout.sh dump map/packets_struct.hpp packet_idle_unit 20181121
//
//	offset 9  : GID  uint32
//	offset 84 : name char[24]
//	offset 73 : maxHP int32
//	offset 77 : HP int32
//	offset 81 : isBoss uint8
func makeIdleUnit_20181121(packetID uint16, gid uint32, name string) []byte {
	b := make([]byte, 108)
	binary.LittleEndian.PutUint16(b[0:], packetID)
	binary.LittleEndian.PutUint16(b[2:], 108)
	b[4] = 5                                   // objecttype = MOB
	gapPutU32(b, 5, gid+1000)                  // AID = gid+1000 (distinct from GID)
	gapPutU32(b, 9, gid)                       // GID
	binary.LittleEndian.PutUint16(b[13:], 150) // speed
	gapPutI32(b, 73, 9000)                     // maxHP
	gapPutI32(b, 77, 7000)                     // HP
	b[81] = 1                                  // isBoss
	// name at offset 84, 24 bytes, null-terminated
	copy(b[84:108], make([]byte, 24)) // zero-fill first
	copy(b[84:], name)
	return b
}

// ─── Test 1: ActorExists_0x0078 Name field ────────────────────────────────────
//
// BUG PROOF: The SemanticDB for 0x0078 has Name: string("") — a hardcoded empty
// string literal. packet_idle_unit at PACKETVER >= 20181121 HAS a name field at
// offset 84 (GCC-verified). ActorExists_0x0078 NEVER decodes it in any branch.
// This test proves the bug: the actor name "Poring" written into the packet bytes
// at the correct GCC offset is never returned. Name is always "".
func TestGap_0x0078_Name_AlwaysEmpty(t *testing.T) {
	b := makeIdleUnit_20181121(0x0078, 12345, "Poring")
	e := ActorExists_0x0078(b, 20200401) // packetver >= 20181121 branch

	// BUG: Name should be "Poring" (bytes at GCC-verified offset 84) but is ""
	// because the SemanticDB field_mapping has Name: string("") for ALL 0x0078
	// implementations. This is a real correctness bug — mob/NPC names are never
	// returned for this packet ID regardless of packetver.
	if e.Name != "" {
		t.Errorf("ActorExists_0x0078 Name: got %q — UNEXPECTED: field appears decoded now", e.Name)
	} else {
		t.Logf("CONFIRMED BUG: ActorExists_0x0078 Name is always empty string")
		t.Logf("  GCC offset 84 contains %q but decode returns %q", "Poring", e.Name)
	}

	// Also confirm GID IS decoded (not a total failure)
	if e.ID != 12345 {
		t.Errorf("ActorExists_0x0078 ID (GID): got %d want 12345 — GID decode is broken too", e.ID)
	}

	// Also confirm HP is never decoded (also missing from 0x0078 SemanticDB)
	// GCC offset 77 has HP=7000 but 0x0078 maps HP: int32(0)
	if e.HP != 0 {
		t.Errorf("ActorExists_0x0078 HP: got %d — UNEXPECTED: HP appears decoded now", e.HP)
	} else {
		t.Logf("CONFIRMED: ActorExists_0x0078 HP always 0 (SemanticDB maps HP: int32(0))")
	}
}

// ─── Test 2: ActorExists_0x09FF Name field at PACKETVER 20200401 ──────────────
//
// CONTRAST: 0x09FF (the modern replacement) DOES decode Name correctly.
// This test proves the divergence: same struct, same GCC offsets, different
// SemanticDB entries → 0x09FF works, 0x0078 does not.
func TestGap_0x09FF_Name_IsDecoded(t *testing.T) {
	b := makeIdleUnit_20181121(0x09FF, 99999, "Baphomet")
	e := ActorExists_0x09FF(b, 20200401) // packetver >= 20181121 branch

	// 0x09FF maps Name: string(packet.name) — decoded from offset 84
	if e.Name != "Baphomet" {
		t.Errorf("ActorExists_0x09FF Name: got %q want %q", e.Name, "Baphomet")
	} else {
		t.Logf("OK: ActorExists_0x09FF correctly decodes Name=%q from GCC offset 84", e.Name)
	}

	// HP is also decoded for 0x09FF
	if e.HP != 7000 {
		t.Errorf("ActorExists_0x09FF HP: got %d want 7000", e.HP)
	} else {
		t.Logf("OK: ActorExists_0x09FF correctly decodes HP=%d from GCC offset 77", e.HP)
	}
}

// ─── Test 3: ActorMoved_0x09DB Name field ─────────────────────────────────────
//
// packet_unit_walking at PACKETVER >= 20181121 (total=114 bytes) has:
//
//	name char[24] at offset 90 (GCC-verified at 20181121)
//
// SemanticDB actor_moved/0x09DB maps Name as a complex expression:
//
//	"strings.TrimRight(string(packet.name), "\x00")"  — implement manually
//
// This means Name is always "" in all branches.
func TestGap_0x09DB_Name_AlwaysEmpty(t *testing.T) {
	// packet_unit_walking at 20181121 = 114 bytes
	// GCC layout (verified via struct_layout.sh dump):
	//   offset 9  : GID uint32
	//   offset 90 : name char[24]
	b := make([]byte, 114)
	binary.LittleEndian.PutUint16(b[0:], 0x09DB)
	binary.LittleEndian.PutUint16(b[2:], 114)
	gapPutU32(b, 5, 111)                       // AID
	gapPutU32(b, 9, 55555)                     // GID → ID
	binary.LittleEndian.PutUint16(b[13:], 200) // speed
	copy(b[90:], "Raydric")                    // name at offset 90

	e := ActorMoved_0x09DB(b, 20200401)

	// Name is "complex expression — implement manually": always ""
	if e.Name != "" {
		t.Errorf("ActorMoved_0x09DB Name: got %q — UNEXPECTED: field appears decoded now", e.Name)
	} else {
		t.Logf("CONFIRMED: ActorMoved_0x09DB Name always empty (complex expression skip)")
		t.Logf("  GCC offset 90 contains %q but decode returns %q", "Raydric", e.Name)
	}

	// ID should be decoded (GID at offset 9)
	if e.ID != 55555 {
		t.Errorf("ActorMoved_0x09DB ID: got %d want 55555", e.ID)
	}
}

// ─── Test 4: AddExchangeItem_0x00E9 Grade at PACKETVER 20200401 vs 20200902 ───
//
// Grade field is added to PACKET_ZC_ADD_EXCHANGE_ITEM at PACKETVER 20200902.
// At 20200401 the struct is the 20181121 version (no grade field) → Grade=0.
// At 20200902 the struct has grade at offset 36 → Grade decoded.
//
// GCC-verified struct layout at 20200902:
//
//	offset 2  : itemId uint32
//	offset 6  : itemType uint8
//	offset 7  : amount int32
//	offset 11 : identified uint8
//	offset 12 : damaged uint8
//	offset 13 : (refine in older; replaced by card slots)
//	offset 36 : grade uint8    ← NEW at 20200902
func TestGap_AddExchangeItem_Grade_20200401_IsZero(t *testing.T) {
	// Build a packet that would have grade != 0 at offset 36
	// but at packetver 20200401 the 20181121 branch is used (no grade field)
	b := make([]byte, 50)
	binary.LittleEndian.PutUint16(b[0:], 0x00E9)
	gapPutU32(b, 2, 501) // itemId
	b[6] = 4             // itemType
	gapPutI32(b, 7, 3)   // amount
	b[11] = 1            // identified
	b[12] = 0            // damaged
	b[36] = 7            // grade=7 (would be decoded at >= 20200902)

	e20200401 := AddExchangeItem_0x00E9(b, 20200401)
	if e20200401.Grade != 0 {
		t.Errorf("Grade at 20200401: got %d want 0 (field absent before 20200902)", e20200401.Grade)
	} else {
		t.Logf("OK: Grade=0 at PACKETVER 20200401 (correct — field absent in that struct version)")
	}

	// At 20200902 grade IS present at offset 36
	e20200902 := AddExchangeItem_0x00E9(b, 20200902)
	if e20200902.Grade != 7 {
		t.Errorf("Grade at 20200902: got %d want 7 (field present at offset 36)", e20200902.Grade)
	} else {
		t.Logf("OK: Grade=%d at PACKETVER 20200902 (correctly decoded from GCC offset 36)", e20200902.Grade)
	}

	// Verify other fields decoded at 20200401
	if e20200401.ID != 501 {
		t.Errorf("ID at 20200401: got %d want 501", e20200401.ID)
	}
}

// ─── Test 5: ZcPositionIdNameInfo_0x0166 PosInfo is nil ──────────────────────
//
// PACKET_ZC_POSITION_ID_NAME_INFO (variable length):
//
//	offset 0: PacketType int16
//	offset 2: PacketLength int16
//	offset 4: positionID int   (guild position data starts here)
//
// SemanticDB maps PosInfo: "data[4:]" — complex expression, implement manually.
// Generated code comments it out. The event PosInfo field is always nil/empty.
//
// This means guild position names are never decoded — relevant for guild UI.
func TestGap_ZcPositionIdNameInfo_PosInfo_IsNil(t *testing.T) {
	// Build a variable-length packet with real payload data at offset 4
	payload := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	totalLen := 4 + len(payload)
	b := make([]byte, totalLen)
	binary.LittleEndian.PutUint16(b[0:], 0x0166)
	binary.LittleEndian.PutUint16(b[2:], uint16(totalLen))
	copy(b[4:], payload)

	e := ZcPositionIdNameInfo_0x0166(b, 20200401)

	// PosInfo is complex expression — always nil/empty regardless of packet content
	if len(e.PosInfo) != 0 {
		t.Errorf("ZcPositionIdNameInfo PosInfo: got len=%d want 0 — UNEXPECTED: field appears decoded now", len(e.PosInfo))
	} else {
		t.Logf("CONFIRMED GAP: ZcPositionIdNameInfo PosInfo is always nil")
		t.Logf("  %d bytes of guild position data at offset 4 are silently discarded", len(payload))
	}

	// PacketType and PacketLength ARE decoded
	if e.PacketType != 0x0166 {
		t.Errorf("PacketType: got 0x%X want 0x0166", e.PacketType)
	}
	if e.PacketLength != int16(totalLen) {
		t.Errorf("PacketLength: got %d want %d", e.PacketLength, totalLen)
	}
}

// ─── Note on CA_LOGIN byte-level tests ───────────────────────────────────────
//
// CA_LOGIN (0x0064) byte-level correctness is fully covered in
// pkg/fsm/packets_test.go: TestBuildLoginPacket_FieldLayout,
// TestBuildLoginPacket_UsernameNullPad, and TestBuildLoginPacket_VersionField.
// buildLoginPacket lives in package fsm and cannot be called from here.
// No duplication needed.
