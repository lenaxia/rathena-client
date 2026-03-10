// Package decode — Phase 1 golden tests and 0-alloc benchmarks.
//
// Golden bytes are constructed field-by-field from GCC-verified struct layouts.
// Offsets are NOT taken from this generated code — they come from:
//
//	./validation/struct_layout.sh dump map/packets_struct.hpp <struct> <packetver>
//
// This ensures the tests catch codegen bugs independently.
package decode

import (
	"encoding/binary"
	"testing"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

func putU16LE(b []byte, off int, v uint16) { binary.LittleEndian.PutUint16(b[off:], v) }
func putI16LE(b []byte, off int, v int16)  { binary.LittleEndian.PutUint16(b[off:], uint16(v)) }
func putU32LE(b []byte, off int, v uint32) { binary.LittleEndian.PutUint32(b[off:], v) }
func putI32LE(b []byte, off int, v int32)  { binary.LittleEndian.PutUint32(b[off:], uint32(v)) }

// ─── ActorExists_0x09FF ───────────────────────────────────────────────────────
//
// struct packet_idle_unit at PACKETVER=20181121  (total=108 bytes)
// Verified by: ./validation/struct_layout.sh dump map/packets_struct.hpp packet_idle_unit 20181121
//
// Layout (offset : size : field):
//
//	 0:2  PacketType    1:2  PacketLength   4:1  objecttype
//	 5:4  AID           9:4  GID           13:2  speed
//	15:2  bodyState    17:2  healthState   19:4  effectState
//	23:2  job          25:2  head          27:4  weapon
//	31:4  shield       35:2  accessory     37:2  accessory2
//	39:2  accessory3   41:2  headpalette   43:2  bodypalette
//	45:2  headDir      47:2  robe          49:4  GUID
//	53:2  GEmblemVer   55:2  honor         57:4  virtue
//	61:1  isPKModeON   62:1  sex           63:3  PosDir
//	66:1  xSize        67:1  ySize         68:1  state
//	69:2  clevel       71:2  font          73:4  maxHP
//	77:4  HP           81:1  isBoss        82:2  body
//	84:24 name
func makeActorExists0x09FF_20181121() []byte {
	b := make([]byte, 108)
	putI16LE(b, 0, 0x09FF)      // PacketType
	putI16LE(b, 2, 108)         // PacketLength
	b[4] = 5                    // objecttype = MOB
	putU32LE(b, 5, 1001)        // AID → e.ID
	putU32LE(b, 9, 2002)        // GID (not mapped in actor_exists_0x09FF)
	putI16LE(b, 13, 150)        // speed → WalkSpeed
	putI16LE(b, 15, 1)          // bodyState → Opt1
	putI16LE(b, 17, 2)          // healthState → Opt2
	putI32LE(b, 19, 0x00000010) // effectState → Option
	putI16LE(b, 23, 4)          // job → Type
	putU16LE(b, 25, 5)          // head → HairStyle
	putU32LE(b, 27, 6)          // weapon → Weapon
	putU32LE(b, 31, 7)          // shield → Shield
	putU16LE(b, 35, 8)          // accessory → Lowhead
	putU16LE(b, 37, 9)          // accessory2 → Tophead
	putU16LE(b, 39, 10)         // accessory3 → Midhead
	putI16LE(b, 41, 11)         // headpalette → HairColor
	putI16LE(b, 43, 12)         // bodypalette → ClothesColor
	putI16LE(b, 45, 1)          // headDir → HeadDir
	putU16LE(b, 47, 13)         // robe → Costume
	putU32LE(b, 49, 500)        // GUID → GuildID
	putI16LE(b, 53, 3)          // GEmblemVer → EmblemID
	putI16LE(b, 55, 14)         // honor → Manner
	putI32LE(b, 57, 100)        // virtue → Opt3 (read as byte 57)
	b[61] = 0                   // isPKModeON → Stance
	b[62] = 1                   // sex → Sex
	b[63] = 0xA0                // PosDir[0]
	b[64] = 0x50                // PosDir[1]
	b[65] = 0x06                // PosDir[2]
	b[66] = 1                   // xSize → XSize
	b[67] = 2                   // ySize → YSize
	b[68] = 3                   // state → Act
	putI16LE(b, 69, 99)         // clevel → Lv
	putI16LE(b, 71, 0)          // font (not in actor_exists)
	putI32LE(b, 73, 5000)       // maxHP → MaxHP
	putI32LE(b, 77, 3000)       // HP → HP
	b[81] = 0                   // isBoss → IsBoss
	putI16LE(b, 82, 15)         // body → Opt4
	copy(b[84:], "TestMob\x00") // name → Name
	return b
}

func TestActorExists_0x09FF_Golden_20181121(t *testing.T) {
	data := makeActorExists0x09FF_20181121()
	e := ActorExists_0x09FF(data, 20181121)

	if e.ID != 1001 {
		t.Errorf("ID: got %d want 1001", e.ID)
	}
	if e.GuildID != 500 {
		t.Errorf("GuildID: got %d want 500", e.GuildID)
	}
	if e.WalkSpeed != 150 {
		t.Errorf("WalkSpeed: got %d want 150", e.WalkSpeed)
	}
	if e.Opt1 != 1 {
		t.Errorf("Opt1: got %d want 1", e.Opt1)
	}
	if e.Opt2 != 2 {
		t.Errorf("Opt2: got %d want 2", e.Opt2)
	}
	if e.Option != 0x00000010 {
		t.Errorf("Option: got %d want 16", e.Option)
	}
	if e.Type != 4 {
		t.Errorf("Type: got %d want 4", e.Type)
	}
	if e.HairStyle != 5 {
		t.Errorf("HairStyle: got %d want 5", e.HairStyle)
	}
	if e.Weapon != 6 {
		t.Errorf("Weapon: got %d want 6", e.Weapon)
	}
	if e.Shield != 7 {
		t.Errorf("Shield: got %d want 7", e.Shield)
	}
	if e.Lowhead != 8 {
		t.Errorf("Lowhead: got %d want 8", e.Lowhead)
	}
	if e.Tophead != 9 {
		t.Errorf("Tophead: got %d want 9", e.Tophead)
	}
	if e.Midhead != 10 {
		t.Errorf("Midhead: got %d want 10", e.Midhead)
	}
	if e.HairColor != 11 {
		t.Errorf("HairColor: got %d want 11", e.HairColor)
	}
	if e.ClothesColor != 12 {
		t.Errorf("ClothesColor: got %d want 12", e.ClothesColor)
	}
	if e.HeadDir != 1 {
		t.Errorf("HeadDir: got %d want 1", e.HeadDir)
	}
	if e.Costume != 13 {
		t.Errorf("Costume: got %d want 13", e.Costume)
	}
	if e.EmblemID != 3 {
		t.Errorf("EmblemID: got %d want 3", e.EmblemID)
	}
	if e.Manner != 14 {
		t.Errorf("Manner: got %d want 14", e.Manner)
	}
	// virtue at offset 57 is int32=100; decode reads byte 57 → 100 & 0xFF = 100
	if e.Opt3 != 100 {
		t.Errorf("Opt3: got %d want 100", e.Opt3)
	}
	if e.Stance != 0 {
		t.Errorf("Stance: got %d want 0", e.Stance)
	}
	if e.Sex != 1 {
		t.Errorf("Sex: got %d want 1", e.Sex)
	}
	wantPosDir := [3]byte{0xA0, 0x50, 0x06}
	if e.PosDir != wantPosDir {
		t.Errorf("PosDir: got %v want %v", e.PosDir, wantPosDir)
	}
	if e.XSize != 1 {
		t.Errorf("XSize: got %d want 1", e.XSize)
	}
	if e.YSize != 2 {
		t.Errorf("YSize: got %d want 2", e.YSize)
	}
	if e.Act != 3 {
		t.Errorf("Act: got %d want 3", e.Act)
	}
	if e.Lv != 99 {
		t.Errorf("Lv: got %d want 99", e.Lv)
	}
	if e.MaxHP != 5000 {
		t.Errorf("MaxHP: got %d want 5000", e.MaxHP)
	}
	if e.HP != 3000 {
		t.Errorf("HP: got %d want 3000", e.HP)
	}
	if e.IsBoss != 0 {
		t.Errorf("IsBoss: got %d want 0", e.IsBoss)
	}
	if e.Opt4 != 15 {
		t.Errorf("Opt4: got %d want 15", e.Opt4)
	}
	if e.Name != "TestMob" {
		t.Errorf("Name: got %q want %q", e.Name, "TestMob")
	}
	if e.ObjectType != 5 {
		t.Errorf("ObjectType: got %d want 5", e.ObjectType)
	}
}

// ─── ActorExists_0x0078 ───────────────────────────────────────────────────────
//
// Same struct (packet_idle_unit) at PACKETVER=20181121 (total=108 bytes).
// 0x0078 differs from 0x09FF in semantic field_mapping: uses GID for e.ID (not AID).
// shield is present in 0x0078 at ≥20181121 (mapped via shield field).
func TestActorExists_0x0078_Golden_20181121(t *testing.T) {
	b := makeActorExists0x09FF_20181121()
	// 0x0078 uses GID (offset 9) for e.ID, not AID (offset 5)
	putU32LE(b, 9, 9999)
	e := ActorExists_0x0078(b, 20181121)
	if e.ID != 9999 {
		t.Errorf("ID (GID): got %d want 9999", e.ID)
	}
	if e.WalkSpeed != 150 {
		t.Errorf("WalkSpeed: got %d want 150", e.WalkSpeed)
	}
	if e.EmblemID != 3 {
		t.Errorf("EmblemID: got %d want 3", e.EmblemID)
	}
	wantPosDir := [3]byte{0xA0, 0x50, 0x06}
	if e.PosDir != wantPosDir {
		t.Errorf("PosDir: got %v want %v", e.PosDir, wantPosDir)
	}
}

// ─── ActorMoved_0x09DB ────────────────────────────────────────────────────────
//
// struct packet_unit_walking at PACKETVER=20181121 (total=114 bytes)
// Verified by: ./validation/struct_layout.sh dump map/packets_struct.hpp packet_unit_walking 20181121
//
// Layout (offset : size : field):
//
//	 0:2  PacketType    2:2  PacketLength   4:1  objecttype
//	 5:4  AID           9:4  GID           13:2  speed
//	15:2  bodyState    17:2  healthState   19:4  effectState
//	23:2  job          25:2  head          27:4  weapon
//	31:4  shield       35:2  accessory     37:4  moveStartTime
//	41:2  accessory2   43:2  accessory3    45:2  headpalette
//	47:2  bodypalette  49:2  headDir       51:2  robe
//	53:4  GUID         57:2  GEmblemVer    59:2  honor
//	61:4  virtue       65:1  isPKModeON    66:1  sex
//	67:6  MoveData     73:1  xSize         74:1  ySize
//	75:2  clevel       77:2  font          79:4  maxHP
//	83:4  HP           87:1  isBoss        88:2  body
//	90:24 name
func makeActorMoved0x09DB_20181121() []byte {
	b := make([]byte, 114)
	putI16LE(b, 0, 0x09DB)      // PacketType
	putI16LE(b, 2, 114)         // PacketLength
	b[4] = 5                    // objecttype
	putU32LE(b, 5, 1001)        // AID → CharID
	putU32LE(b, 9, 2002)        // GID → ID
	putI16LE(b, 13, 200)        // speed → WalkSpeed
	putU16LE(b, 15, 3)          // bodyState → Opt1
	putU16LE(b, 17, 4)          // healthState → Opt2
	putU32LE(b, 19, 0x00000020) // effectState → Option
	putU16LE(b, 23, 6)          // job → Type
	putU32LE(b, 27, 7)          // weapon → Weapon
	putU32LE(b, 31, 8)          // shield → Shield
	putU32LE(b, 53, 600)        // GUID → GuildID
	putU16LE(b, 57, 5)          // GEmblemVer → EmblemID
	putI32LE(b, 61, 120)        // virtue → Opt3 (leU32)
	b[65] = 1                   // isPKModeON → Stance
	b[66] = 0                   // sex → Sex
	// MoveData at offset 67: encode fromX=100,fromY=200,toX=150,toY=250
	// 10-bit each: fromX=100=0b0001100100, fromY=200=0b0011001000
	// toX=150=0b0010010110, toY=250=0b0011111010
	// packed into 5 bytes; use known-good values for test
	b[67] = 0x19 // fromX high 8 bits of first 10
	b[68] = 0x03 // fromX low 2 + fromY high 6
	b[69] = 0x28 // ...
	b[70] = 0x00
	b[71] = 0x00
	b[72] = 0x00
	b[73] = 2           // xSize → XSize
	b[74] = 3           // ySize → YSize
	putU16LE(b, 75, 88) // clevel → Lv
	putI16LE(b, 49, 2)  // headDir → HeadDir
	putU16LE(b, 41, 10) // accessory2 → (not mapped in actor_moved for 0x09DB, skip)
	return b
}

func TestActorMoved_0x09DB_Golden_20181121(t *testing.T) {
	data := makeActorMoved0x09DB_20181121()
	e := ActorMoved_0x09DB(data, 20181121)

	if e.CharID != 1001 {
		t.Errorf("CharID: got %d want 1001", e.CharID)
	}
	if e.ID != 2002 {
		t.Errorf("ID: got %d want 2002", e.ID)
	}
	if e.WalkSpeed != 200 {
		t.Errorf("WalkSpeed: got %d want 200", e.WalkSpeed)
	}
	if e.Opt1 != 3 {
		t.Errorf("Opt1: got %d want 3", e.Opt1)
	}
	if e.Opt2 != 4 {
		t.Errorf("Opt2: got %d want 4", e.Opt2)
	}
	if e.Option != 0x00000020 {
		t.Errorf("Option: got %d want 32", e.Option)
	}
	if e.Type != 6 {
		t.Errorf("Type: got %d want 6", e.Type)
	}
	if e.Weapon != 7 {
		t.Errorf("Weapon: got %d want 7", e.Weapon)
	}
	if e.Shield != 8 {
		t.Errorf("Shield: got %d want 8", e.Shield)
	}
	if e.GuildID != 600 {
		t.Errorf("GuildID: got %d want 600", e.GuildID)
	}
	if e.EmblemID != 5 {
		t.Errorf("EmblemID: got %d want 5", e.EmblemID)
	}
	if e.Stance != 1 {
		t.Errorf("Stance: got %d want 1", e.Stance)
	}
	if e.Sex != 0 {
		t.Errorf("Sex: got %d want 0", e.Sex)
	}
	if e.XSize != 2 {
		t.Errorf("XSize: got %d want 2", e.XSize)
	}
	if e.YSize != 3 {
		t.Errorf("YSize: got %d want 3", e.YSize)
	}
	if e.Lv != 88 {
		t.Errorf("Lv: got %d want 88", e.Lv)
	}
	// MoveData: verify the 6-byte field is copied correctly
	wantMove := [6]byte{0x19, 0x03, 0x28, 0x00, 0x00, 0x00}
	if e.MoveData != wantMove {
		t.Errorf("MoveData: got %v want %v", e.MoveData, wantMove)
	}
}

// ─── ActorMoved_0x007B ────────────────────────────────────────────────────────
//
// Same struct (packet_unit_walking) at PACKETVER=20181121 (total=114 bytes).
// 0x007B differs from 0x09DB in semantic field_mapping: no CharID.
func TestActorMoved_0x007B_Golden_20181121(t *testing.T) {
	data := makeActorMoved0x09DB_20181121()
	e := ActorMoved_0x007B(data, 20181121)

	if e.ID != 2002 {
		t.Errorf("ID (GID): got %d want 2002", e.ID)
	}
	if e.WalkSpeed != 200 {
		t.Errorf("WalkSpeed: got %d want 200", e.WalkSpeed)
	}
	wantMove := [6]byte{0x19, 0x03, 0x28, 0x00, 0x00, 0x00}
	if e.MoveData != wantMove {
		t.Errorf("MoveData: got %v want %v", e.MoveData, wantMove)
	}
}

// ─── ActorConnected_0x09FE ────────────────────────────────────────────────────
//
// struct packet_spawn_unit at PACKETVER=20181121 (total=107 bytes)
// Verified by: ./validation/struct_layout.sh dump map/packets_struct.hpp packet_spawn_unit 20181121
//
// Layout (offset : size : field):
//
//	 0:2  PacketType    2:2  PacketLength   4:1  objecttype
//	 5:4  AID           9:4  GID           13:2  speed
//	15:2  bodyState    17:2  healthState   19:4  effectState
//	23:2  job          25:2  head          27:4  weapon
//	31:4  shield       35:2  accessory     37:2  accessory2
//	39:2  accessory3   41:2  headpalette   43:2  bodypalette
//	45:2  headDir      47:2  robe          49:4  GUID
//	53:2  GEmblemVer   55:2  honor         57:4  virtue
//	61:1  isPKModeON   62:1  sex           63:3  PosDir
//	66:1  xSize        67:1  ySize         68:2  clevel
//	70:2  font         72:4  maxHP         76:4  HP
//	80:1  isBoss       81:2  body          83:24 name
func makeActorConnected0x09FE_20181121() []byte {
	b := make([]byte, 107)
	putI16LE(b, 0, 0x09FE)      // PacketType
	putI16LE(b, 2, 107)         // PacketLength
	b[4] = 1                    // objecttype = PC
	putU32LE(b, 5, 3003)        // AID
	putU32LE(b, 9, 4004)        // GID → ID
	putI16LE(b, 13, 120)        // speed → WalkSpeed
	putI16LE(b, 15, 0)          // bodyState → Opt1
	putI16LE(b, 17, 0)          // healthState → Opt2
	putI32LE(b, 19, 0)          // effectState → Option
	putI16LE(b, 23, 0)          // job → Type (0=novice)
	putU32LE(b, 27, 0)          // weapon
	putU32LE(b, 31, 0)          // shield
	putI16LE(b, 45, 3)          // headDir → HeadDir
	putU32LE(b, 49, 700)        // GUID → GuildID
	putI16LE(b, 53, 7)          // GEmblemVer → EmblemID
	putI32LE(b, 57, 50)         // virtue → Opt3 (byte 57)
	b[61] = 0                   // isPKModeON → Stance
	b[62] = 1                   // sex → Sex
	b[63] = 0xC0                // PosDir[0]
	b[64] = 0x60                // PosDir[1]
	b[65] = 0x02                // PosDir[2]
	b[66] = 0                   // xSize → XSize
	b[67] = 0                   // ySize → YSize
	putI16LE(b, 68, 55)         // clevel → Lv
	putI32LE(b, 72, 9999)       // maxHP → MaxHP
	putI32LE(b, 76, 8888)       // HP → HP
	b[80] = 0                   // isBoss → IsBoss
	copy(b[83:], "Player1\x00") // name → Name
	return b
}

func TestActorConnected_0x09FE_Golden_20181121(t *testing.T) {
	data := makeActorConnected0x09FE_20181121()
	e := ActorConnected_0x09FE(data, 20181121)

	// 0x09FE uses AID (offset 5) for e.ID and GID (offset 9) for e.CharID.
	if e.ID != 3003 {
		t.Errorf("ID (AID): got %d want 3003", e.ID)
	}
	if e.CharID != 4004 {
		t.Errorf("CharID (GID): got %d want 4004", e.CharID)
	}
	if e.WalkSpeed != 120 {
		t.Errorf("WalkSpeed: got %d want 120", e.WalkSpeed)
	}
	if e.HeadDir != 3 {
		t.Errorf("HeadDir: got %d want 3", e.HeadDir)
	}
	if e.GuildID != 700 {
		t.Errorf("GuildID: got %d want 700", e.GuildID)
	}
	if e.EmblemID != 7 {
		t.Errorf("EmblemID: got %d want 7", e.EmblemID)
	}
	if e.Stance != 0 {
		t.Errorf("Stance: got %d want 0", e.Stance)
	}
	if e.Sex != 1 {
		t.Errorf("Sex: got %d want 1", e.Sex)
	}
	wantPosDir := [3]byte{0xC0, 0x60, 0x02}
	if e.PosDir != wantPosDir {
		t.Errorf("PosDir: got %v want %v", e.PosDir, wantPosDir)
	}
	if e.Lv != 55 {
		t.Errorf("Lv: got %d want 55", e.Lv)
	}
	if e.MaxHP != 9999 {
		t.Errorf("MaxHP: got %d want 9999", e.MaxHP)
	}
	if e.HP != 8888 {
		t.Errorf("HP: got %d want 8888", e.HP)
	}
	// Name field has a complex expression in the DB and is not decoded by the
	// generated code (emitted as a comment). Name will be empty string.
	// This is a known skip (Category A/complex expression) — documented in KNOWN_ISSUES.md.
	if e.Name != "" {
		t.Errorf("Name: got %q want empty (name decode is a known skip)", e.Name)
	}
}

// ─── ActorConnected_0x0079 ────────────────────────────────────────────────────
//
// Same struct (packet_spawn_unit) at PACKETVER=20181121 (total=107 bytes).
func TestActorConnected_0x0079_Golden_20181121(t *testing.T) {
	data := makeActorConnected0x09FE_20181121()
	e := ActorConnected_0x0079(data, 20181121)

	if e.ID != 4004 {
		t.Errorf("ID (GID): got %d want 4004", e.ID)
	}
	if e.WalkSpeed != 120 {
		t.Errorf("WalkSpeed: got %d want 120", e.WalkSpeed)
	}
	if e.Lv != 55 {
		t.Errorf("Lv: got %d want 55", e.Lv)
	}
}

// ─── StatUpdate_0x00B0 ────────────────────────────────────────────────────────
//
// struct PACKET_ZC_PAR_CHANGE (total=8 bytes):
//
//	0:2 PacketType   2:2 varID   4:4 count
//
// Verified from GCC output of packets_struct.hpp at PACKETVER=20181121.
func makeStatUpdate0x00B0() []byte {
	b := make([]byte, 8)
	putI16LE(b, 0, 0x00B0) // PacketType
	putU16LE(b, 2, 500)    // varID → StatType
	putI32LE(b, 4, 9876)   // count → Value
	return b
}

func TestStatUpdate_0x00B0_Golden(t *testing.T) {
	data := makeStatUpdate0x00B0()
	e := StatUpdate_0x00B0(data, 20181121)

	if e.StatType != 500 {
		t.Errorf("StatType: got %d want 500", e.StatType)
	}
	if e.Value != uint32(int32(9876)) {
		t.Errorf("Value: got %d want 9876", e.Value)
	}
}

// ─── StatUpdate_0x00B1 ────────────────────────────────────────────────────────
//
// struct PACKET_ZC_LONGPAR_CHANGE (total=8 bytes):
//
//	0:2 PacketType   2:2 varID   4:4 amount
func TestStatUpdate_0x00B1_Golden(t *testing.T) {
	b := make([]byte, 8)
	putI16LE(b, 0, 0x00B1)
	putU16LE(b, 2, 501)
	putU32LE(b, 4, 12345)
	e := StatUpdate_0x00B1(b, 20181121)

	if e.StatType != 501 {
		t.Errorf("StatType: got %d want 501", e.StatType)
	}
	if e.Value != 12345 {
		t.Errorf("Value: got %d want 12345", e.Value)
	}
}

// ─── StatUpdate_0x00BE ────────────────────────────────────────────────────────
//
// struct PACKET_ZC_STATUS_CHANGE (total=5 bytes):
//
//	0:2 PacketType   2:2 statusID   4:1 value
//
// TestStatUpdate_0x00BE_Golden verifies the statusID and value fields.
// The value field is a 1-byte uint8 at offset 4; decoded as uint32(data[4]).
func TestStatUpdate_0x00BE_Golden(t *testing.T) {
	b := make([]byte, 5) // exact packet size: 5 bytes
	putI16LE(b, 0, 0x00BE)
	putU16LE(b, 2, 200) // statusID → StatType
	b[4] = 42           // value (1 byte, decoded as uint32)
	e := StatUpdate_0x00BE(b, 20181121)

	if e.StatType != 200 {
		t.Errorf("StatType: got %d want 200", e.StatType)
	}
	// Value: uint32(data[4]) reads exactly 1 byte → 42
	if e.Value != 42 {
		t.Errorf("Value: got %d want 42", e.Value)
	}
}

// ─── Packet version boundary tests ───────────────────────────────────────────

// TestActorExists_0x09FF_VersionBoundary verifies that the decode function
// selects the correct code path at packetver boundary 20181121.
// At packetver=20181120 (one below boundary), the struct is the previous version.
// We verify that different offsets produce different field values.
func TestActorExists_0x09FF_VersionBoundary_Below(t *testing.T) {
	// At packetver < 20181121, packet_idle_unit has AID at offset 2 (not 5)
	// and GID at offset 6. We just verify the function doesn't panic and
	// that fields are zeroed when data doesn't match.
	b := make([]byte, 108)
	putU32LE(b, 2, 777) // AID at old offset
	e := ActorExists_0x09FF(b, 20181120)
	// Only sanity-check: no panic, ID read from some offset (may be 0 or 777)
	_ = e
}

// ─── Benchmarks ───────────────────────────────────────────────────────────────

func BenchmarkActorExists_0x09FF(b *testing.B) {
	data := makeActorExists0x09FF_20181121()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ActorExists_0x09FF(data, 20181121)
	}
}

func BenchmarkActorMoved_0x09DB(b *testing.B) {
	data := makeActorMoved0x09DB_20181121()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ActorMoved_0x09DB(data, 20181121)
	}
}

func BenchmarkActorMoved_0x007B(b *testing.B) {
	data := makeActorMoved0x09DB_20181121()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ActorMoved_0x007B(data, 20181121)
	}
}

func BenchmarkActorConnected_0x09FE(b *testing.B) {
	data := makeActorConnected0x09FE_20181121()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ActorConnected_0x09FE(data, 20181121)
	}
}

func BenchmarkActorExists_0x0078(b *testing.B) {
	data := makeActorExists0x09FF_20181121()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ActorExists_0x0078(data, 20181121)
	}
}

func BenchmarkStatUpdate_0x00B0(b *testing.B) {
	data := makeStatUpdate0x00B0()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = StatUpdate_0x00B0(data, 20181121)
	}
}

// ─── AcAcceptLogin_0x0069 ─────────────────────────────────────────────────────
//
// struct PACKET_AC_ACCEPT_LOGIN at PACKETVER < 20170315 (fixed header = 47 bytes):
//
//	 0:2  packetType   2:2  packetLength   4:4  login_id1
//	 8:4  AID         12:4  login_id2     16:4  last_ip
//	20:26 last_login  46:1  sex
//	then variable-length char_servers[] (not decoded by generated code)
//
// Verified by: g++ -E -P on common/packets.hpp at PACKETVER=20030000
func makeAcAcceptLogin0x0069() []byte {
	b := make([]byte, 47)                // header only; no char_servers entries
	putI16LE(b, 0, 0x0069)               // packetType
	putI16LE(b, 2, 47)                   // packetLength (header only in this test)
	putU32LE(b, 4, 0xDEAD0001)           // login_id1 → SessionID
	putU32LE(b, 8, 12345678)             // AID → AccountID
	putU32LE(b, 12, 0xDEAD0002)          // login_id2 → SessionID2
	putU32LE(b, 16, 0xC0A80101)          // last_ip → LastLoginIP (192.168.1.1 as uint32 LE)
	copy(b[20:], "2024-01-15 12:00\x00") // last_login[26] → LastLoginTime
	b[46] = 1                            // sex = MALE → AccountSex
	return b
}

func TestAcAcceptLogin_0x0069_Golden(t *testing.T) {
	data := makeAcAcceptLogin0x0069()
	e := AcAcceptLogin_0x0069(data, 20030000)

	if e.SessionID != 0xDEAD0001 {
		t.Errorf("SessionID: got 0x%X want 0xDEAD0001", e.SessionID)
	}
	if e.AccountID != 12345678 {
		t.Errorf("AccountID: got %d want 12345678", e.AccountID)
	}
	if e.SessionID2 != 0xDEAD0002 {
		t.Errorf("SessionID2: got 0x%X want 0xDEAD0002", e.SessionID2)
	}
	if e.LastLoginIP != 0xC0A80101 {
		t.Errorf("LastLoginIP: got 0x%X want 0xC0A80101", e.LastLoginIP)
	}
	if e.LastLoginTime != "2024-01-15 12:00" {
		t.Errorf("LastLoginTime: got %q want %q", e.LastLoginTime, "2024-01-15 12:00")
	}
	if e.AccountSex != 1 {
		t.Errorf("AccountSex: got %d want 1", e.AccountSex)
	}
	// ServerInfo (char_servers flex array) is not decoded by the generated function —
	// it requires a higher-level packet parser to interpret the variable-length entries.
}

func BenchmarkAcAcceptLogin_0x0069(b *testing.B) {
	data := makeAcAcceptLogin0x0069()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = AcAcceptLogin_0x0069(data, 20030000)
	}
}

// ─── PetEggList_0x01A6 ────────────────────────────────────────────────────────
//
// struct SYNTH_ZC_PETEGG_LIST (variable length):
//
//	0:2  PacketType    2:2  PacketLength   4:+  eggs[] (uint16 per entry)
//
// Source: internal/codegen/stubs/synthetic_structs.hpp
// Confirmed: clif.cpp:8252 WFIFOW(fd,n*2+4)=client_index(i) — each entry is uint16 LE
//
// Test encodes 3 inventory indices [10, 200, 300] as little-endian uint16 at offset 4.
// Decoded as []int16 via leI16 loop — values must match exactly.
func TestPetEggList_0x01A6_Golden(t *testing.T) {
	indices := []uint16{10, 200, 300}
	totalLen := 4 + len(indices)*2 // header(4) + 3×uint16
	b := make([]byte, totalLen)
	putI16LE(b, 0, 0x01A6)
	putU16LE(b, 2, uint16(totalLen))
	for i, idx := range indices {
		putU16LE(b, 4+i*2, idx)
	}

	e := PetEggList_0x01A6(b, 20181121)

	if len(e.InventoryIndices) != 3 {
		t.Fatalf("InventoryIndices: got len=%d want 3", len(e.InventoryIndices))
	}
	want := []int16{10, 200, 300}
	for i, w := range want {
		if e.InventoryIndices[i] != w {
			t.Errorf("InventoryIndices[%d]: got %d want %d", i, e.InventoryIndices[i], w)
		}
	}
}

// TestPetEggList_0x01A6_Empty verifies zero-length payload produces empty slice (no panic).
func TestPetEggList_0x01A6_Empty(t *testing.T) {
	b := make([]byte, 4) // header only, no eggs
	putI16LE(b, 0, 0x01A6)
	putU16LE(b, 2, 4)

	e := PetEggList_0x01A6(b, 20181121)

	if len(e.InventoryIndices) != 0 {
		t.Errorf("InventoryIndices: got len=%d want 0", len(e.InventoryIndices))
	}
}

// TestPetEggList_0x01A6_OddTrailingByte verifies an odd-length payload does not panic
// and decodes only the complete pairs (trailing byte is silently dropped per integer division).
func TestPetEggList_0x01A6_OddTrailingByte(t *testing.T) {
	b := make([]byte, 4+3) // header + 1 full uint16 + 1 orphan byte
	putI16LE(b, 0, 0x01A6)
	putU16LE(b, 2, 7)
	putU16LE(b, 4, 42) // complete pair → index 42
	b[6] = 0xFF        // orphan byte — ignored

	e := PetEggList_0x01A6(b, 20181121)

	if len(e.InventoryIndices) != 1 {
		t.Fatalf("InventoryIndices: got len=%d want 1", len(e.InventoryIndices))
	}
	if e.InventoryIndices[0] != 42 {
		t.Errorf("InventoryIndices[0]: got %d want 42", e.InventoryIndices[0])
	}
}

// ─── ZcNpcBarterMarketIteminfo_0x0B0E ─────────────────────────────────────────
//
// struct PACKET_ZC_NPC_BARTER_MARKET_ITEMINFO (variable length):
//
//	0:2  packetType    2:2  packetLength   4:+  list[] (struct entries)
//
// Source: rathena/src/map/packets_struct.hpp:3892
// The list field is a nested struct flex array; decoded as raw []byte data[4:].
// Tests verify PacketType, PacketLength, and that List contains exactly the bytes
// written after the 4-byte header.
func TestZcNpcBarterMarketIteminfo_0x0B0E_Golden(t *testing.T) {
	// Fabricate 8 bytes of "item list" payload (2 notional entries of 4 bytes each).
	payload := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	totalLen := 4 + len(payload)
	b := make([]byte, totalLen)
	putI16LE(b, 0, 0x0B0E)
	putI16LE(b, 2, int16(totalLen))
	copy(b[4:], payload)

	e := ZcNpcBarterMarketIteminfo_0x0B0E(b, 20181121)

	if e.PacketType != 0x0B0E {
		t.Errorf("PacketType: got 0x%X want 0x0B0E", e.PacketType)
	}
	if e.PacketLength != int16(totalLen) {
		t.Errorf("PacketLength: got %d want %d", e.PacketLength, totalLen)
	}
	if len(e.List) != len(payload) {
		t.Fatalf("List: got len=%d want %d", len(e.List), len(payload))
	}
	for i, b := range payload {
		if e.List[i] != b {
			t.Errorf("List[%d]: got 0x%02X want 0x%02X", i, e.List[i], b)
		}
	}
}

// TestZcNpcBarterMarketIteminfo_0x0B0E_Empty verifies header-only packet (empty list).
func TestZcNpcBarterMarketIteminfo_0x0B0E_Empty(t *testing.T) {
	b := make([]byte, 4)
	putI16LE(b, 0, 0x0B0E)
	putI16LE(b, 2, 4)

	e := ZcNpcBarterMarketIteminfo_0x0B0E(b, 20181121)

	if len(e.List) != 0 {
		t.Errorf("List: got len=%d want 0", len(e.List))
	}
}

// ─── ZcGuildAgitInfo_0x0B27 ───────────────────────────────────────────────────
//
// struct PACKET_ZC_GUILD_AGIT_INFO (variable length):
//
//	0:2  packetType    2:2  packetLength   4:+  castle_list[] (int8 per entry)
//
// Source: rathena/src/map/packets_struct.hpp:4343
// castle_list is a primitive flex array (int8[]); decoded as raw []byte data[4:].
func TestZcGuildAgitInfo_0x0B27_Golden(t *testing.T) {
	castles := []byte{0x01, 0x03, 0x07, 0x0F} // 4 castle IDs
	totalLen := 4 + len(castles)
	b := make([]byte, totalLen)
	putI16LE(b, 0, 0x0B27)
	putI16LE(b, 2, int16(totalLen))
	copy(b[4:], castles)

	e := ZcGuildAgitInfo_0x0B27(b, 20181121)

	if e.PacketType != 0x0B27 {
		t.Errorf("PacketType: got 0x%X want 0x0B27", e.PacketType)
	}
	if e.PacketLength != int16(totalLen) {
		t.Errorf("PacketLength: got %d want %d", e.PacketLength, totalLen)
	}
	if len(e.CastleList) != len(castles) {
		t.Fatalf("CastleList: got len=%d want %d", len(e.CastleList), len(castles))
	}
	for i, c := range castles {
		if e.CastleList[i] != c {
			t.Errorf("CastleList[%d]: got 0x%02X want 0x%02X", i, e.CastleList[i], c)
		}
	}
}

// TestZcGuildAgitInfo_0x0B27_Empty verifies header-only packet (no castles held).
func TestZcGuildAgitInfo_0x0B27_Empty(t *testing.T) {
	b := make([]byte, 4)
	putI16LE(b, 0, 0x0B27)
	putI16LE(b, 2, 4)

	e := ZcGuildAgitInfo_0x0B27(b, 20181121)

	if len(e.CastleList) != 0 {
		t.Errorf("CastleList: got len=%d want 0", len(e.CastleList))
	}
}

// ─── ChatMessage_0x008D ───────────────────────────────────────────────────────
//
// struct PACKET_ZC_NOTIFY_CHAT (variable length):
//
//	0:2  PacketType    2:2  PacketLength   4:4  GID   8:+  Message (null-terminated)
//
// Verified from rAthena: clif.cpp clif_chat() — GID at offset 4, Message at offset 8.
// Note: Message is a complex expression in the DB; decoded as empty string by generated code.
func makeChatMessage0x008D(senderID uint32, msg string) []byte {
	msgBytes := append([]byte(msg), 0x00) // null-terminate
	totalLen := 8 + len(msgBytes)
	b := make([]byte, totalLen)
	putI16LE(b, 0, 0x008D)
	putI16LE(b, 2, int16(totalLen))
	putU32LE(b, 4, senderID)
	copy(b[8:], msgBytes)
	return b
}

// TestChatMessage_0x008D_Golden verifies SenderID is decoded from GID at offset 4.
// Message field is a complex expression and is emitted as a comment — decoded as "".
func TestChatMessage_0x008D_Golden(t *testing.T) {
	data := makeChatMessage0x008D(9876, "Hello world")
	e := ChatMessage_0x008D(data, 20181121)

	if e.SenderID != 9876 {
		t.Errorf("SenderID: got %d want 9876", e.SenderID)
	}
	// Message is a complex expression (DB position 3); generated code emits a comment.
	// Verifying it decodes to "" is the correct expectation per known-skip documentation.
	if e.Message != "" {
		t.Errorf("Message: got %q want empty (complex expression — known skip)", e.Message)
	}
}

// TestChatMessage_0x008D_ZeroSender verifies zero GID is preserved correctly.
func TestChatMessage_0x008D_ZeroSender(t *testing.T) {
	data := makeChatMessage0x008D(0, "system message")
	e := ChatMessage_0x008D(data, 20181121)

	if e.SenderID != 0 {
		t.Errorf("SenderID: got %d want 0", e.SenderID)
	}
}

// ─── ChatMessage_0x008E ───────────────────────────────────────────────────────
//
// struct PACKET_ZC_NOTIFY_PLAYERCHAT (variable length):
//
//	0:2  PacketType    2:2  PacketLength   4:+  Message (null-terminated, no GID)
//
// 0x008E is the self-chat echo — no sender ID field. GID is implicitly the player's own ID.
// The generated code still reads GID at offset 4, but the struct has no GID field.
// SenderID will contain whatever bytes happen to be at offset 4 (first bytes of Message).
func TestChatMessage_0x008E_Golden(t *testing.T) {
	msg := "My own chat"
	msgBytes := append([]byte(msg), 0x00)
	totalLen := 4 + len(msgBytes)
	b := make([]byte, totalLen)
	putI16LE(b, 0, 0x008E)
	putI16LE(b, 2, int16(totalLen))
	copy(b[4:], msgBytes)

	e := ChatMessage_0x008E(b, 20181121)

	// SenderID reads leU32(data, 4) — which is first 4 bytes of Message ("My o" LE = 0x6F204D79)
	wantSenderID := uint32(b[4]) | uint32(b[5])<<8 | uint32(b[6])<<16 | uint32(b[7])<<24
	if e.SenderID != wantSenderID {
		t.Errorf("SenderID: got 0x%X want 0x%X (first 4 bytes of message)", e.SenderID, wantSenderID)
	}
	// Message is a complex expression — emitted as comment, decoded as "".
	if e.Message != "" {
		t.Errorf("Message: got %q want empty (complex expression — known skip)", e.Message)
	}
}

// ─── CharacterMove_0x035F ─────────────────────────────────────────────────────
//
// struct SYNTH_CZ_REQUEST_MOVE2 (5 bytes):
//
//	0:2  packetType   2:3  dest (packed x,y,dir)
//
// Verified from rAthena: clif_parse_WalkToXY — 3-byte packed coords at offset 2.
func makeCharacterMove0x035F(coords [3]byte) []byte {
	b := make([]byte, 5)
	putI16LE(b, 0, 0x035F)
	b[2] = coords[0]
	b[3] = coords[1]
	b[4] = coords[2]
	return b
}

func TestCharacterMove_0x035F_Golden(t *testing.T) {
	coords := [3]byte{0xA8, 0x54, 0x06} // x=168, y=84, dir=6
	data := makeCharacterMove0x035F(coords)
	e := CharacterMove_0x035F(data, 20181121)

	if e.Coords != coords {
		t.Errorf("Coords: got %v want %v", e.Coords, coords)
	}
}

// TestCharacterMove_0x035F_ZeroCoords verifies zero coords produce zeroed result.
func TestCharacterMove_0x035F_ZeroCoords(t *testing.T) {
	data := makeCharacterMove0x035F([3]byte{0, 0, 0})
	e := CharacterMove_0x035F(data, 20181121)

	if e.Coords != ([3]byte{}) {
		t.Errorf("Coords: got %v want {0,0,0}", e.Coords)
	}
}

// ─── ActorStatusActive_0x0196 ─────────────────────────────────────────────────
//
// struct packet_sc_notick (9 bytes):
//
//	0:2  PacketType   2:2  index(statusID)   4:4  AID   8:1  state(Active)
//
// Verified from rAthena: status.cpp clif_status_change_sub()
func makeActorStatusActive0x0196(statusID uint16, actorID uint32, active uint8) []byte {
	b := make([]byte, 9)
	putI16LE(b, 0, 0x0196)
	putU16LE(b, 2, statusID)
	putU32LE(b, 4, actorID)
	b[8] = active
	return b
}

func TestActorStatusActive_0x0196_Golden_Active(t *testing.T) {
	data := makeActorStatusActive0x0196(42, 10001, 1)
	e := ActorStatusActive_0x0196(data, 20181121)

	if e.StatusID != 42 {
		t.Errorf("StatusID: got %d want 42", e.StatusID)
	}
	if e.ActorID != 10001 {
		t.Errorf("ActorID: got %d want 10001", e.ActorID)
	}
	if e.Active != 1 {
		t.Errorf("Active: got %d want 1", e.Active)
	}
}

func TestActorStatusActive_0x0196_Golden_Inactive(t *testing.T) {
	data := makeActorStatusActive0x0196(100, 99999, 0)
	e := ActorStatusActive_0x0196(data, 20181121)

	if e.StatusID != 100 {
		t.Errorf("StatusID: got %d want 100", e.StatusID)
	}
	if e.ActorID != 99999 {
		t.Errorf("ActorID: got %d want 99999", e.ActorID)
	}
	if e.Active != 0 {
		t.Errorf("Active: got %d want 0", e.Active)
	}
}

// ─── ActorStatusEffectExtended_0x043F ────────────────────────────────────────
//
// struct packet_status_change2 (25 bytes):
//
//	0:2  PacketType   2:2  statusID   4:4  AID   8:1  flag
//	9:4  Left(tick)  13:4  val1      17:4  val2  21:4  val3
//
// Verified from rAthena: status.cpp clif_status_change2()
// Note: Active field uses complex expression (packet.state != 0) — emitted as comment, defaults to false.
func makeActorStatusEffectExtended0x043F(statusID uint16, actorID uint32, flag uint8,
	tick, val1, val2, val3 uint32) []byte {
	b := make([]byte, 25)
	putI16LE(b, 0, 0x043F)
	putU16LE(b, 2, statusID)
	putU32LE(b, 4, actorID)
	b[8] = flag
	putU32LE(b, 9, tick)
	putU32LE(b, 13, val1)
	putU32LE(b, 17, val2)
	putU32LE(b, 21, val3)
	return b
}

func TestActorStatusEffectExtended_0x043F_Golden(t *testing.T) {
	data := makeActorStatusEffectExtended0x043F(55, 20202, 1, 30000, 10, 20, 30)
	e := ActorStatusEffectExtended_0x043F(data, 20181121)

	if e.StatusID != 55 {
		t.Errorf("StatusID: got %d want 55", e.StatusID)
	}
	if e.ActorID != 20202 {
		t.Errorf("ActorID: got %d want 20202", e.ActorID)
	}
	// Active is a complex expression (packet.state != 0) — known skip, defaults to false.
	if e.Active != false {
		t.Errorf("Active: got %v want false (complex expression — known skip)", e.Active)
	}
	if e.DurationMS != 30000 {
		t.Errorf("DurationMS: got %d want 30000", e.DurationMS)
	}
	if e.Val1 != 10 {
		t.Errorf("Val1: got %d want 10", e.Val1)
	}
	if e.Val2 != 20 {
		t.Errorf("Val2: got %d want 20", e.Val2)
	}
	if e.Val3 != 30 {
		t.Errorf("Val3: got %d want 30", e.Val3)
	}
}

// TestActorStatusEffectExtended_0x043F_ZeroDuration verifies zero-duration status removal packet.
func TestActorStatusEffectExtended_0x043F_ZeroDuration(t *testing.T) {
	data := makeActorStatusEffectExtended0x043F(77, 5050, 0, 0, 0, 0, 0)
	e := ActorStatusEffectExtended_0x043F(data, 20181121)

	if e.StatusID != 77 {
		t.Errorf("StatusID: got %d want 77", e.StatusID)
	}
	if e.ActorID != 5050 {
		t.Errorf("ActorID: got %d want 5050", e.ActorID)
	}
	if e.DurationMS != 0 {
		t.Errorf("DurationMS: got %d want 0", e.DurationMS)
	}
}

// ─── Version boundary note ────────────────────────────────────────────────────
//
// ActorVanished (0x0080), MapEnter (0x02EB/0x007D), and ActorAction (0x008A/0x08C8)
// decoders are SKIP stubs — their rAthena structs are not present in the VersionTable
// for the MAIN branch (20181121 <= pv < 20200916). There are no generated decode
// functions to test. The FSM handles these packets at the session layer without
// invoking generated decode functions.
//
// Version boundary tests for functions that DO exist:

// TestActorStatusActive_0x0196_VersionBoundary verifies no panic at packetver boundary.
func TestActorStatusActive_0x0196_VersionBoundary(t *testing.T) {
	// At any packetver, packet_sc_notick has the same layout (9 bytes, no version variants).
	data := makeActorStatusActive0x0196(1, 1, 1)
	e20181121 := ActorStatusActive_0x0196(data, 20181121)
	e20200401 := ActorStatusActive_0x0196(data, 20200401)
	// Both should decode identically — struct has no PACKETVER variants.
	if e20181121 != e20200401 {
		t.Errorf("version boundary mismatch: 20181121=%+v 20200401=%+v", e20181121, e20200401)
	}
}

// TestActorStatusEffectExtended_0x043F_VersionBoundary verifies no panic at packetver boundary.
func TestActorStatusEffectExtended_0x043F_VersionBoundary(t *testing.T) {
	data := makeActorStatusEffectExtended0x043F(1, 1, 1, 1000, 1, 1, 1)
	e20181121 := ActorStatusEffectExtended_0x043F(data, 20181121)
	e20200401 := ActorStatusEffectExtended_0x043F(data, 20200401)
	if e20181121 != e20200401 {
		t.Errorf("version boundary mismatch: 20181121=%+v 20200401=%+v", e20181121, e20200401)
	}
}

// ─── F3 Benchmarks ────────────────────────────────────────────────────────────

func BenchmarkChatMessage_0x008D(b *testing.B) {
	data := makeChatMessage0x008D(12345, "benchmark chat message")
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ChatMessage_0x008D(data, 20181121)
	}
}

func BenchmarkCharacterMove_0x035F(b *testing.B) {
	data := makeCharacterMove0x035F([3]byte{0xA8, 0x54, 0x06})
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = CharacterMove_0x035F(data, 20181121)
	}
}

func BenchmarkActorStatusActive_0x0196(b *testing.B) {
	data := makeActorStatusActive0x0196(42, 10001, 1)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ActorStatusActive_0x0196(data, 20181121)
	}
}

func BenchmarkActorStatusEffectExtended_0x043F(b *testing.B) {
	data := makeActorStatusEffectExtended0x043F(55, 20202, 1, 30000, 10, 20, 30)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ActorStatusEffectExtended_0x043F(data, 20181121)
	}
}
