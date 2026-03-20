package encode_test

import (
	"testing"

	"github.com/lenaxia/rathena-client/pkg/encode"
	"github.com/lenaxia/rathena-client/pkg/send"
)

// TestEncodeShopSell_SellListWritten verifies that sell list bytes appear in output.
// This test currently FAILS because EncodeShopSell returns [4]byte.
func TestEncodeShopSell_SellListWritten(t *testing.T) {
	sellList := []byte{0x02, 0x00, 0x01, 0x00} // index=2 LE, amount=1 LE
	req := send.ShopSell{SellList: sellList}
	p := encode.EncodeShopSell(req, 20200401)
	if len(p) < 8 {
		t.Fatalf("output too short: got %d bytes, want >= 8", len(p))
	}
	for i, b := range sellList {
		if p[4+i] != b {
			t.Errorf("byte[%d]: got 0x%02X, want 0x%02X (sell list not written)", 4+i, p[4+i], b)
		}
	}
}

// TestEncodeShopSell_PacketID verifies the wire packet ID bytes.
func TestEncodeShopSell_PacketID(t *testing.T) {
	req := send.ShopSell{SellList: []byte{0x01, 0x00, 0x01, 0x00}}
	p := encode.EncodeShopSell(req, 20200401)
	if p[0] != 0xc9 || p[1] != 0x00 {
		t.Errorf("packet ID: got [0x%02X, 0x%02X], want [0xC9, 0x00]", p[0], p[1])
	}
}

// TestEncodeShopSell_TotalLength verifies the total output length is 4 + len(sellList).
func TestEncodeShopSell_TotalLength(t *testing.T) {
	sellList := []byte{0x01, 0x00, 0x02, 0x00, 0x03, 0x00, 0x01, 0x00} // 2 entries
	req := send.ShopSell{SellList: sellList}
	p := encode.EncodeShopSell(req, 20200401)
	want := 4 + len(sellList)
	if len(p) != want {
		t.Errorf("length: got %d, want %d", len(p), want)
	}
}

// TestEncodeShopSell_LengthFieldComputed verifies that bytes 2-3 contain the total length.
func TestEncodeShopSell_LengthFieldComputed(t *testing.T) {
	sellList := []byte{0x01, 0x00, 0x02, 0x00} // 4 bytes → total = 8
	req := send.ShopSell{SellList: sellList}
	p := encode.EncodeShopSell(req, 20200401)
	gotLen := int(p[2]) | int(p[3])<<8
	wantLen := 4 + len(sellList)
	if gotLen != wantLen {
		t.Errorf("length field: got %d, want %d", gotLen, wantLen)
	}
}

// TestEncodeShopSell_EmptySellList verifies that zero entries produces a 4-byte packet.
func TestEncodeShopSell_EmptySellList(t *testing.T) {
	req := send.ShopSell{SellList: nil}
	p := encode.EncodeShopSell(req, 20200401)
	if len(p) != 4 {
		t.Errorf("empty sell list: got %d bytes, want 4", len(p))
	}
}

// TestEncodeShopSell_MultipleEntries verifies two entries are written correctly.
func TestEncodeShopSell_MultipleEntries(t *testing.T) {
	// 2 entries: PACKET_CZ_PC_SELL_ITEMLIST_sub = index(uint16) + amount(uint16)
	sellList := []byte{
		0x05, 0x00, 0x01, 0x00, // index=5, amount=1
		0x0A, 0x00, 0x03, 0x00, // index=10, amount=3
	}
	req := send.ShopSell{SellList: sellList}
	p := encode.EncodeShopSell(req, 20200401)
	if len(p) != 12 {
		t.Fatalf("length: got %d, want 12", len(p))
	}
	for i, b := range sellList {
		if p[4+i] != b {
			t.Errorf("sell byte[%d]: got 0x%02X, want 0x%02X", i, p[4+i], b)
		}
	}
}

func BenchmarkEncodeShopSell(b *testing.B) {
	sellList := make([]byte, 16) // 4 entries
	req := send.ShopSell{SellList: sellList}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodeShopSell(req, 20200401)
	}
}
