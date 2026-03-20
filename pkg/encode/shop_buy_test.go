package encode_test

import (
	"testing"

	"github.com/lenaxia/rathena-client/pkg/encode"
	"github.com/lenaxia/rathena-client/pkg/send"
)

// TestEncodeShopBuy_ItemsWritten verifies that item bytes are present in the output.
// This test currently FAILS because EncodeShopBuy returns [4]byte and the copy into
// p[4:] is a no-op. After the fix, it must return []byte with items at offset 4.
func TestEncodeShopBuy_ItemsWritten(t *testing.T) {
	items := []byte{0x01, 0x00, 0x03, 0x00} // amount=1 LE, itemId=3 LE
	req := send.ShopBuy{Items: items}
	p := encode.EncodeShopBuy(req, 20200401)
	if len(p) < 8 {
		t.Fatalf("output too short: got %d bytes, want >= 8", len(p))
	}
	for i, b := range items {
		if p[4+i] != b {
			t.Errorf("byte[%d]: got 0x%02X, want 0x%02X (items not written)", 4+i, p[4+i], b)
		}
	}
}

// TestEncodeShopBuy_PacketID verifies the wire packet ID bytes.
func TestEncodeShopBuy_PacketID(t *testing.T) {
	req := send.ShopBuy{Items: []byte{0x01, 0x00, 0x03, 0x00}}
	p := encode.EncodeShopBuy(req, 20200401)
	if p[0] != 0xc8 || p[1] != 0x00 {
		t.Errorf("packet ID: got [0x%02X, 0x%02X], want [0xC8, 0x00]", p[0], p[1])
	}
}

// TestEncodeShopBuy_TotalLength verifies the total output length is 4 + len(items).
func TestEncodeShopBuy_TotalLength(t *testing.T) {
	items := []byte{0x01, 0x00, 0x03, 0x00, 0x02, 0x00, 0x05, 0x00} // 2 items
	req := send.ShopBuy{Items: items}
	p := encode.EncodeShopBuy(req, 20200401)
	want := 4 + len(items)
	if len(p) != want {
		t.Errorf("length: got %d, want %d", len(p), want)
	}
}

// TestEncodeShopBuy_LengthFieldComputed verifies that bytes 2-3 contain the total length.
func TestEncodeShopBuy_LengthFieldComputed(t *testing.T) {
	items := []byte{0x01, 0x00, 0x03, 0x00} // 4 bytes → total = 8
	req := send.ShopBuy{Items: items}
	p := encode.EncodeShopBuy(req, 20200401)
	gotLen := int(p[2]) | int(p[3])<<8
	wantLen := 4 + len(items)
	if gotLen != wantLen {
		t.Errorf("length field: got %d, want %d", gotLen, wantLen)
	}
}

// TestEncodeShopBuy_EmptyItems verifies that zero items produces a 4-byte packet.
func TestEncodeShopBuy_EmptyItems(t *testing.T) {
	req := send.ShopBuy{Items: nil}
	p := encode.EncodeShopBuy(req, 20200401)
	if len(p) != 4 {
		t.Errorf("empty items: got %d bytes, want 4", len(p))
	}
}

// TestEncodeShopBuy_MultipleItems verifies three items are all written correctly.
func TestEncodeShopBuy_MultipleItems(t *testing.T) {
	// 3 items × 4 bytes each (pre-20181121 item size)
	items := []byte{
		0x01, 0x00, 0x01, 0x00, // amount=1, itemId=1
		0x03, 0x00, 0x02, 0x00, // amount=3, itemId=2
		0x01, 0x00, 0xFF, 0x00, // amount=1, itemId=255
	}
	req := send.ShopBuy{Items: items}
	p := encode.EncodeShopBuy(req, 20180307)
	if len(p) != 16 {
		t.Fatalf("length: got %d, want 16", len(p))
	}
	for i, b := range items {
		if p[4+i] != b {
			t.Errorf("item byte[%d]: got 0x%02X, want 0x%02X", i, p[4+i], b)
		}
	}
}

func BenchmarkEncodeShopBuy(b *testing.B) {
	items := make([]byte, 20) // 5 items
	req := send.ShopBuy{Items: items}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodeShopBuy(req, 20200401)
	}
}
