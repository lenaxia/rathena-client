// Manually implemented — see GitHub issue #7 and worklog for repair_item PACKETVER fix.
//
// Tests verify EncodeRepairItem against all three wire layouts derived from
// rAthena source (ground truth from GCC preprocessor):
//
//	packets_struct.hpp:2925 REPAIRITEM_INFO1   (packet 0x01FD)
//	packets_struct.hpp:2901 REPAIRITEM_INFO2   (packet 0x0B66, pv >= 20191224)
//	packets_struct.hpp:410  EQUIPSLOTINFO      (card[4] widens at pv >= 20181121)
//	packets_struct.hpp:2944 PACKET_CZ_REQ_ITEMREPAIR1
//	packets_struct.hpp:2937 PACKET_CZ_REQ_ITEMREPAIR2 (pv >= 20191224)
//	clif_packetdb.hpp:256,1977 packet registrations (REPAIR2 gated at pv >= 20191224)

package encode_test

import (
	"bytes"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/encode"
	"github.com/lenaxia/rathena-client/pkg/send"
)

// Verified wire layouts (offsets measured from start of packet including the
// 2-byte packet ID). Each verified empirically by compiling the actual rAthena
// C structs with __attribute__((packed)) and printing sizeof/offsetof:
//
//	EQUIPSLOTINFO narrow (pv<20181121):  uint16 card[4] = 8 bytes
//	EQUIPSLOTINFO wide   (pv>=20181121): uint32 card[4] = 16 bytes
//
//	REPAIRITEM_INFO1 narrow: index(2)+itemId(2)+refine(1)+slot(8)   = 13
//	REPAIRITEM_INFO1 wide:   index(2)+itemId(4)+refine(1)+slot(16)  = 23
//	REPAIRITEM_INFO2 (wide): index(2)+itemId(4)+slot(16)+refine(1)+grade(1) = 24
//
//	PACKET_CZ_REQ_ITEMREPAIR1 narrow =  2 + 13 = 15 bytes
//	PACKET_CZ_REQ_ITEMREPAIR1 wide   =  2 + 23 = 25 bytes
//	PACKET_CZ_REQ_ITEMREPAIR2        =  2 + 24 = 26 bytes

// TestEncodeRepairItem_PacketIDAndLength_AllVariants asserts the correct
// packet ID and total length are emitted for each PACKETVER regime. Boundary
// values are tested explicitly on both sides.
func TestEncodeRepairItem_PacketIDAndLength_AllVariants(t *testing.T) {
	req := send.RepairItem{
		Index:  7,
		ItemId: 1234,
		Refine: 3,
		Card:   [4]uint32{10, 20, 30, 40},
		Grade:  1,
	}
	cases := []struct {
		name    string
		pv      uint32
		wantLen int
		wantID  uint16 // wire packet ID (post-shuffle; these IDs are NOT in the shuffle table)
	}{
		// Variant A: REPAIR1 narrow (itemId uint16, slot uint16[4])
		{"narrow pre-20181121", 20180307, 15, 0x01FD},
		{"narrow boundary 20181120", 20181120, 15, 0x01FD},
		// Variant B: REPAIR1 wide (itemId uint32, slot uint32[4])
		{"wide boundary 20181121", 20181121, 25, 0x01FD},
		{"wide mid 20190000", 20190000, 25, 0x01FD},
		{"wide boundary 20191223", 20191223, 25, 0x01FD},
		// Variant C: REPAIR2 (0x0B66) — registered only when PACKETVER >= 20191224
		{"repair2 boundary 20191224", 20191224, 26, 0x0B66},
		{"repair2 goKore production 20200401", 20200401, 26, 0x0B66},
		{"repair2 future 20200916", 20200916, 26, 0x0B66},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := encode.EncodeRepairItem(req, tc.pv)
			if len(p) != tc.wantLen {
				t.Fatalf("pv=%d: length = %d, want %d", tc.pv, len(p), tc.wantLen)
			}
			gotID := uint16(p[0]) | uint16(p[1])<<8
			if gotID != tc.wantID {
				t.Fatalf("pv=%d: packet ID = 0x%04X, want 0x%04X", tc.pv, gotID, tc.wantID)
			}
		})
	}
}

// TestEncodeRepairItem_GoldenBytes_Narrow hand-synthesizes the 15-byte
// REPAIR1 narrow wire layout byte-by-byte from the rAthena struct definition
// and confirms EncodeRepairItem matches exactly.
//
// Layout (PACKET_CZ_REQ_ITEMREPAIR1 narrow, pv < 20181121):
//
//	[0..1]   packetType = 0x01FD LE
//	[2..3]   index       int16  LE
//	[4..5]   itemId      uint16 LE  (narrowed from req.ItemId)
//	[6]      refine      uint8
//	[7..8]   card[0]     uint16 LE
//	[9..10]  card[1]     uint16 LE
//	[11..12] card[2]     uint16 LE
//	[13..14] card[3]     uint16 LE
func TestEncodeRepairItem_GoldenBytes_Narrow(t *testing.T) {
	req := send.RepairItem{
		Index:  0x1234,
		ItemId: 0xABCD,
		Refine: 5,
		Card:   [4]uint32{0x1111, 0x2222, 0x3333, 0x4444},
	}
	want := []byte{
		0xFD, 0x01, // packet ID 0x01FD LE
		0x34, 0x12, // index 0x1234 LE
		0xCD, 0xAB, // itemId 0xABCD LE (narrowed to uint16)
		0x05, // refine
		0x11, 0x11, // card[0]
		0x22, 0x22, // card[1]
		0x33, 0x33, // card[2]
		0x44, 0x44, // card[3]
	}
	p := encode.EncodeRepairItem(req, 20180307)
	if len(p) != 15 {
		t.Fatalf("length: got %d, want 15", len(p))
	}
	if !bytes.Equal(p, want) {
		t.Fatalf("narrow bytes mismatch:\n got % X\n want % X", p, want)
	}
}

// TestEncodeRepairItem_GoldenBytes_Wide hand-synthesizes the 25-byte REPAIR1
// wide wire layout. This is the BUG scenario from issue #7 — at pv=20200401
// production servers expect this layout (or REPAIR2), not the old 15-byte form.
//
// Layout (PACKET_CZ_REQ_ITEMREPAIR1 wide, 20181121 <= pv < 20191224):
//
//	[0..1]  packetType = 0x01FD LE
//	[2..3]  index       int16  LE
//	[4..7]  itemId      uint32 LE
//	[8]     refine      uint8
//	[9..12] card[0]     uint32 LE
//	[13..16]card[1]     uint32 LE
//	[17..20]card[2]     uint32 LE
//	[21..24]card[3]     uint32 LE
func TestEncodeRepairItem_GoldenBytes_Wide(t *testing.T) {
	req := send.RepairItem{
		Index:  0x5678,
		ItemId: 0x12345678,
		Refine: 7,
		Card:   [4]uint32{0x11223344, 0x55667788, 0x99AABBCC, 0xDDEEFF00},
	}
	want := []byte{
		0xFD, 0x01, // packet ID 0x01FD LE
		0x78, 0x56, // index 0x5678 LE
		0x78, 0x56, 0x34, 0x12, // itemId 0x12345678 LE
		0x07, // refine
		0x44, 0x33, 0x22, 0x11, // card[0]
		0x88, 0x77, 0x66, 0x55, // card[1]
		0xCC, 0xBB, 0xAA, 0x99, // card[2]
		0x00, 0xFF, 0xEE, 0xDD, // card[3]
	}
	// 20181121 boundary: wide layout
	p := encode.EncodeRepairItem(req, 20181121)
	if len(p) != 25 {
		t.Fatalf("length: got %d, want 25", len(p))
	}
	if !bytes.Equal(p, want) {
		t.Fatalf("wide bytes mismatch:\n got % X\n want % X", p, want)
	}
	// 20191223 boundary: still REPAIR1 wide
	p2 := encode.EncodeRepairItem(req, 20191223)
	if len(p2) != 25 || !bytes.Equal(p2, want) {
		t.Fatalf("20191223 (last wide day): got % X, want % X", p2, want)
	}
}

// TestEncodeRepairItem_GoldenBytes_Repair2 hand-synthesizes the 26-byte
// REPAIR2 layout. NOTE the field order differs from REPAIR1: slot comes
// BEFORE refine, and a new grade byte is appended at the end.
//
// Layout (PACKET_CZ_REQ_ITEMREPAIR2, pv >= 20191224):
//
//	[0..1]   packetType = 0x0B66 LE
//	[2..3]   index       int16  LE
//	[4..7]   itemId      uint32 LE
//	[8..11]  card[0]     uint32 LE   (slot)
//	[12..15] card[1]     uint32 LE   (slot)
//	[16..19] card[2]     uint32 LE   (slot)
//	[20..23] card[3]     uint32 LE   (slot)
//	[24]     refine      uint8
//	[25]     grade       uint8
func TestEncodeRepairItem_GoldenBytes_Repair2(t *testing.T) {
	req := send.RepairItem{
		Index:  0x4242,
		ItemId: 0xDEADBEEF,
		Refine: 9,
		Card:   [4]uint32{0x01020304, 0x05060708, 0x090A0B0C, 0x0D0E0F10},
		Grade:  2,
	}
	want := []byte{
		0x66, 0x0B, // packet ID 0x0B66 LE
		0x42, 0x42, // index 0x4242 LE
		0xEF, 0xBE, 0xAD, 0xDE, // itemId 0xDEADBEEF LE
		0x04, 0x03, 0x02, 0x01, // card[0] (slot BEFORE refine)
		0x08, 0x07, 0x06, 0x05, // card[1]
		0x0C, 0x0B, 0x0A, 0x09, // card[2]
		0x10, 0x0F, 0x0E, 0x0D, // card[3]
		0x09, // refine (AFTER slot in REPAIR2)
		0x02, // grade
	}
	p := encode.EncodeRepairItem(req, 20200401)
	if len(p) != 26 {
		t.Fatalf("length: got %d, want 26", len(p))
	}
	if !bytes.Equal(p, want) {
		t.Fatalf("repair2 bytes mismatch:\n got % X\n want % X", p, want)
	}
	// 20191224 boundary: first REPAIR2 day
	p2 := encode.EncodeRepairItem(req, 20191224)
	if len(p2) != 26 || !bytes.Equal(p2, want) {
		t.Fatalf("20191224 (first repair2 day): got % X, want % X", p2, want)
	}
}

// TestEncodeRepairItem_FieldOrder_Repair2 confirms the critical layout
// difference: in REPAIR2, slot comes BEFORE refine (and grade is appended).
// This is what the issue's investigation uncovered from the rAthena source:
//
//	REPAIRITEM_INFO1: index, itemId, refine, slot
//	REPAIRITEM_INFO2: index, itemId, slot, refine, grade
func TestEncodeRepairItem_FieldOrder_Repair2(t *testing.T) {
	req := send.RepairItem{
		Index:  0,
		ItemId: 0,
		Refine: 0xAA,
		Card:   [4]uint32{0, 0, 0, 0},
		Grade:  0xBB,
	}
	// REPAIR1 wide: refine at offset 8, card at 9..24, no grade.
	p1 := encode.EncodeRepairItem(req, 20190000)
	if p1[8] != 0xAA {
		t.Fatalf("REPAIR1: refine byte at offset 8 = 0x%02X, want 0xAA", p1[8])
	}
	for i := 9; i < 25; i++ {
		if p1[i] != 0 {
			t.Fatalf("REPAIR1: card slot at offset %d = 0x%02X, want 0", i, p1[i])
		}
	}

	// REPAIR2: slot (zero) at 8..23, refine at 24, grade at 25.
	p2 := encode.EncodeRepairItem(req, 20200401)
	for i := 8; i < 24; i++ {
		if p2[i] != 0 {
			t.Fatalf("REPAIR2: card slot at offset %d = 0x%02X, want 0", i, p2[i])
		}
	}
	if p2[24] != 0xAA {
		t.Fatalf("REPAIR2: refine at offset 24 = 0x%02X, want 0xAA", p2[24])
	}
	if p2[25] != 0xBB {
		t.Fatalf("REPAIR2: grade at offset 25 = 0x%02X, want 0xBB", p2[25])
	}
}

// TestEncodeRepairItem_CardNarrowing verifies that uint32 cards are truncated
// to uint16 on the narrow wire layout (pv < 20181121). The caller is
// responsible for ensuring card values fit in uint16 when targeting narrow
// servers; the encoder matches rAthena behavior (uint16 truncation).
func TestEncodeRepairItem_CardNarrowing(t *testing.T) {
	// Card value 0x12345678 narrowed to uint16 = 0x5678 → bytes 0x78 0x56 LE.
	req := send.RepairItem{
		Card: [4]uint32{0x12345678, 0, 0, 0},
	}
	p := encode.EncodeRepairItem(req, 20180307)
	if p[7] != 0x78 || p[8] != 0x56 {
		t.Fatalf("narrowed card[0]: got %02X %02X, want 78 56 (0x5678 LE)", p[7], p[8])
	}
}

// TestEncodeRepairItem_ItemIdNarrowing verifies itemId truncation on narrow
// layout. Matches rAthena EQUIPSLOTINFO/NORMALITEM_INFO uint16 encoding.
func TestEncodeRepairItem_ItemIdNarrowing(t *testing.T) {
	req := send.RepairItem{ItemId: 0x1234ABCD}
	p := encode.EncodeRepairItem(req, 20180307)
	if p[4] != 0xCD || p[5] != 0xAB {
		t.Fatalf("narrowed itemId: got %02X %02X, want CD AB (0xABCD LE)", p[4], p[5])
	}
}

// TestEncodeRepairItem_Boundaries verifies the two critical boundaries:
//
//	20181120 → 15 bytes (narrow), 20181121 → 25 bytes (wide)
//	20191223 → 25 bytes (REPAIR1), 20191224 → 26 bytes (REPAIR2)
//
// Boundary tests catch off-by-one errors in the packetver comparisons
// (must match rAthena's >= conditions exactly).
func TestEncodeRepairItem_Boundaries(t *testing.T) {
	req := send.RepairItem{Index: 1, ItemId: 1, Refine: 1, Card: [4]uint32{1, 1, 1, 1}}

	if p := encode.EncodeRepairItem(req, 20181120); len(p) != 15 {
		t.Errorf("pv=20181120 (last narrow day): len=%d, want 15", len(p))
	}
	if p := encode.EncodeRepairItem(req, 20181121); len(p) != 25 {
		t.Errorf("pv=20181121 (first wide day): len=%d, want 25", len(p))
	}
	if p := encode.EncodeRepairItem(req, 20191223); len(p) != 25 {
		t.Errorf("pv=20191223 (last REPAIR1 day): len=%d, want 25", len(p))
	}
	if p := encode.EncodeRepairItem(req, 20191224); len(p) != 26 {
		t.Errorf("pv=20191224 (first REPAIR2 day): len=%d, want 26", len(p))
	}
}

// TestEncodeRepairItem_PacketIDVaries confirms different packetvers produce
// different wire IDs (catches the original bug where _ = packetver discarded
// the argument entirely).
func TestEncodeRepairItem_PacketIDVaries(t *testing.T) {
	req := send.RepairItem{Index: 1, ItemId: 1, Refine: 1}
	pNarrow := encode.EncodeRepairItem(req, 20180307)
	pWide := encode.EncodeRepairItem(req, 20190000)
	pRepair2 := encode.EncodeRepairItem(req, 20200401)
	// Narrow and wide share the wire ID 0x01FD but differ in length.
	idNarrow := uint16(pNarrow[0]) | uint16(pNarrow[1])<<8
	idWide := uint16(pWide[0]) | uint16(pWide[1])<<8
	idRepair2 := uint16(pRepair2[0]) | uint16(pRepair2[1])<<8
	if idNarrow != idWide {
		t.Errorf("narrow/wire ID differ: 0x%04X vs 0x%04X (both should be 0x01FD)", idNarrow, idWide)
	}
	if idNarrow == idRepair2 {
		t.Errorf("expected REPAIR2 (0x0B66) to differ from REPAIR1 (0x01FD), both = 0x%04X", idNarrow)
	}
}

// TestEncodeRepairItem_RoundTrip index is the only field read by the server
// dispatcher (clif.cpp:13285: skill_repairweapon(*sd, p->item.index)). Confirm
// index survives round-trip in all three layouts at offset [2..3].
func TestEncodeRepairItem_IndexAlwaysAtOffset2(t *testing.T) {
	req := send.RepairItem{Index: 0x4321}
	for _, pv := range []uint32{20180307, 20181121, 20200401} {
		p := encode.EncodeRepairItem(req, pv)
		if p[2] != 0x21 || p[3] != 0x43 {
			t.Errorf("pv=%d: index at offset 2-3 = %02X %02X, want 21 43", pv, p[2], p[3])
		}
	}
}

func BenchmarkEncodeRepairItem_Narrow(b *testing.B) {
	req := send.RepairItem{Index: 7, ItemId: 1234, Refine: 3, Card: [4]uint32{10, 20, 30, 40}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodeRepairItem(req, 20180307)
	}
}

func BenchmarkEncodeRepairItem_Wide(b *testing.B) {
	req := send.RepairItem{Index: 7, ItemId: 1234, Refine: 3, Card: [4]uint32{10, 20, 30, 40}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodeRepairItem(req, 20190000)
	}
}

func BenchmarkEncodeRepairItem_Repair2(b *testing.B) {
	req := send.RepairItem{Index: 7, ItemId: 1234, Refine: 3, Card: [4]uint32{10, 20, 30, 40}, Grade: 1}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodeRepairItem(req, 20200401)
	}
}
