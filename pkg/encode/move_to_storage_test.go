// Manually implemented — regression tests for worklog 0073.
// move_to_storage encoder must use shuffled wire ID, not hardcoded 0x00F3.

package encode_test

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/encode"
	"github.com/lenaxia/rathena-client/pkg/send"
)

func mtsPacketID(p [8]byte) uint16 {
	return binary.LittleEndian.Uint16(p[0:2])
}

// TestEncodeMoveToStorage_PacketID_Table covers every explicit boundary
// from pv=20101124 onward plus shuffle era.
// Cross-validated: rAthena clif_shuffle.hpp:4736, OpenKore storage_item_add 0364.
func TestEncodeMoveToStorage_PacketID_Table(t *testing.T) {
	req := send.MoveToStorage{Index: 5, Amount: 100}

	cases := []struct {
		name   string
		pv     uint32
		wantID uint16
	}{
		{"baseline pv=20030000", 20030000, 0x00F3},
		{"pre-boundary pv=20101123", 20101123, 0x00F3},

		// pv >= 20101124: 0x0364 — clif_packetdb.hpp:1386
		// OpenKore: RagexeRE_2010_11_24a.pm storage_item_add 0364 ✓
		{"boundary pv=20101124", 20101124, 0x0364},
		{"pv=20111004", 20111004, 0x0364},

		{"boundary pv=20111005", 20111005, 0x0893},
		{"boundary pv=20120307", 20120307, 0x093B},
		{"boundary pv=20120410", 20120410, 0x086C},
		{"boundary pv=20120418", 20120418, 0x07EC},
		{"boundary pv=20120702", 20120702, 0x08A0},
		{"boundary pv=20130320", 20130320, 0x08AC},
		{"pv=20130514", 20130514, 0x08AC},

		// shuffle era — clif_shuffle.hpp pv=20130515: 0x0887
		{"shuffle boundary pv=20130515", 20130515, 0x0887},

		// post-shuffle stable: 0x0364 — clif_shuffle.hpp:4736
		{"post-shuffle pv=20200401", 20200401, 0x0364},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := encode.EncodeMoveToStorage(req, tc.pv)
			got := mtsPacketID(p)
			if got != tc.wantID {
				t.Errorf("pv=%d: packet ID = 0x%04X, want 0x%04X", tc.pv, got, tc.wantID)
			}
		})
	}
}

func TestEncodeMoveToStorage_Length(t *testing.T) {
	p := encode.EncodeMoveToStorage(send.MoveToStorage{}, 20200401)
	if len(p) != 8 {
		t.Fatalf("length: got %d, want 8", len(p))
	}
}

func TestEncodeMoveToStorage_Index(t *testing.T) {
	p := encode.EncodeMoveToStorage(send.MoveToStorage{Index: 0xBEEF, Amount: 0}, 20200401)
	got := binary.LittleEndian.Uint16(p[2:4])
	if got != 0xBEEF {
		t.Fatalf("Index: got 0x%04X at [2:4], want 0xBEEF", got)
	}
}

func TestEncodeMoveToStorage_Amount(t *testing.T) {
	p := encode.EncodeMoveToStorage(send.MoveToStorage{Index: 0, Amount: 0xDEADBEEF}, 20200401)
	got := binary.LittleEndian.Uint32(p[4:8])
	if got != 0xDEADBEEF {
		t.Fatalf("Amount: got 0x%08X at [4:8], want 0xDEADBEEF", got)
	}
}

func BenchmarkEncodeMoveToStorage(b *testing.B) {
	req := send.MoveToStorage{Index: 5, Amount: 100}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodeMoveToStorage(req, 20200401)
	}
}
