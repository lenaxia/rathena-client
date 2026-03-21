// Manually implemented — regression test for worklog 0069.
// EncodeItemUse must send 0x0439 at pv >= 20080910, not 0x00A7.

package encode_test

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/encode"
	"github.com/lenaxia/rathena-client/pkg/send"
)

func TestEncodeItemUse_PacketID_Modern(t *testing.T) {
	// pv >= 20080910: must use 0x0439 (CZ_USE_ITEM2)
	// Before worklog 0069 fix this sent 0x00A7, causing the server to
	// disconnect with "received packet 0x00a7 with expected length 9, only 8 bytes".
	p := encode.EncodeItemUse(send.ItemUse{Index: 3, AID: 2000001}, 20200401)
	if p[0] != 0x39 || p[1] != 0x04 {
		t.Fatalf("pv 20200401: packet ID %02X%02X, want 3904", p[1], p[0])
	}
}

func TestEncodeItemUse_PacketID_Boundary(t *testing.T) {
	// Exactly at the boundary pv == 20080910 must also use 0x0439.
	p := encode.EncodeItemUse(send.ItemUse{Index: 1, AID: 1}, 20080910)
	if p[0] != 0x39 || p[1] != 0x04 {
		t.Fatalf("pv 20080910: packet ID %02X%02X, want 3904", p[1], p[0])
	}
}

func TestEncodeItemUse_PacketID_Legacy(t *testing.T) {
	// pv < 20080910: legacy 0x00A7 (CZ_USE_ITEM)
	p := encode.EncodeItemUse(send.ItemUse{Index: 3, AID: 2000001}, 20040705)
	if p[0] != 0xa7 || p[1] != 0x00 {
		t.Fatalf("pv 20040705: packet ID %02X%02X, want a700", p[1], p[0])
	}
}

func TestEncodeItemUse_Length(t *testing.T) {
	// Both variants are 8 bytes: int16 + uint16 + uint32
	p := encode.EncodeItemUse(send.ItemUse{}, 20200401)
	if len(p) != 8 {
		t.Fatalf("length: got %d, want 8", len(p))
	}
}

func TestEncodeItemUse_Index(t *testing.T) {
	// Index field at bytes [2:4] (little-endian uint16)
	p := encode.EncodeItemUse(send.ItemUse{Index: 0xBEEF, AID: 0}, 20200401)
	got := binary.LittleEndian.Uint16(p[2:4])
	if got != 0xBEEF {
		t.Fatalf("Index: got %04X at bytes[2:4], want BEEF", got)
	}
}

func TestEncodeItemUse_AID(t *testing.T) {
	// AID field at bytes [4:8] (little-endian uint32)
	p := encode.EncodeItemUse(send.ItemUse{Index: 0, AID: 0xDEADBEEF}, 20200401)
	got := binary.LittleEndian.Uint32(p[4:8])
	if got != 0xDEADBEEF {
		t.Fatalf("AID: got %08X at bytes[4:8], want DEADBEEF", got)
	}
}

func BenchmarkEncodeItemUse(b *testing.B) {
	req := send.ItemUse{Index: 3, AID: 2000001}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodeItemUse(req, 20200401)
	}
}
