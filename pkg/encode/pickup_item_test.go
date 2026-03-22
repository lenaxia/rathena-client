// Manually implemented — regression test for worklog 0070.
// EncodePickupItem must emit the shuffled/reassigned wire ID for pv >= 20101124.

package encode_test

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/encode"
	"github.com/lenaxia/rathena-client/pkg/send"
)

// packetID returns the little-endian uint16 packet ID from the first two bytes.
func pickupPacketID(p [6]byte) uint16 {
	return binary.LittleEndian.Uint16(p[0:2])
}

// TestEncodePickupItem_PacketID_Table covers every explicit clif_packetdb.hpp
// boundary from pv=20101124 onward, plus the shuffle era and baseline.
// Each case is verified against rAthena clif_packetdb.hpp and OpenKore modules
// (see worklog 0070 cross-validation).
func TestEncodePickupItem_PacketID_Table(t *testing.T) {
	req := send.PickupItem{ITID: 0xDEADBEEF}

	cases := []struct {
		name    string
		pv      uint32
		wantID  uint16
		wantSrc string
	}{
		// Baseline — correct for pv < 20101124
		// clif_packetdb.hpp line 50: parseable_packet(0x009f,6,clif_parse_TakeItem,2)
		{"baseline pv=20030000", 20030000, 0x009F, "clif_packetdb.hpp:50"},
		{"pre-boundary pv=20101123", 20101123, 0x009F, "clif_packetdb.hpp:50"},

		// pv >= 20101124: 0x0362
		// clif_packetdb.hpp:1384: parseable_packet(0x0362,6,clif_parse_TakeItem,2)
		// OpenKore: RagexeRE_2010_11_24a.pm item_take 0362 ✓
		{"boundary pv=20101124", 20101124, 0x0362, "clif_packetdb.hpp:1384"},
		{"pv=20111004", 20111004, 0x0362, "clif_packetdb.hpp:1384"},

		// pv >= 20111005: 0x0815
		// clif_packetdb.hpp:1402: parseable_packet(0x0815,6,clif_parse_TakeItem,2)
		// OpenKore: RagexeRE_2011_11_02a.pm item_take 0815 ✓ (rAthena authoritative)
		{"boundary pv=20111005", 20111005, 0x0815, "clif_packetdb.hpp:1402"},
		{"pv=20120306", 20120306, 0x0815, "clif_packetdb.hpp:1402"},

		// pv >= 20120307: 0x0865
		// clif_packetdb.hpp:1441: parseable_packet(0x0865,6,clif_parse_TakeItem,2)
		// OpenKore: RagexeRE_2012_03_07f.pm item_take 0865 ✓
		{"boundary pv=20120307", 20120307, 0x0865, "clif_packetdb.hpp:1441"},
		{"pv=20120409", 20120409, 0x0865, "clif_packetdb.hpp:1441"},

		// pv >= 20120410: 0x0938
		// clif_packetdb.hpp:1494: parseable_packet(0x0938,6,clif_parse_TakeItem,2)
		// OpenKore: RagexeRE_2012_04_10a.pm item_take 0938 ✓
		{"boundary pv=20120410", 20120410, 0x0938, "clif_packetdb.hpp:1494"},
		{"pv=20120417", 20120417, 0x0938, "clif_packetdb.hpp:1494"},

		// pv >= 20120418: 0x07E4
		// clif_packetdb.hpp:1560: parseable_packet(0x07E4,6,clif_parse_TakeItem,2)
		// OpenKore: RagexeRE_2012_04_18a.pm item_take 07E4 ✓
		{"boundary pv=20120418", 20120418, 0x07E4, "clif_packetdb.hpp:1560"},
		{"pv=20120701", 20120701, 0x07E4, "clif_packetdb.hpp:1560"},

		// pv >= 20120702: 0x089F
		// clif_packetdb.hpp:1587: parseable_packet(0x089f,6,clif_parse_TakeItem,2)
		{"boundary pv=20120702", 20120702, 0x089F, "clif_packetdb.hpp:1587"},
		{"pv=20130319", 20130319, 0x089F, "clif_packetdb.hpp:1587"},

		// pv >= 20130320: 0x0933
		// clif_packetdb.hpp:1631: parseable_packet(0x0933,6,clif_parse_TakeItem,2)
		// OpenKore: RagexeRE_2013_03_20.pm item_take 0933 ✓
		{"boundary pv=20130320", 20130320, 0x0933, "clif_packetdb.hpp:1631"},
		{"pv=20130514", 20130514, 0x0933, "clif_packetdb.hpp:1631"},

		// pv >= 20130515: shuffle era — shuffledCtoSID(pv, 0x009F)
		// OpenKore: RagexeRE_2013_05_15a.pm item_take 08A1 ✓
		{"shuffle boundary pv=20130515", 20130515, 0x08A1, "clif_shuffle.hpp pv=20130515"},
		// OpenKore: RagexeRE_2013_05_22.pm item_take 095E ✓
		{"shuffle pv=20130522", 20130522, 0x095E, "clif_shuffle.hpp pv=20130522"},

		// stable post-20180307: shuffledCtoSID always returns 0x0362
		// OpenKore: RagexeRE_2018_11_21.pm item_take 0362 ✓
		// Production: pv=20200401
		{"post-shuffle pv=20200401", 20200401, 0x0362, "clif_shuffle.hpp pv>20180307"},
		{"post-shuffle pv=20180308", 20180308, 0x0362, "clif_shuffle.hpp pv>20180307"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := encode.EncodePickupItem(req, tc.pv)
			got := pickupPacketID(p)
			if got != tc.wantID {
				t.Errorf("pv=%d: packet ID = 0x%04X, want 0x%04X (source: %s)",
					tc.pv, got, tc.wantID, tc.wantSrc)
			}
		})
	}
}

// TestEncodePickupItem_Length verifies the wire packet is always 6 bytes.
// All post-20101124 TakeItem variants are 6 bytes with ITID at offset 2.
// Source: clif_packetdb.hpp lines 1384, 1402, 1441, 1494, 1560, 1587, 1631.
func TestEncodePickupItem_Length(t *testing.T) {
	for _, pv := range []uint32{20030000, 20101124, 20200401} {
		p := encode.EncodePickupItem(send.PickupItem{ITID: 1}, pv)
		if len(p) != 6 {
			t.Errorf("pv=%d: len=%d, want 6", pv, len(p))
		}
	}
}

// TestEncodePickupItem_ITID verifies ITID is encoded at bytes [2:6] little-endian.
// Source: clif_packetdb.hpp parseable_packet(..., clif_parse_TakeItem, 2) — field at offset 2.
func TestEncodePickupItem_ITID(t *testing.T) {
	const itid = uint32(0xCAFEBABE)
	p := encode.EncodePickupItem(send.PickupItem{ITID: itid}, 20200401)
	got := binary.LittleEndian.Uint32(p[2:6])
	if got != itid {
		t.Fatalf("ITID: got 0x%08X at bytes[2:6], want 0xCAFEBABE", got)
	}
}

// TestEncodePickupItem_ZeroITID verifies zero ITID encodes correctly.
func TestEncodePickupItem_ZeroITID(t *testing.T) {
	p := encode.EncodePickupItem(send.PickupItem{ITID: 0}, 20200401)
	id := pickupPacketID(p)
	if id != 0x0362 {
		t.Errorf("zero ITID: packet ID = 0x%04X, want 0x0362", id)
	}
	for i := 2; i < 6; i++ {
		if p[i] != 0 {
			t.Errorf("zero ITID: byte[%d] = 0x%02X, want 0x00", i, p[i])
		}
	}
}

// TestEncodePickupItem_ITIDPreservedAcrossPacketvers verifies ITID encoding
// is identical regardless of which wire packet ID is used.
func TestEncodePickupItem_ITIDPreservedAcrossPacketvers(t *testing.T) {
	const itid = uint32(0x0001FFFF)
	want := itid
	for _, pv := range []uint32{20030000, 20101124, 20130515, 20200401} {
		p := encode.EncodePickupItem(send.PickupItem{ITID: itid}, pv)
		got := binary.LittleEndian.Uint32(p[2:6])
		if got != want {
			t.Errorf("pv=%d: ITID got 0x%08X, want 0x%08X", pv, got, want)
		}
	}
}

// BenchmarkEncodePickupItem verifies 0 allocs/op — required by README-LLM.md §Performance.
func BenchmarkEncodePickupItem(b *testing.B) {
	req := send.PickupItem{ITID: 0xDEADBEEF}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodePickupItem(req, 20200401)
	}
}
