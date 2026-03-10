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
func TestActorExists_0x0078_NameAndHP_Decoded(t *testing.T) {
	b := makeIdleUnit_20181121(0x0078, 12345, "Poring")
	e := ActorExists_0x0078(b, 20200401) // packetver >= 20181121 branch

	// GCC-verified: packet_idle_unit at PACKETVER >= 20181121 has name char[24] at offset 84.
	// Regression test for Bug 14-A: SemanticDB field_mapping was string("") — now fixed to
	// string(packet.name) so the codegen emits nullTermString(data[84:108]).
	if e.Name != "Poring" {
		t.Fatalf("ActorExists_0x0078 Name: got %q want %q — decode regression (Bug 14-A)", e.Name, "Poring")
	}

	if e.GID != 12345 {
		t.Errorf("ActorExists_0x0078 GID: got %d want 12345", e.GID)
	}

	// GCC-verified: HP int32 at offset 77, maxHP int32 at offset 73.
	if e.HP != 7000 {
		t.Fatalf("ActorExists_0x0078 HP: got %d want 7000 — decode regression (Bug 14-A)", e.HP)
	}
	if e.MaxHP != 9000 {
		t.Fatalf("ActorExists_0x0078 MaxHP: got %d want 9000 — decode regression (Bug 14-A)", e.MaxHP)
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
func TestActorMoved_0x09DB_Name_Decoded(t *testing.T) {
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

	// Regression test for Bug 14-B: SemanticDB had complex expression
	// strings.TrimRight(string(packet.name), "\x00") which codegen skipped.
	// Fixed to string(packet.name) so codegen emits nullTermString(data[90:114]).
	if e.Name != "Raydric" {
		t.Fatalf("ActorMoved_0x09DB Name: got %q want %q — decode regression (Bug 14-B)", e.Name, "Raydric")
	}

	// ID should be decoded (GID at offset 9)
	if e.GID != 55555 {
		t.Errorf("ActorMoved_0x09DB GID: got %d want 55555", e.GID)
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
	if e20200401.ItemId != 501 {
		t.Errorf("ItemId at 20200401: got %d want 501", e20200401.ItemId)
	}
}

// ─── Test 5: ZcPositionIdNameInfo_0x0166 PositionID field ────────────────────
//
// PACKET_ZC_POSITION_ID_NAME_INFO (variable length):
//
//	offset 0: PacketType int16
//	offset 2: PacketLength int16
//	offset 4: positionID (guild position data starts here, 4 bytes per entry)
//	offset 8: posName (char[24])
//
// Generated code decodes positionID as data[4:] (raw byte slice pointing to bytes 4..end).
func TestZcPositionIdNameInfo_0x0166_PosInfo_Decoded(t *testing.T) {
	// Build a packet large enough for the fixed-size decode (data[8:32] requires 32 bytes min).
	// GCC-verified struct: offset 4 = positionID (4 bytes), offset 8 = posName (char[24]).
	payload := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	totalLen := 32 // must be >= 32 for data[8:32] slice
	b := make([]byte, totalLen)
	binary.LittleEndian.PutUint16(b[0:], 0x0166)
	binary.LittleEndian.PutUint16(b[2:], uint16(totalLen))
	copy(b[4:], payload) // first 8 bytes of payload at offset 4

	e := ZcPositionIdNameInfo_0x0166(b, 20200401)

	// PositionID is decoded as data[4:] — raw byte slice, starts with our payload bytes
	if len(e.PositionID) < len(payload) {
		t.Fatalf("PositionID len: got %d want >= %d", len(e.PositionID), len(payload))
	}
	for i, v := range payload {
		if e.PositionID[i] != v {
			t.Fatalf("PositionID[%d]: got 0x%02X want 0x%02X", i, e.PositionID[i], v)
		}
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
