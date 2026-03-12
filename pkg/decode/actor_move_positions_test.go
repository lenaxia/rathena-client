// Package decode — wire-level tests for actor move position decoding.
//
// These tests feed real captured wire bytes (from goKore-test dumps) through
// the decode functions, then pass the resulting PosDir / MoveData fields
// through packing.DecodePosDir and packing.DecodeMoveData to assert that the
// concrete map coordinates match what the game session reports.
//
// Sources:
//   - ~/personal/goKore-test/docs/03_REFERENCE/dumps/DUMP8_movement (2026-01-17)
//     bot "botijo16" on geffen.gat / prontera approach (x≈52)
//   - ~/personal/goKore-test/docs/03_REFERENCE/dumps/DUMP9_melee    (2026-01-17)
//     bot fighting Porings on gef_fild07 (x≈148–170, y≈342–358)
//   - ~/personal/goKore-test/docs/03_REFERENCE/dumps/DUMP2          (2025-04-30)
//     Chonchon monster on gef_fild07 (x=258)
//
// Packet formats verified via GCC preprocessor:
//
//	PACKET_ZC_NOTIFY_PLAYERMOVE  (0x0087) — fixed 12 bytes, all versions
//	struct packet_unit_walking   (0x09FD) — 114 bytes at PACKETVER=20181121
//	struct packet_idle_unit      (0x09FF) — 108 bytes at PACKETVER=20181121
//	SYNTH_ZC_NOTIFY_MOVE         (0x0086) — fixed 16 bytes, all versions
//
// Wire format reference (README-LLM.md "Non-Trivial Wire Formats"):
//
//	WBUFPOS  (3-byte): Byte0=[x9..x2] Byte1=[x1 x0 y9..y4] Byte2=[y3..y0 d3..d0]
//	WBUFPOS2 (6-byte): p[0..4] = fromX(10b) fromY(10b) toX(10b) toY(10b)
//	                   p[5]    = sx0(4b) sy0(4b)  — NOT a direction value
package decode

import (
	"testing"

	"github.com/lenaxia/rathena-client/pkg/packing"
)

// ─── CharacterMoves_0x0087 — self-move decode with real wire bytes ─────────────
//
// From DUMP8_movement (2026-01-17), bot "botijo16" moves south on geffen/prontera.
// Nine consecutive 0x0087 packets captured; pkts 1–7 keep X=52, pkts 8–9 vary X.
//
// Packet layout (12 bytes):
//
//	offset 0: packetType uint16
//	offset 2: moveStartTime uint32 (server tick, LE)
//	offset 6: MoveData [6]byte (WBUFPOS2)

// DUMP8 pkt1: 87 00 E4 2E A6 03 0D 08 00 D0 76 88
// Decoded: fromX=52 fromY=128 toX=52 toY=118 — first move from spawn
var dump8Move1 = [12]byte{0x87, 0x00, 0xE4, 0x2E, 0xA6, 0x03, 0x0D, 0x08, 0x00, 0xD0, 0x76, 0x88}

// DUMP8 pkt8: 87 00 0E 33 A6 03 0D 07 90 D4 70 88
// Decoded: fromX=52 fromY=121 toX=53 toY=112 — first packet where toX changes
var dump8Move8 = [12]byte{0x87, 0x00, 0x0E, 0x33, 0xA6, 0x03, 0x0D, 0x07, 0x90, 0xD4, 0x70, 0x88}

// DUMP8 pkt9: 87 00 E0 33 A6 03 0D 47 80 D8 6F 88
// Decoded: fromX=53 fromY=120 toX=54 toY=111 — both fromX and toX advance
var dump8Move9 = [12]byte{0x87, 0x00, 0xE0, 0x33, 0xA6, 0x03, 0x0D, 0x47, 0x80, 0xD8, 0x6F, 0x88}

// DUMP9_melee pkt1: 87 00 0A E3 A7 03 28 96 42 89 62 88
// Decoded: fromX=162 fromY=356 toX=162 toY=354 — gef_fild07, different coord range
var dump9Move1 = [12]byte{0x87, 0x00, 0x0A, 0xE3, 0xA7, 0x03, 0x28, 0x96, 0x42, 0x89, 0x62, 0x88}

// DUMP9_melee pkt2: 87 00 A0 E3 A7 03 28 96 32 89 62 88
// Decoded: fromX=162 fromY=355 toX=162 toY=354 — one step closer to target
var dump9Move2 = [12]byte{0x87, 0x00, 0xA0, 0xE3, 0xA7, 0x03, 0x28, 0x96, 0x32, 0x89, 0x62, 0x88}

func TestCharacterMoves_0x0087_WireDecode_Pkt1_FromTo(t *testing.T) {
	e := CharacterMoves_0x0087(dump8Move1[:], 20181121)

	fromX, fromY, toX, toY, _, _ := packing.DecodeMoveData(e.MoveData[:])

	if fromX != 52 {
		t.Errorf("fromX: got %d want 52", fromX)
	}
	if fromY != 128 {
		t.Errorf("fromY: got %d want 128", fromY)
	}
	if toX != 52 {
		t.Errorf("toX: got %d want 52", toX)
	}
	if toY != 118 {
		t.Errorf("toY: got %d want 118", toY)
	}
	if e.MoveStartTime == 0 {
		t.Errorf("MoveStartTime: want non-zero, got 0")
	}
}

// TestCharacterMoves_0x0087_WireDecode_Pkt8_XChanges asserts that when the path
// curves, toX differs from fromX (first packet where X changes in this session).
func TestCharacterMoves_0x0087_WireDecode_Pkt8_XChanges(t *testing.T) {
	e := CharacterMoves_0x0087(dump8Move8[:], 20181121)

	fromX, fromY, toX, toY, _, _ := packing.DecodeMoveData(e.MoveData[:])

	if fromX != 52 {
		t.Errorf("fromX: got %d want 52", fromX)
	}
	if fromY != 121 {
		t.Errorf("fromY: got %d want 121", fromY)
	}
	// toX advances by 1 — the path turns here
	if toX != 53 {
		t.Errorf("toX: got %d want 53", toX)
	}
	if toY != 112 {
		t.Errorf("toY: got %d want 112", toY)
	}
}

// TestCharacterMoves_0x0087_WireDecode_Pkt9_BothXChange asserts that both
// fromX and toX are different from pkt8, confirming full coordinate progression.
func TestCharacterMoves_0x0087_WireDecode_Pkt9_BothXChange(t *testing.T) {
	e := CharacterMoves_0x0087(dump8Move9[:], 20181121)

	fromX, fromY, toX, toY, _, _ := packing.DecodeMoveData(e.MoveData[:])

	if fromX != 53 {
		t.Errorf("fromX: got %d want 53", fromX)
	}
	if fromY != 120 {
		t.Errorf("fromY: got %d want 120", fromY)
	}
	if toX != 54 {
		t.Errorf("toX: got %d want 54", toX)
	}
	if toY != 111 {
		t.Errorf("toY: got %d want 111", toY)
	}
}

// TestCharacterMoves_0x0087_WireDecode_Byte5IsNotDirection asserts the goKore-v1
// known bug is NOT present: byte 5 of MoveData is sx0/sy0, NOT a direction.
// In these captured packets byte 5 = 0x88 → sx0=8, sy0=8.
func TestCharacterMoves_0x0087_WireDecode_Byte5IsNotDirection(t *testing.T) {
	e := CharacterMoves_0x0087(dump8Move1[:], 20181121)

	_, _, _, _, sx0, sy0 := packing.DecodeMoveData(e.MoveData[:])

	if sx0 != 8 {
		t.Errorf("sx0: got %d want 8 (byte5=0x88; NOT a direction)", sx0)
	}
	if sy0 != 8 {
		t.Errorf("sy0: got %d want 8 (byte5=0x88; NOT a direction)", sy0)
	}
}

// TestCharacterMoves_0x0087_WireDecode_DUMP9_DifferentMap verifies decoding on
// gef_fild07 where coordinates are in the 160–360 range (vs prontera's ~52).
func TestCharacterMoves_0x0087_WireDecode_DUMP9_DifferentMap(t *testing.T) {
	tests := []struct {
		name                   string
		data                   [12]byte
		fromX, fromY, toX, toY uint16
	}{
		// Bot approaching a Poring on gef_fild07 (two steps captured)
		{"gef_fild07_pkt1", dump9Move1, 162, 356, 162, 354},
		{"gef_fild07_pkt2", dump9Move2, 162, 355, 162, 354},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := CharacterMoves_0x0087(tt.data[:], 20181121)
			fromX, fromY, toX, toY, _, _ := packing.DecodeMoveData(e.MoveData[:])
			if fromX != tt.fromX {
				t.Errorf("fromX: got %d want %d", fromX, tt.fromX)
			}
			if fromY != tt.fromY {
				t.Errorf("fromY: got %d want %d", fromY, tt.fromY)
			}
			if toX != tt.toX {
				t.Errorf("toX: got %d want %d", toX, tt.toX)
			}
			if toY != tt.toY {
				t.Errorf("toY: got %d want %d", toY, tt.toY)
			}
		})
	}
}

func BenchmarkCharacterMoves_0x0087_WireDecode(b *testing.B) {
	data := dump8Move1[:]
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e := CharacterMoves_0x0087(data, 20181121)
		_, _, _, _, _, _ = packing.DecodeMoveData(e.MoveData[:])
	}
}

// ─── ActorMoved (0x09FD) — NPC/monster move with real wire bytes ──────────────
//
// Packet layout (114 bytes at PACKETVER=20181121):
//
//	offset 0:  packetType uint16
//	offset 2:  packetLength uint16
//	offset 4:  objecttype uint8
//	offset 5:  AID uint32
//	offset 9:  GID uint32
//	offset 13: speed uint16
//	offset 67: MoveData [6]byte (WBUFPOS2)
//	offset 90: name [24]byte
//
// DUMP2 (2025-04-30): Chonchon monster on gef_fild07 — from=(258,174) to=(258,177)
//
//	FD 09 72 00 05 63 01 8F 06 00 00 00 00 C8 00 00
//	00 00 00 00 00 00 00 F3 03 00 00 00 00 00 00 00
//	00 00 00 00 CE 9B 83 02 00 00 00 00 00 00 00 00
//	00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
//	00 00 00 40 8A E4 08 B1 88 00 00 05 00 00 00 FF
//	FF FF FF FF FF FF FF 00 00 00 43 68 6F 6E 63 68
//	6F 6E 00 00 00 00 00 00 00 00 00 00 00 00 00 00
//	00 00
//
// AID=110035299 GID=0 speed=200 objecttype=5(MOB) name="Chonchon"
var dump2ActorMoved0x09FD = [114]byte{
	0xFD, 0x09, 0x72, 0x00, 0x05, 0x63, 0x01, 0x8F,
	0x06, 0x00, 0x00, 0x00, 0x00, 0xC8, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xF3,
	0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0xCE, 0x9B, 0x83, 0x02,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x40, 0x8A, 0xE4, 0x08, 0xB1,
	0x88, 0x00, 0x00, 0x05, 0x00, 0x00, 0x00, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x00,
	0x00, 0x00, 0x43, 0x68, 0x6F, 0x6E, 0x63, 0x68,
	0x6F, 0x6E, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00,
}

// DUMP9_melee (2026-01-17): Poring monsters on gef_fild07 — coordinates vary
// in both X and Y across packets, providing comprehensive bit-pattern coverage.
//
// Poring1 (AID=110039402): from=(162,342) to=(165,348)  — X increases
// MoveData[67:73] = 28 95 62 95 5C 88
var dump9Poring1ActorMoved = [114]byte{
	0xFD, 0x09, 0x72, 0x00, 0x05, 0x6A, 0x11, 0x8F,
	0x06, 0x00, 0x00, 0x00, 0x00, 0x90, 0x01, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xEA,
	0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0xFE, 0xDE, 0xA7,
	0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x28, 0x95, 0x62, 0x95, 0x5C,
	0x88, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x00,
	0x00, 0x00, 0x50, 0x6F, 0x72, 0x69, 0x6E, 0x67,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00,
}

// Poring2 (AID=110039396): from=(165,351) to=(161,354)  — X decreases, Y increases
// MoveData[67:73] = 29 55 F2 85 62 88
var dump9Poring2ActorMoved = [114]byte{
	0xFD, 0x09, 0x72, 0x00, 0x05, 0x64, 0x11, 0x8F,
	0x06, 0x00, 0x00, 0x00, 0x00, 0x90, 0x01, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xEA,
	0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0xFE, 0xDE, 0xA7,
	0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x29, 0x55, 0xF2, 0x85, 0x62,
	0x88, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x00,
	0x00, 0x00, 0x50, 0x6F, 0x72, 0x69, 0x6E, 0x67,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00,
}

// Poring3 (AID=110039416): from=(170,350) to=(164,355)  — large X decrease
// MoveData[67:73] = 2A 95 E2 91 63 88
var dump9Poring3ActorMoved = [114]byte{
	0xFD, 0x09, 0x72, 0x00, 0x05, 0x78, 0x11, 0x8F,
	0x06, 0x00, 0x00, 0x00, 0x00, 0x90, 0x01, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xEA,
	0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0xFE, 0xDE, 0xA7,
	0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x2A, 0x95, 0xE2, 0x91, 0x63,
	0x88, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x00,
	0x00, 0x00, 0x50, 0x6F, 0x72, 0x69, 0x6E, 0x67,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00,
}

// Poring4 (AID=110039409): from=(148,358) to=(150,358)  — X changes, Y stays same
// MoveData[67:73] = 25 16 62 59 66 88
var dump9Poring4ActorMoved = [114]byte{
	0xFD, 0x09, 0x72, 0x00, 0x05, 0x71, 0x11, 0x8F,
	0x06, 0x00, 0x00, 0x00, 0x00, 0x90, 0x01, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xEA,
	0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x50, 0xE5, 0xA7,
	0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x25, 0x16, 0x62, 0x59, 0x66,
	0x88, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x00,
	0x00, 0x00, 0x50, 0x6F, 0x72, 0x69, 0x6E, 0x67,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00,
}

func TestActorMoved_0x09FD_WireDecode_Chonchon_FromTo(t *testing.T) {
	e := ActorMoved_0x09FD(dump2ActorMoved0x09FD[:], 20181121)

	fromX, fromY, toX, toY, _, _ := packing.DecodeMoveData(e.MoveData[:])

	if fromX != 258 {
		t.Errorf("fromX: got %d want 258", fromX)
	}
	if fromY != 174 {
		t.Errorf("fromY: got %d want 174", fromY)
	}
	if toX != 258 {
		t.Errorf("toX: got %d want 258", toX)
	}
	if toY != 177 {
		t.Errorf("toY: got %d want 177", toY)
	}
}

func TestActorMoved_0x09FD_WireDecode_Chonchon_MetaFields(t *testing.T) {
	e := ActorMoved_0x09FD(dump2ActorMoved0x09FD[:], 20181121)

	if e.AID != 110035299 {
		t.Errorf("AID: got %d want 110035299", e.AID)
	}
	if e.GID != 0 {
		t.Errorf("GID: got %d want 0", e.GID)
	}
	if e.Speed != 200 {
		t.Errorf("Speed: got %d want 200", e.Speed)
	}
	if e.Objecttype != 5 {
		t.Errorf("Objecttype: got %d want 5 (MOB)", e.Objecttype)
	}
	if e.Name != "Chonchon" {
		t.Errorf("Name: got %q want %q", e.Name, "Chonchon")
	}
}

func TestActorMoved_0x09FD_WireDecode_Chonchon_Byte5IsNotDirection(t *testing.T) {
	e := ActorMoved_0x09FD(dump2ActorMoved0x09FD[:], 20181121)

	// MoveData[67:73]: 40 8A E4 08 B1 88 — byte5=0x88 → sx0=8 sy0=8 (not direction)
	_, _, _, _, sx0, sy0 := packing.DecodeMoveData(e.MoveData[:])

	if sx0 != 8 {
		t.Errorf("sx0: got %d want 8", sx0)
	}
	if sy0 != 8 {
		t.Errorf("sy0: got %d want 8", sy0)
	}
}

// TestActorMoved_0x09FD_WireDecode_Porings verifies that Poring move packets
// from gef_fild07 decode to the expected from/to coordinates. Each Poring has
// a distinct X value, and X changes direction (increases, decreases, stays same)
// across the four cases — exercising all 10-bit field boundary combinations.
func TestActorMoved_0x09FD_WireDecode_Porings(t *testing.T) {
	tests := []struct {
		name                   string
		data                   [114]byte
		aid                    uint32
		fromX, fromY, toX, toY uint16
		note                   string
	}{
		{
			"poring1_x_increases",
			dump9Poring1ActorMoved, 110039402,
			162, 342, 165, 348,
			"X increases 162→165",
		},
		{
			"poring2_x_decreases",
			dump9Poring2ActorMoved, 110039396,
			165, 351, 161, 354,
			"X decreases 165→161",
		},
		{
			"poring3_x_large_decrease",
			dump9Poring3ActorMoved, 110039416,
			170, 350, 164, 355,
			"X decreases 170→164",
		},
		{
			"poring4_x_only",
			dump9Poring4ActorMoved, 110039409,
			148, 358, 150, 358,
			"Y stays same, X advances 148→150",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := ActorMoved_0x09FD(tt.data[:], 20181121)

			fromX, fromY, toX, toY, _, _ := packing.DecodeMoveData(e.MoveData[:])

			if e.AID != tt.aid {
				t.Errorf("AID: got %d want %d", e.AID, tt.aid)
			}
			if fromX != tt.fromX {
				t.Errorf("fromX: got %d want %d (%s)", fromX, tt.fromX, tt.note)
			}
			if fromY != tt.fromY {
				t.Errorf("fromY: got %d want %d", fromY, tt.fromY)
			}
			if toX != tt.toX {
				t.Errorf("toX: got %d want %d (%s)", toX, tt.toX, tt.note)
			}
			if toY != tt.toY {
				t.Errorf("toY: got %d want %d", toY, tt.toY)
			}
		})
	}
}

func BenchmarkActorMoved_0x09FD_WireDecode(b *testing.B) {
	data := dump2ActorMoved0x09FD[:]
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e := ActorMoved_0x09FD(data, 20181121)
		_, _, _, _, _, _ = packing.DecodeMoveData(e.MoveData[:])
	}
}

// ─── ActorExists_0x09FF — NPC/monster existence with real wire bytes ──────────
//
// Packet layout (108 bytes at PACKETVER=20181121):
//
//	offset 0:  packetType uint16
//	offset 2:  packetLength uint16
//	offset 4:  objecttype uint8
//	offset 5:  AID uint32
//	offset 9:  GID uint32
//	offset 13: speed uint16
//	offset 63: PosDir [3]byte (WBUFPOS)
//	offset 84: name [24]byte
//
// DUMP8_movement (2026-01-17): NPC "Magician's Guild Guide#" on geffen.gat
// Dump annotation: "NPC Exists: Magician's Guild Guide# (43, 123) (ID 110022517)"
//
//	FF 09 6C 00 06 75 CF 8E 06 00 00 00 00 C8 00 00
//	00 00 00 00 00 00 00 7B 00 00 00 00 00 00 00 00
//	00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
//	00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 0A
//	C7 B6 00 00 00 00 00 00 00 FF FF FF FF FF FF FF
//	FF 00 00 00 4D 61 67 69 63 69 61 6E 27 73 20 47
//	75 69 6C 64 20 47 75 69 64 65 23 00
//
// AID=110022517 speed=200 objecttype=6(NPC) PosDir[63:66]=0A C7 B6 → (43,123,dir=6)
var dump8ActorExists0x09FF = [108]byte{
	0xFF, 0x09, 0x6C, 0x00, 0x06, 0x75, 0xCF, 0x8E,
	0x06, 0x00, 0x00, 0x00, 0x00, 0xC8, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x7B,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0A,
	0xC7, 0xB6, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0x00, 0x00, 0x00, 0x4D, 0x61, 0x67, 0x69,
	0x63, 0x69, 0x61, 0x6E, 0x27, 0x73, 0x20, 0x47,
	0x75, 0x69, 0x6C, 0x64, 0x20, 0x47, 0x75, 0x69,
	0x64, 0x65, 0x23, 0x00,
}

// DUMP9_melee (2026-01-17): Porings on gef_fild07
// Each Poring has a distinct X (162, 165, 170) — exercising the full 10-bit X range.
//
// Poring1 (AID=110039402): PosDir[63:66]=28 95 62 → (162, 342, dir=2)
var dump9Poring1ActorExists = [108]byte{
	0xFF, 0x09, 0x6C, 0x00, 0x05, 0x6A, 0x11, 0x8F,
	0x06, 0x00, 0x00, 0x00, 0x00, 0x90, 0x01, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xEA,
	0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x28,
	0x95, 0x62, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00,
	0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0x00, 0x00, 0x00, 0x50, 0x6F, 0x72, 0x69,
	0x6E, 0x67, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00,
}

// Poring2 (AID=110039396): PosDir[63:66]=29 55 F4 → (165, 351, dir=4)
var dump9Poring2ActorExists = [108]byte{
	0xFF, 0x09, 0x6C, 0x00, 0x05, 0x64, 0x11, 0x8F,
	0x06, 0x00, 0x00, 0x00, 0x00, 0x90, 0x01, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xEA,
	0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x29,
	0x55, 0xF4, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00,
	0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0x00, 0x00, 0x00, 0x50, 0x6F, 0x72, 0x69,
	0x6E, 0x67, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00,
}

// Poring3 (AID=110039416): PosDir[63:66]=2A 95 E2 → (170, 350, dir=2)
var dump9Poring3ActorExists = [108]byte{
	0xFF, 0x09, 0x6C, 0x00, 0x05, 0x78, 0x11, 0x8F,
	0x06, 0x00, 0x00, 0x00, 0x00, 0x90, 0x01, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xEA,
	0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x2A,
	0x95, 0xE2, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00,
	0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0x00, 0x00, 0x00, 0x50, 0x6F, 0x72, 0x69,
	0x6E, 0x67, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00,
}

func TestActorExists_0x09FF_WireDecode_NPC_Position(t *testing.T) {
	e := ActorExists_0x09FF(dump8ActorExists0x09FF[:], 20181121)

	x, y, dir := packing.DecodePosDir(e.PosDir[:])

	// Dump annotation: "NPC Exists: Magician's Guild Guide# (43, 123)"
	if x != 43 {
		t.Errorf("x: got %d want 43", x)
	}
	if y != 123 {
		t.Errorf("y: got %d want 123", y)
	}
	// dir=6 = DIR_EAST (from path.hpp: DIR_EAST=6)
	if dir != 6 {
		t.Errorf("dir: got %d want 6 (DIR_EAST)", dir)
	}
}

func TestActorExists_0x09FF_WireDecode_NPC_MetaFields(t *testing.T) {
	e := ActorExists_0x09FF(dump8ActorExists0x09FF[:], 20181121)

	if e.AID != 110022517 {
		t.Errorf("AID: got %d want 110022517", e.AID)
	}
	if e.Objecttype != 6 {
		t.Errorf("Objecttype: got %d want 6 (NPC)", e.Objecttype)
	}
	if e.Speed != 200 {
		t.Errorf("Speed: got %d want 200", e.Speed)
	}
	if e.Name != "Magician's Guild Guide#" {
		t.Errorf("Name: got %q want %q", e.Name, "Magician's Guild Guide#")
	}
}

// TestActorExists_0x09FF_WireDecode_Porings verifies that three simultaneous
// Poring spawns on gef_fild07 decode to distinct (x,y) positions. X values span
// 162–170, exercising different bit patterns in the 10-bit coordinate field.
func TestActorExists_0x09FF_WireDecode_Porings(t *testing.T) {
	tests := []struct {
		name string
		data [108]byte
		aid  uint32
		x, y uint16
		dir  uint8
	}{
		{"poring1_x162", dump9Poring1ActorExists, 110039402, 162, 342, 2},
		{"poring2_x165", dump9Poring2ActorExists, 110039396, 165, 351, 4},
		{"poring3_x170", dump9Poring3ActorExists, 110039416, 170, 350, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := ActorExists_0x09FF(tt.data[:], 20181121)

			x, y, dir := packing.DecodePosDir(e.PosDir[:])

			if e.AID != tt.aid {
				t.Errorf("AID: got %d want %d", e.AID, tt.aid)
			}
			if x != tt.x {
				t.Errorf("x: got %d want %d", x, tt.x)
			}
			if y != tt.y {
				t.Errorf("y: got %d want %d", y, tt.y)
			}
			if dir != tt.dir {
				t.Errorf("dir: got %d want %d", dir, tt.dir)
			}
		})
	}
}

func BenchmarkActorExists_0x09FF_WireDecode(b *testing.B) {
	data := dump8ActorExists0x09FF[:]
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e := ActorExists_0x09FF(data, 20181121)
		_, _, _ = packing.DecodePosDir(e.PosDir[:])
	}
}

// ─── EntityMove_0x0086 — generic entity move position decoding ────────────────
//
// struct SYNTH_ZC_NOTIFY_MOVE (16 bytes, fixed):
//
//	offset 0, size 2: packetType = 0x0086
//	offset 2, size 4: gid       (uint32 LE)
//	offset 6, size 6: moveData  ([6]byte)
//	offset 12, size 4: moveStartTime (uint32 LE)
//
// Synthesized from rAthena struct layout; uses same WBUFPOS2 encoding.
// GCC-verified struct: SYNTH_ZC_NOTIFY_MOVE total=16 bytes.

func makeEntityMove0x0086(gid uint32, fromX, fromY, toX, toY uint16, moveStartTime uint32) []byte {
	b := make([]byte, 16)
	putI16LE(b, 0, 0x0086)
	putU32LE(b, 2, gid)
	md := packing.EncodeMoveData(fromX, fromY, toX, toY, 0, 0)
	copy(b[6:], md[:])
	putU32LE(b, 12, moveStartTime)
	return b
}

func TestEntityMove_0x0086_WireDecode_Position(t *testing.T) {
	data := makeEntityMove0x0086(99999, 100, 200, 150, 250, 0xDEADBEEF)
	e := EntityMove_0x0086(data, 20181121)

	fromX, fromY, toX, toY, _, _ := packing.DecodeMoveData(e.MoveData[:])

	if fromX != 100 {
		t.Errorf("fromX: got %d want 100", fromX)
	}
	if fromY != 200 {
		t.Errorf("fromY: got %d want 200", fromY)
	}
	if toX != 150 {
		t.Errorf("toX: got %d want 150", toX)
	}
	if toY != 250 {
		t.Errorf("toY: got %d want 250", toY)
	}
	if e.Gid != 99999 {
		t.Errorf("Gid: got %d want 99999", e.Gid)
	}
	if e.MoveStartTime != 0xDEADBEEF {
		t.Errorf("MoveStartTime: got 0x%X want 0xDEADBEEF", e.MoveStartTime)
	}
}

// TestEntityMove_0x0086_WireDecode_MaxCoords verifies maximum 10-bit coordinate (1023).
func TestEntityMove_0x0086_WireDecode_MaxCoords(t *testing.T) {
	data := makeEntityMove0x0086(1, 1023, 1023, 1023, 1023, 0)
	e := EntityMove_0x0086(data, 20181121)

	fromX, fromY, toX, toY, _, _ := packing.DecodeMoveData(e.MoveData[:])

	if fromX != 1023 {
		t.Errorf("fromX: got %d want 1023", fromX)
	}
	if fromY != 1023 {
		t.Errorf("fromY: got %d want 1023", fromY)
	}
	if toX != 1023 {
		t.Errorf("toX: got %d want 1023", toX)
	}
	if toY != 1023 {
		t.Errorf("toY: got %d want 1023", toY)
	}
}

// TestEntityMove_0x0086_WireDecode_ZeroCoords verifies (0,0)→(0,0) is a valid move.
func TestEntityMove_0x0086_WireDecode_ZeroCoords(t *testing.T) {
	data := makeEntityMove0x0086(42, 0, 0, 0, 0, 0)
	e := EntityMove_0x0086(data, 20181121)

	fromX, fromY, toX, toY, _, _ := packing.DecodeMoveData(e.MoveData[:])

	if fromX != 0 || fromY != 0 || toX != 0 || toY != 0 {
		t.Errorf("coords: got (%d,%d)→(%d,%d) want (0,0)→(0,0)", fromX, fromY, toX, toY)
	}
}

func BenchmarkEntityMove_0x0086_WireDecode(b *testing.B) {
	data := makeEntityMove0x0086(99999, 100, 200, 150, 250, 0x12345678)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e := EntityMove_0x0086(data, 20181121)
		_, _, _, _, _, _ = packing.DecodeMoveData(e.MoveData[:])
	}
}
