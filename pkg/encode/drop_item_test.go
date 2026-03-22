// Manually implemented — regression tests for worklog 0073.
// drop_item encoder must use shuffled wire ID, not hardcoded 0x00A2.

package encode_test

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/encode"
	"github.com/lenaxia/rathena-client/pkg/send"
)

func dropPacketID(p [6]byte) uint16 {
	return binary.LittleEndian.Uint16(p[0:2])
}

// TestEncodeDropItem_PacketID_Table covers every explicit clif_packetdb.hpp
// boundary from pv=20101124 onward plus shuffle era and stable block.
// Cross-validated: rAthena clif_shuffle.hpp:4729, OpenKore item_drop 0363.
func TestEncodeDropItem_PacketID_Table(t *testing.T) {
	req := send.DropItem{Index: 3, Amount: 10}

	cases := []struct {
		name   string
		pv     uint32
		wantID uint16
	}{
		// baseline (before modern era) — 0x00A2
		{"baseline pv=20030000", 20030000, 0x00A2},
		{"pre-boundary pv=20101123", 20101123, 0x00A2},

		// pv >= 20101124: 0x0363 — clif_packetdb.hpp:1385
		// OpenKore: RagexeRE_2010_11_24a.pm item_drop 0363 ✓
		{"boundary pv=20101124", 20101124, 0x0363},
		{"pv=20111004", 20111004, 0x0363},

		// pv >= 20111005: 0x0885 — clif_packetdb.hpp:1403
		{"boundary pv=20111005", 20111005, 0x0885},
		{"pv=20120306", 20120306, 0x0885},

		// pv >= 20120307: 0x02C4 — clif_packetdb.hpp:1442
		{"boundary pv=20120307", 20120307, 0x02C4},
		{"pv=20120417", 20120417, 0x02C4},

		// pv >= 20120418: 0x0362 — clif_packetdb.hpp:1561
		{"boundary pv=20120418", 20120418, 0x0362},
		{"pv=20120701", 20120701, 0x0362},

		// pv >= 20120702: 0x089E — clif_packetdb.hpp:1586
		{"boundary pv=20120702", 20120702, 0x089E},
		{"pv=20130319", 20130319, 0x089E},

		// pv >= 20130320: 0x0438 — clif_packetdb.hpp:1606
		{"boundary pv=20130320", 20130320, 0x0438},
		{"pv=20130514", 20130514, 0x0438},

		// pv >= 20130515: shuffle era — shuffledCtoSID(pv, 0x00A2)
		// clif_shuffle.hpp pv=20130515: 0x0944
		{"shuffle boundary pv=20130515", 20130515, 0x0944},

		// pv > 20180307: stable 0x0363 — clif_shuffle.hpp:4729, OpenKore ✓
		{"post-shuffle pv=20200401", 20200401, 0x0363},
		{"post-shuffle pv=20180308", 20180308, 0x0363},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := encode.EncodeDropItem(req, tc.pv)
			got := dropPacketID(p)
			if got != tc.wantID {
				t.Errorf("pv=%d: packet ID = 0x%04X, want 0x%04X", tc.pv, got, tc.wantID)
			}
		})
	}
}

func TestEncodeDropItem_Length(t *testing.T) {
	p := encode.EncodeDropItem(send.DropItem{}, 20200401)
	if len(p) != 6 {
		t.Fatalf("length: got %d, want 6", len(p))
	}
}

// TestEncodeDropItem_Index verifies Index at bytes [2:4] (little-endian uint16).
// Source: clif_packetdb.hpp parseable_packet(..., clif_parse_DropItem, 2, 4) — index@2
func TestEncodeDropItem_Index(t *testing.T) {
	p := encode.EncodeDropItem(send.DropItem{Index: 0xBEEF, Amount: 0}, 20200401)
	got := binary.LittleEndian.Uint16(p[2:4])
	if got != 0xBEEF {
		t.Fatalf("Index: got 0x%04X at [2:4], want 0xBEEF", got)
	}
}

// TestEncodeDropItem_Amount verifies Amount at bytes [4:6] (little-endian uint16).
// Source: clif_packetdb.hpp — amount@4
func TestEncodeDropItem_Amount(t *testing.T) {
	p := encode.EncodeDropItem(send.DropItem{Index: 0, Amount: 0xCAFE}, 20200401)
	got := binary.LittleEndian.Uint16(p[4:6])
	if got != 0xCAFE {
		t.Fatalf("Amount: got 0x%04X at [4:6], want 0xCAFE", got)
	}
}

func BenchmarkEncodeDropItem(b *testing.B) {
	req := send.DropItem{Index: 3, Amount: 10}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodeDropItem(req, 20200401)
	}
}
