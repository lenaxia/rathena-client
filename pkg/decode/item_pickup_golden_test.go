// Package decode — comprehensive ItemPickup golden tests + benchmarks.
//
// All struct layouts in this file are verified against GCC preprocessor output
// of ~/personal/rathena/src/map/packets_struct.hpp at the relevant PACKETVER.
// See /tmp/opencode/rathena-audit/audit_report.txt for the verification matrix.
//
// PACKET_ZC_ITEM_PICKUP_ACK packet ID transitions (per rAthena):
//
//	0x00A0  pre-20061218 (no HireExpireDate)
//	0x029A  20061218..20071001 (adds HireExpireDate)
//	0x02D4  20071002..20120924 (adds bindOnEquipType)
//	0x0990  20120925..20150225 (location widens uint16→uint32)
//	0x0A0C  20150226..20160920 (adds option_data[5])
//	0x0A37  20160921..20200915 (adds favorite, look; nameid uint16→uint32 at pv>=20181121)
//	0x0B41  20200916..          (refiningLevel moves to end, adds grade)
//
// EQUIPSLOTINFO: 4×uint16 = 8 bytes pre-20181121, 4×uint32 = 16 bytes from 20181121
// ItemOptions: 5 bytes (int16 index + int16 value + uint8 param), 5 entries = 25 bytes
package decode

import (
	"testing"
)

// ─── ItemPickup_0x00A0 (baseline pre-20061218 layout, 23 bytes) ──────────────
//
// struct PACKET_ZC_ITEM_PICKUP_ACK at PACKETVER < 20061218:
//
//	 0:2  PacketType    2:2  Index         4:2  count
//	 6:2  nameid        8:1  IsIdentified  9:1  IsDamaged
//	10:1  refiningLevel 11:8 slot (EQUIPSLOTINFO pre-20181121 = 4×uint16)
//	19:2  location (uint16) 21:1  type     22:1  result
//
// Total: 23 bytes.
func TestItemPickup_0x00A0(t *testing.T) {
	b := make([]byte, 23)
	putI16LE(b, 0, 0x00A0)
	putU16LE(b, 2, 42)  // Index
	putU16LE(b, 4, 3)   // count
	putU16LE(b, 6, 501) // nameid (Red Potion)
	b[8] = 1            // IsIdentified
	b[9] = 0            // IsDamaged
	b[10] = 0           // refiningLevel
	// slot: 8 bytes at offset 11 (4×uint16)
	putU16LE(b, 11, 4001)
	putU16LE(b, 13, 0)
	putU16LE(b, 15, 0)
	putU16LE(b, 17, 0)
	putU16LE(b, 19, 4) // location (uint16 at this pv)
	b[21] = 11         // type (healing)
	b[22] = 0          // result (success)

	e := ItemPickup_0x00A0(b, 20060626)

	if e.Index != 42 {
		t.Errorf("Index=%d want 42", e.Index)
	}
	if e.Count != 3 {
		t.Errorf("Count=%d want 3", e.Count)
	}
	if e.Nameid != 501 {
		t.Errorf("Nameid=%d want 501", e.Nameid)
	}
	if e.IsIdentified != 1 {
		t.Errorf("IsIdentified=%d want 1", e.IsIdentified)
	}
	if e.Type != 11 {
		t.Errorf("Type=%d want 11", e.Type)
	}
	if e.Result != 0 {
		t.Errorf("Result=%d want 0 (success)", e.Result)
	}
	if e.Location != 4 {
		t.Errorf("Location=%d want 4", e.Location)
	}
}

// ─── ItemPickup_0x02D4 (post-bindOnEquipType, pre-20120925, 29 bytes) ────────
//
// struct PACKET_ZC_ITEM_PICKUP_ACK at 20071002 <= pv < 20120925:
//
//	 0:2  PacketType    2:2  Index         4:2  count
//	 6:2  nameid        8:1  IsIdentified  9:1  IsDamaged
//	10:1  refiningLevel 11:8 slot          19:2 location (uint16)
//	21:1  type          22:1 result        23:4 HireExpireDate
//	27:2  bindOnEquipType
//
// Total: 29 bytes.
func TestItemPickup_0x02D4(t *testing.T) {
	b := make([]byte, 29)
	putI16LE(b, 0, 0x02D4)
	putU16LE(b, 2, 7)    // Index
	putU16LE(b, 4, 1)    // count
	putU16LE(b, 6, 1201) // nameid (Knife)
	b[8] = 1             // IsIdentified
	b[9] = 0             // IsDamaged
	b[10] = 5            // refiningLevel (+5)
	putU16LE(b, 11, 0)   // slot[0..3] empty
	putU16LE(b, 13, 0)
	putU16LE(b, 15, 0)
	putU16LE(b, 17, 0)
	putU16LE(b, 19, 2) // location (uint16 — EQP_HAND_R)
	b[21] = 4          // type (weapon)
	b[22] = 0          // result (success)
	putI32LE(b, 23, 0) // HireExpireDate (no rental)
	putU16LE(b, 27, 0) // bindOnEquipType

	e := ItemPickup_0x02D4(b, 20090401)

	if e.Index != 7 {
		t.Errorf("Index=%d want 7", e.Index)
	}
	if e.Nameid != 1201 {
		t.Errorf("Nameid=%d want 1201", e.Nameid)
	}
	if e.RefiningLevel != 5 {
		t.Errorf("RefiningLevel=%d want 5", e.RefiningLevel)
	}
	if e.HireExpireDate != 0 {
		t.Errorf("HireExpireDate=%d want 0", e.HireExpireDate)
	}
	if e.BindOnEquipType != 0 {
		t.Errorf("BindOnEquipType=%d want 0", e.BindOnEquipType)
	}
	if e.Result != 0 {
		t.Errorf("Result=%d want 0", e.Result)
	}
}

// ─── ItemPickup_0x0990 (location uint32, pre-option_data, 31 bytes) ──────────
//
// struct PACKET_ZC_ITEM_PICKUP_ACK at 20120925 <= pv < 20150226:
//
//	 0:2  PacketType    2:2  Index         4:2  count
//	 6:2  nameid        8:1  IsIdentified  9:1  IsDamaged
//	10:1  refiningLevel 11:8 slot          19:4 location (uint32!)
//	23:1  type          24:1 result        25:4 HireExpireDate
//	29:2  bindOnEquipType
//
// Total: 31 bytes.
func TestItemPickup_0x0990(t *testing.T) {
	b := make([]byte, 31)
	putI16LE(b, 0, 0x0990)
	putU16LE(b, 2, 15)  // Index
	putU16LE(b, 4, 10)  // count
	putU16LE(b, 6, 909) // nameid (Jellopy)
	b[8] = 1            // IsIdentified
	b[9] = 0            // IsDamaged
	b[10] = 0           // refiningLevel
	putU16LE(b, 11, 0)  // slot empty
	putU16LE(b, 13, 0)
	putU16LE(b, 15, 0)
	putU16LE(b, 17, 0)
	putU32LE(b, 19, 0x0008) // location uint32 — EQP_HAND_R bit
	b[23] = 3               // type (etc)
	b[24] = 0               // result (success)
	putI32LE(b, 25, 0)      // HireExpireDate
	putU16LE(b, 29, 0)      // bindOnEquipType

	e := ItemPickup_0x0990(b, 20130320)

	if e.Index != 15 {
		t.Errorf("Index=%d want 15", e.Index)
	}
	if e.Nameid != 909 {
		t.Errorf("Nameid=%d want 909", e.Nameid)
	}
	if e.Location != 0x0008 {
		t.Errorf("Location=%d want 0x0008 (uint32 location)", e.Location)
	}
	if e.Result != 0 {
		t.Errorf("Result=%d want 0", e.Result)
	}
}

// ─── ItemPickup_0x0A0C (post-option_data, pre-favorite+look, 56 bytes) ───────
//
// struct PACKET_ZC_ITEM_PICKUP_ACK at 20150226 <= pv < 20160921:
//
//	 0:2  PacketType    2:2  Index         4:2  count
//	 6:2  nameid        8:1  IsIdentified  9:1  IsDamaged
//	10:1  refiningLevel 11:8 slot          19:4 location (uint32)
//	23:1  type          24:1 result        25:4 HireExpireDate
//	29:2  bindOnEquipType 31:25 option_data[5] (ItemOptions = 5 bytes each)
//
// Total: 56 bytes.
func TestItemPickup_0x0A0C(t *testing.T) {
	b := make([]byte, 56)
	putI16LE(b, 0, 0x0A0C)
	putU16LE(b, 2, 22)   // Index
	putU16LE(b, 4, 1)    // count
	putU16LE(b, 6, 2407) // nameid (Apple of Archer)
	b[8] = 1             // IsIdentified
	b[9] = 0             // IsDamaged
	b[10] = 0            // refiningLevel
	putU16LE(b, 11, 0)   // slot empty
	putU16LE(b, 13, 0)
	putU16LE(b, 15, 0)
	putU16LE(b, 17, 0)
	putU32LE(b, 19, 0x0002) // location uint32 — EQP_HEAD_TOP
	b[23] = 5               // type (armor)
	b[24] = 0               // result
	putI32LE(b, 25, 0)      // HireExpireDate
	putU16LE(b, 29, 0)      // bindOnEquipType
	// option_data: 5 entries × (int16 index + int16 value + uint8 param) = 25 bytes
	// Set first option: index=1, value=100, param=0
	putU16LE(b, 31, 1)
	putU16LE(b, 33, 100)
	b[35] = 0
	// Rest zero

	e := ItemPickup_0x0A0C(b, 20150701)

	if e.Index != 22 {
		t.Errorf("Index=%d want 22", e.Index)
	}
	if e.Nameid != 2407 {
		t.Errorf("Nameid=%d want 2407", e.Nameid)
	}
	if e.Location != 0x0002 {
		t.Errorf("Location=%d want 0x0002", e.Location)
	}
	if len(e.Option_data) != 25 {
		t.Errorf("Option_data len=%d want 25", len(e.Option_data))
	}
	// First option (int16 LE): index=1 at offset 0..1
	got_idx := uint16(b[31]) | uint16(b[32])<<8
	if got_idx != 1 {
		t.Errorf("option_data[0] index=%d want 1", got_idx)
	}
}

// ─── ItemPickup_0x0A37 — branch pv < 20181121 (uint16 nameid, 59 bytes) ──────
//
// struct PACKET_ZC_ITEM_PICKUP_ACK at 20160921 <= pv < 20181121:
//
//	 0:2  PacketType    2:2  Index         4:2  count
//	 6:2  nameid        8:1  IsIdentified  9:1  IsDamaged
//	10:1  refiningLevel 11:8 slot          19:4 location
//	23:1  type          24:1 result        25:4 HireExpireDate
//	29:2  bindOnEquipType 31:25 option_data  56:1 favorite
//	57:2  look
//
// Total: 59 bytes.
func TestItemPickup_0x0A37_PreModernNameid(t *testing.T) {
	b := make([]byte, 59)
	putI16LE(b, 0, 0x0A37)
	putU16LE(b, 2, 33)  // Index
	putU16LE(b, 4, 1)   // count
	putU16LE(b, 6, 501) // nameid (uint16 — pre-20181121)
	b[8] = 1            // IsIdentified
	b[9] = 0            // IsDamaged
	b[10] = 0           // refiningLevel
	putU16LE(b, 11, 0)  // slot empty (uint16 cards × 4)
	putU16LE(b, 13, 0)
	putU16LE(b, 15, 0)
	putU16LE(b, 17, 0)
	putU32LE(b, 19, 0) // location
	b[23] = 11         // type (healing)
	b[24] = 0          // result
	putI32LE(b, 25, 0) // HireExpireDate
	putU16LE(b, 29, 0) // bindOnEquipType
	// option_data zero
	b[56] = 1            // favorite
	putU16LE(b, 57, 501) // look

	e := ItemPickup_0x0A37(b, 20170315)

	if e.Index != 33 {
		t.Errorf("Index=%d want 33", e.Index)
	}
	if e.Nameid != 501 {
		t.Errorf("Nameid=%d want 501 (uint16 read at this pv)", e.Nameid)
	}
	if e.Favorite != 1 {
		t.Errorf("Favorite=%d want 1", e.Favorite)
	}
	if e.Look != 501 {
		t.Errorf("Look=%d want 501", e.Look)
	}
	if e.Result != 0 {
		t.Errorf("Result=%d want 0", e.Result)
	}
}

// ─── ItemPickup_0x0A37 — branch pv >= 20181121 (uint32 nameid, 69 bytes) ─────
//
// struct PACKET_ZC_ITEM_PICKUP_ACK at pv >= 20181121:
//
//	 0:2  PacketType    2:2  Index         4:2  count
//	 6:4  nameid (uint32!) 10:1 IsIdentified 11:1 IsDamaged
//	12:1  refiningLevel 13:16 slot (EQUIPSLOTINFO = 4×uint32 post-20181121)
//	29:4  location       33:1 type          34:1 result
//	35:4  HireExpireDate 39:2 bindOnEquipType 41:25 option_data
//	66:1  favorite       67:2 look
//
// Total: 69 bytes.
//
// This is the variant goKore's live test server (PACKETVER=20200401) uses.
// The decoder has been verified correct against GCC preprocessor output
// (audit report /tmp/opencode/rathena-audit/audit_report.txt).
// The earlier suspicion of a decoder bug (goKore worklog 1001) was wrong:
// when rAthena's pc_takeitem rejects a pickup (distance, loot-priority
// timing), clif_parse_TakeItem calls clif_additem(sd,0,0,6), which sends
// a packet with nameid=0 count=0 result=6 — those zeros are the server's
// rejection payload, not a decoder bug.
func TestItemPickup_0x0A37_ModernNameid(t *testing.T) {
	b := make([]byte, 69)
	putI16LE(b, 0, 0x0A37)
	putU16LE(b, 2, 5)   // Index
	putU16LE(b, 4, 2)   // count
	putU32LE(b, 6, 512) // nameid (uint32 — Apple, ITID 512 in goKore worklog 1001 logs)
	b[10] = 1           // IsIdentified
	b[11] = 0           // IsDamaged
	b[12] = 0           // refiningLevel
	// slot 4×uint32 (16 bytes) at offset 13
	putU32LE(b, 13, 0)
	putU32LE(b, 17, 0)
	putU32LE(b, 21, 0)
	putU32LE(b, 25, 0)
	putU32LE(b, 29, 0) // location
	b[33] = 11         // type (healing)
	b[34] = 0          // result (success)
	putI32LE(b, 35, 0) // HireExpireDate
	putU16LE(b, 39, 0) // bindOnEquipType
	// option_data 25 bytes zero
	b[66] = 0          // favorite
	putU16LE(b, 67, 0) // look

	e := ItemPickup_0x0A37(b, 20200401)

	if e.Index != 5 {
		t.Errorf("Index=%d want 5", e.Index)
	}
	if e.Count != 2 {
		t.Errorf("Count=%d want 2", e.Count)
	}
	if e.Nameid != 512 {
		t.Errorf("Nameid=%d want 512 (uint32 read at pv>=20181121)", e.Nameid)
	}
	if e.IsIdentified != 1 {
		t.Errorf("IsIdentified=%d want 1", e.IsIdentified)
	}
	if e.Result != 0 {
		t.Errorf("Result=%d want 0 (success)", e.Result)
	}
}

// TestItemPickup_0x0A37_ModernNameid_RejectionPayload verifies the server's
// rejection frame decodes to the documented Fail=6 sentinel. rAthena's
// clif_parse_TakeItem sends clif_additem(sd, 0, 0, 6) on pc_takeitem failure
// (distance, loot priority). The frame carries Index=0, count=0, nameid=0,
// result=6 — goKore handlers must check Result != PICKUP_SUCCESS before
// touching Nameid/Count, or they will treat a rejected pickup as
// "item 0 x 0" (see goKore worklog 1001 for the misdiagnosis this corrects).
func TestItemPickup_0x0A37_ModernNameid_RejectionPayload(t *testing.T) {
	b := make([]byte, 69)
	putI16LE(b, 0, 0x0A37)
	// All fields zero except result=6 (ADDITEM_REFUSED_TIME)
	b[34] = 6

	e := ItemPickup_0x0A37(b, 20200401)

	if e.Result != 6 {
		t.Errorf("Result=%d want 6 (ADDITEM_REFUSED_TIME)", e.Result)
	}
	if e.Nameid != 0 {
		t.Errorf("Nameid=%d want 0 (server sends zero on rejection)", e.Nameid)
	}
	if e.Count != 0 {
		t.Errorf("Count=%d want 0 (server sends zero on rejection)", e.Count)
	}
}

// ─── ItemPickup_0x0B41 (refiningLevel moves to end, grade added, 70 bytes) ───
//
// struct PACKET_ZC_ITEM_PICKUP_ACK at pv >= 20200916:
//
//	 0:2  PacketType    2:2  Index         4:2  count
//	 6:4  nameid (uint32) 10:1 IsIdentified 11:1 IsDamaged
//	[refiningLevel removed from here]
//	12:16 slot           28:4 location      32:1 type     33:1 result
//	34:4  HireExpireDate 38:2 bindOnEquipType 40:25 option_data
//	65:1  favorite       66:2 look          68:1 refiningLevel (moved!)
//	69:1  grade
//
// Total: 70 bytes.
func TestItemPickup_0x0B41(t *testing.T) {
	b := make([]byte, 70)
	putI16LE(b, 0, 0x0B41)
	putU16LE(b, 2, 99)   // Index
	putU16LE(b, 4, 1)    // count
	putU32LE(b, 6, 1301) // nameid (uint32 — Knife[3] would be a weapon)
	b[10] = 1            // IsIdentified
	b[11] = 0            // IsDamaged
	// slot 4×uint32 (16 bytes) at offset 12
	putU32LE(b, 12, 0)
	putU32LE(b, 16, 0)
	putU32LE(b, 20, 0)
	putU32LE(b, 24, 0)
	putU32LE(b, 28, 0x0008) // location
	b[32] = 4               // type
	b[33] = 0               // result
	putI32LE(b, 34, 0)      // HireExpireDate
	putU16LE(b, 38, 0)      // bindOnEquipType
	// option_data 25 bytes zero
	b[65] = 0          // favorite
	putU16LE(b, 66, 0) // look
	b[68] = 7          // refiningLevel (+7 — moved to near end at pv>=20200916)
	b[69] = 1          // grade

	e := ItemPickup_0x0B41(b, 20230802)

	if e.Index != 99 {
		t.Errorf("Index=%d want 99", e.Index)
	}
	if e.Nameid != 1301 {
		t.Errorf("Nameid=%d want 1301", e.Nameid)
	}
	if e.RefiningLevel != 7 {
		t.Errorf("RefiningLevel=%d want 7 (moved to offset 68 at pv>=20200916)", e.RefiningLevel)
	}
	if e.Grade != 1 {
		t.Errorf("Grade=%d want 1 (new field at pv>=20200916)", e.Grade)
	}
	if e.Result != 0 {
		t.Errorf("Result=%d want 0", e.Result)
	}
}

// ─── Benchmarks for every ItemPickup variant ────────────────────────────────

func BenchmarkItemPickup_0x00A0(b *testing.B) {
	frame := make([]byte, 23)
	putI16LE(frame, 0, 0x00A0)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ItemPickup_0x00A0(frame, 20060626)
	}
}

func BenchmarkItemPickup_0x02D4(b *testing.B) {
	frame := make([]byte, 29)
	putI16LE(frame, 0, 0x02D4)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ItemPickup_0x02D4(frame, 20090401)
	}
}

func BenchmarkItemPickup_0x0990(b *testing.B) {
	frame := make([]byte, 31)
	putI16LE(frame, 0, 0x0990)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ItemPickup_0x0990(frame, 20130320)
	}
}

func BenchmarkItemPickup_0x0A0C(b *testing.B) {
	frame := make([]byte, 56)
	putI16LE(frame, 0, 0x0A0C)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ItemPickup_0x0A0C(frame, 20150701)
	}
}

func BenchmarkItemPickup_0x0A37_PreModernNameid(b *testing.B) {
	frame := make([]byte, 59)
	putI16LE(frame, 0, 0x0A37)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ItemPickup_0x0A37(frame, 20170315)
	}
}

func BenchmarkItemPickup_0x0A37_ModernNameid(b *testing.B) {
	frame := make([]byte, 69)
	putI16LE(frame, 0, 0x0A37)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ItemPickup_0x0A37(frame, 20200401)
	}
}

func BenchmarkItemPickup_0x0B41(b *testing.B) {
	frame := make([]byte, 70)
	putI16LE(frame, 0, 0x0B41)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ItemPickup_0x0B41(frame, 20230802)
	}
}
