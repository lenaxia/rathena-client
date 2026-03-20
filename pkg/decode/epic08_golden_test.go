// Package decode — EPIC-08 golden tests.
//
// These tests verify byte-level correctness of the new decode functions
// generated after EPIC-08 DB additions. Each test:
//
//  1. Constructs a packet frame by laying out C struct fields manually,
//     cross-referenced against rAthena struct definitions.
//
//  2. Calls the generated decode function.
//
//  3. Asserts specific field values.
//
// Tests are written BEFORE codegen is run, so they fail with "undefined"
// errors on the current codebase and pass after codegen regenerates the
// decode functions.
//
// Struct layouts are verified against:
//
//	g++ -E -P -DPACKETVER=YYYYMMDD -I ~/personal/rathena/src \
//	    ~/personal/rathena/src/map/packets.hpp | grep -A20 'struct PACKET_ZC_ACCEPT_ENTER'
package decode

import (
	"testing"

	"github.com/lenaxia/rathena-client/pkg/events"
)

// ─── ZcAcceptEnter_0x0073 ─────────────────────────────────────────────────────
//
// struct PACKET_ZC_ACCEPT_ENTER at PACKETVER < 20080102 (11 bytes):
//
//	0:2  PacketType   2:4  startTime   6:3  posDir[3]
//	9:1  xSize       10:1  ySize
//
// rAthena: packets.hpp DEFINE_PACKET_HEADER(ZC_ACCEPT_ENTER, 0x73)
func TestZcAcceptEnter_0x0073(t *testing.T) {
	b := make([]byte, 11)
	putI16LE(b, 0, 0x0073) // PacketType
	putU32LE(b, 2, 123456) // startTime
	b[6] = 0xA0            // posDir[0]: x=160>>2=40, upper bits
	b[7] = 0x64            // posDir[1]
	b[8] = 0x06            // posDir[2]: dir=6 (East)
	b[9] = 5               // xSize
	b[10] = 5              // ySize

	e := ZcAcceptEnter_0x0073(b, 20050101)

	if e.StartTime != 123456 {
		t.Errorf("StartTime=%d want 123456", e.StartTime)
	}
	// posDir decodes to x, y, dir via packing.DecodePosDir
	if e.PosDir != [3]byte{0xA0, 0x64, 0x06} {
		t.Errorf("PosDir=%v want [0xA0 0x64 0x06]", e.PosDir)
	}
	if e.XSize != 5 {
		t.Errorf("XSize=%d want 5", e.XSize)
	}
	if e.YSize != 5 {
		t.Errorf("YSize=%d want 5", e.YSize)
	}
}

// ─── ZcAcceptEnter_0x02EB (< 20141022) ───────────────────────────────────────
//
// struct PACKET_ZC_ACCEPT_ENTER at PACKETVER < 20141022 (13 bytes):
//
//	0:2  PacketType   2:4  startTime   6:3  posDir[3]
//	9:1  xSize       10:1  ySize      11:2  font
//
// rAthena: packets.hpp DEFINE_PACKET_HEADER(ZC_ACCEPT_ENTER, 0x2eb)
// Condition: PACKETVER < 20141022 || PACKETVER >= 20160330
func TestZcAcceptEnter_0x02EB(t *testing.T) {
	b := make([]byte, 13)
	putI16LE(b, 0, 0x02EB)
	putU32LE(b, 2, 987654)
	b[6] = 0xB0
	b[7] = 0x78
	b[8] = 0x02
	b[9] = 5
	b[10] = 5
	putI16LE(b, 11, 7) // font

	e := ZcAcceptEnter_0x02EB(b, 20100101)

	if e.StartTime != 987654 {
		t.Errorf("StartTime=%d want 987654", e.StartTime)
	}
	if e.Font != 7 {
		t.Errorf("Font=%d want 7", e.Font)
	}
}

// ─── ItemPickup_0x029A ────────────────────────────────────────────────────────
//
// struct PACKET_ZC_ITEM_PICKUP_ACK at PACKETVER >= 20061218 (27 bytes):
//
//	 0:2 PacketType   2:2 Index   4:2 count   6:2 nameid
//	 8:1 IsIdentified 9:1 IsDamaged  10:1 refiningLevel
//	11:8 slot (EQUIPSLOTINFO)   19:2 location (uint16 at this pv)
//	21:1 type  22:1 result  23:4 HireExpireDate
//
// rAthena: packets_struct.hpp DEFINE_PACKET_HEADER(ZC_ITEM_PICKUP_ACK, 0x029a)
func TestItemPickup_0x029A(t *testing.T) {
	b := make([]byte, 27)
	putI16LE(b, 0, 0x029A)
	putU16LE(b, 2, 10)  // Index
	putU16LE(b, 4, 1)   // count
	putU16LE(b, 6, 501) // nameid
	b[8] = 1            // IsIdentified
	b[9] = 0            // IsDamaged
	b[10] = 0           // refiningLevel
	// slot: 8 bytes at offset 11 (EQUIPSLOTINFO = 4×uint16 = 8 bytes)
	putU16LE(b, 11, 100) // slot[0]
	putU16LE(b, 13, 0)
	putU16LE(b, 15, 0)
	putU16LE(b, 17, 0)
	putU16LE(b, 19, 4) // location (uint16 at this pv)
	b[21] = 4          // type
	b[22] = 0          // result (success)
	putI32LE(b, 23, 0) // HireExpireDate

	e := ItemPickup_0x029A(b, 20070101)

	if e.Index != 10 {
		t.Errorf("Index=%d want 10", e.Index)
	}
	if e.Nameid != 501 {
		t.Errorf("Nameid=%d want 501", e.Nameid)
	}
	if e.IsIdentified != 1 {
		t.Errorf("IsIdentified=%d want 1", e.IsIdentified)
	}
	if e.Result != 0 {
		t.Errorf("Result=%d want 0", e.Result)
	}
}

// ─── ZcReqTakeoffEquipAck_0x00AC ─────────────────────────────────────────────
//
// struct PACKET_ZC_REQ_TAKEOFF_EQUIP_ACK at PACKETVER < 20110824 (8 bytes):
//
//	0:2 PacketType   2:2 index   4:2 wearLocation   6:1 flag (bool)   7:pad
//
// rAthena: packets.hpp DEFINE_PACKET_HEADER(ZC_REQ_TAKEOFF_EQUIP_ACK, 0xac)
func TestZcReqTakeoffEquipAck_0x00AC(t *testing.T) {
	b := make([]byte, 8)
	putI16LE(b, 0, 0x00AC)
	putU16LE(b, 2, 5)    // index
	putU16LE(b, 4, 0x10) // wearLocation
	b[6] = 1             // flag (bool: 1 = success)

	e := ZcReqTakeoffEquipAck_0x00AC(b, 20050101)

	if e.Index != 5 {
		t.Errorf("Index=%d want 5", e.Index)
	}
	if e.WearLocation != 0x10 {
		t.Errorf("WearLocation=%d want 0x10", e.WearLocation)
	}
	if len(e.Flag) == 0 || e.Flag[0] != 1 {
		t.Errorf("Flag[0]=%v want 1", e.Flag)
	}
}

// ─── ZcReqWearEquipAck_0x0999 ────────────────────────────────────────────────
//
// struct PACKET_ZC_REQ_WEAR_EQUIP_ACK at MAIN >= 20121205 (12 bytes):
//
//	0:2 PacketType   2:2 index   4:4 wearLocation (u32)
//	8:2 wItemSpriteNumber   10:1 result
//
// rAthena: packets_struct.hpp DEFINE_PACKET_HEADER(ZC_REQ_WEAR_EQUIP_ACK, 0x0999)
func TestZcReqWearEquipAck_0x0999(t *testing.T) {
	b := make([]byte, 12)
	putI16LE(b, 0, 0x0999)
	putU16LE(b, 2, 3)      // index
	putU32LE(b, 4, 0x8000) // wearLocation
	putU16LE(b, 8, 42)     // wItemSpriteNumber
	b[10] = 0              // result (success)

	e := ZcReqWearEquipAck_0x0999(b, 20130101)

	if e.Index != 3 {
		t.Errorf("Index=%d want 3", e.Index)
	}
	if e.WearLocation != 0x8000 {
		t.Errorf("WearLocation=%d want 0x8000", e.WearLocation)
	}
	if e.Result != 0 {
		t.Errorf("Result=%d want 0", e.Result)
	}
}

// ─── Exp_0x07F6 ───────────────────────────────────────────────────────────────
//
// SYNTH_ZC_LONG_PAR_CHANGE (14 bytes) — derived from clif.cpp clif_displayexp():
//
//	0:2 PacketType   2:4 aid   6:4 exp (uint32)   10:2 type   12:2 quest
//
// Source: clif.cpp WFIFOW(fd,0)=cmd, WFIFOL(fd,2)=sd->id, WFIFOL(fd,6)=exp,
//
//	WFIFOW(fd,10)=type, WFIFOW(fd,12)=quest
func TestExp_0x07F6(t *testing.T) {
	b := make([]byte, 14)
	putI16LE(b, 0, 0x07F6)
	putU32LE(b, 2, 10001) // aid
	putU32LE(b, 6, 5000)  // exp value
	putU16LE(b, 10, 1)    // type: SP_BASEEXP=1
	putU16LE(b, 12, 0)    // quest flag

	e := Exp_0x07F6(b, 20100101)

	if e.Aid != 10001 {
		t.Errorf("Aid=%d want 10001", e.Aid)
	}
	if e.Exp != 5000 {
		t.Errorf("Exp=%d want 5000", e.Exp)
	}
	if e.Type != 1 {
		t.Errorf("Type=%d want 1 (SP_BASEEXP)", e.Type)
	}
	if e.Quest != 0 {
		t.Errorf("Quest=%d want 0", e.Quest)
	}
}

// ─── Exp_0x0ACC ───────────────────────────────────────────────────────────────
//
// SYNTH_ZC_LONG_PAR_CHANGE2 (18 bytes) — uint64 exp field:
//
//	0:2 PacketType   2:4 aid   6:8 exp (uint64)   14:2 type   16:2 quest
//
// Source: clif.cpp WFIFOQ(fd,6)=client_exp(exp), WFIFOW(fd,14)=type, WFIFOW(fd,16)=quest
func TestExp_0x0ACC(t *testing.T) {
	b := make([]byte, 18)
	putI16LE(b, 0, 0x0ACC)
	putU32LE(b, 2, 20002) // aid
	// uint64 exp at offset 6
	b[6] = 0x40
	b[7] = 0x42
	b[8] = 0x0F
	b[9] = 0
	b[10] = 0
	b[11] = 0
	b[12] = 0
	b[13] = 0          // = 1000000 in little-endian
	putU16LE(b, 14, 2) // type: SP_JOBEXP=2
	putU16LE(b, 16, 1) // quest flag

	e := Exp_0x0ACC(b, 20180101)

	if e.Aid != 20002 {
		t.Errorf("Aid=%d want 20002", e.Aid)
	}
	if e.Exp != 1000000 {
		t.Errorf("Exp=%d want 1000000", e.Exp)
	}
	if e.Type != 2 {
		t.Errorf("Type=%d want 2 (SP_JOBEXP)", e.Type)
	}
	if e.Quest != 1 {
		t.Errorf("Quest=%d want 1", e.Quest)
	}
}

// ─── ActorMoved_0x02EC (bug-fix) ─────────────────────────────────────────────
//
// packet_unit_walking at PACKETVER < 20091103 (67 bytes).
// At this packetver range there is NO PacketLength field (added at >= 20091103).
// objecttype at offset 2, GID at offset 3.
//
// rAthena: packets_struct.hpp unit_walkingType = 0x2ec [PACKETVER < 20091103]
func TestActorMoved_0x02EC(t *testing.T) {
	const size = 67
	b := make([]byte, size)
	putI16LE(b, 0, 0x02EC)
	b[2] = 1             // objecttype = PC (offset 2, no PacketLength at < 20091103)
	putU32LE(b, 3, 9999) // GID (offset 3)
	putI16LE(b, 7, 200)  // speed (offset 7)

	e := ActorMoved_0x02EC(b, 20080102)

	if e.GID != 9999 {
		t.Errorf("GID=%d want 9999", e.GID)
	}
	if e.Speed != 200 {
		t.Errorf("Speed=%d want 200", e.Speed)
	}
}

// ─── CharCreated_0x006D ───────────────────────────────────────────────────────
//
// struct PACKET_HC_ACCEPT_MAKECHAR (variable, contains CHARACTER_INFO).
// We test only that the function exists and returns an events.CharCreated
// without panicking on a minimal frame. Field-level correctness is inherited
// from the CHARACTER_INFO decode already tested by received_characters tests.
//
// rAthena: common/packets.hpp DEFINE_PACKET_HEADER(HC_ACCEPT_MAKECHAR, 0x6d)
func TestCharCreated_0x006D_DoesNotPanic(t *testing.T) {
	// CHARACTER_INFO is large (~147+ bytes depending on packetver).
	// Use a 150-byte frame to avoid slice-out-of-bounds panics.
	b := make([]byte, 150)
	putI16LE(b, 0, 0x006D)

	// Should not panic.
	var e events.CharCreated
	e = CharCreated_0x006D(b, 20100101)
	_ = e
}

// ─── ZcHoParChange_0x07DB ────────────────────────────────────────────────────
//
// struct PACKET_ZC_HO_PAR_CHANGE at PACKETVER else (8 bytes):
//
//	0:2 PacketType   2:2 type   4:4 value (int32)
//
// rAthena: packets.hpp DEFINE_PACKET_HEADER(ZC_HO_PAR_CHANGE, 0x7db)
func TestZcHoParChange_0x07DB(t *testing.T) {
	b := make([]byte, 8)
	putI16LE(b, 0, 0x07DB)
	putU16LE(b, 2, 32)    // type (some SP_ constant)
	putI32LE(b, 4, 12345) // value

	e := ZcHoParChange_0x07DB(b, 20100101)

	if e.Type != 32 {
		t.Errorf("Type=%d want 32", e.Type)
	}
	if e.Value != 12345 {
		t.Errorf("Value=%d want 12345", e.Value)
	}
}

// ─── ZcElParChange_0x081E ────────────────────────────────────────────────────
//
// struct PACKET_ZC_EL_PAR_CHANGE (8 bytes, unconditional):
//
//	0:2 PacketType   2:2 type   4:4 value (uint32)
//
// rAthena: packets.hpp DEFINE_PACKET_HEADER(ZC_EL_PAR_CHANGE, 0x81e)
func TestZcElParChange_0x081E(t *testing.T) {
	b := make([]byte, 8)
	putI16LE(b, 0, 0x081E)
	putU16LE(b, 2, 65)    // type
	putU32LE(b, 4, 99999) // value

	e := ZcElParChange_0x081E(b, 20130101)

	if e.Type != 65 {
		t.Errorf("Type=%d want 65", e.Type)
	}
	if e.Value != 99999 {
		t.Errorf("Value=%d want 99999", e.Value)
	}
}

// ─── benchmark: zero allocs on new decode functions ──────────────────────────

func BenchmarkZcAcceptEnter_0x0073(b *testing.B) {
	frame := make([]byte, 11)
	putI16LE(frame, 0, 0x0073)
	putU32LE(frame, 2, 100)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ZcAcceptEnter_0x0073(frame, 20050101)
	}
}

func BenchmarkExp_0x07F6(b *testing.B) {
	frame := make([]byte, 14)
	putI16LE(frame, 0, 0x07F6)
	putU32LE(frame, 2, 1001)
	putU32LE(frame, 6, 500)
	putU16LE(frame, 10, 1)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Exp_0x07F6(frame, 20100101)
	}
}

func BenchmarkItemPickup_0x029A(b *testing.B) {
	frame := make([]byte, 27)
	putI16LE(frame, 0, 0x029A)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ItemPickup_0x029A(frame, 20070101)
	}
}
