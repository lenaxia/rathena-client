package encode_test

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/encode"
	"github.com/lenaxia/rathena-client/pkg/send"
)

// TestEncodeRequestBuySellList_GoldenBytes verifies the full 7-byte wire output.
// Wire layout (PACKET_CZ_ACK_SELECT_DEALTYPE, packets.hpp:1511):
//
//	byte[0-1]: 0xC5 0x00 (packet ID 0x00C5 LE)
//	byte[2-5]: GID (uint32 LE)
//	byte[6]:   Type (uint8)
func TestEncodeRequestBuySellList_GoldenBytes(t *testing.T) {
	req := send.RequestBuySellList{GID: 0x00001234, Type: 1}
	p := encode.EncodeRequestBuySellList(req, 20200401)

	if len(p) != 7 {
		t.Fatalf("length: got %d, want 7", len(p))
	}
	if p[0] != 0xc5 || p[1] != 0x00 {
		t.Errorf("packet ID: got [0x%02X, 0x%02X], want [0xC5, 0x00]", p[0], p[1])
	}
	gotGID := binary.LittleEndian.Uint32(p[2:6])
	if gotGID != 0x00001234 {
		t.Errorf("GID: got 0x%08X, want 0x00001234", gotGID)
	}
	if p[6] != 1 {
		t.Errorf("Type (buy flag): got %d, want 1", p[6])
	}
}

// TestEncodeRequestBuySellList_BuyFlag verifies Type=1 for buy dialog.
func TestEncodeRequestBuySellList_BuyFlag(t *testing.T) {
	req := send.RequestBuySellList{GID: 999, Type: 1}
	p := encode.EncodeRequestBuySellList(req, 20200401)
	if p[6] != 1 {
		t.Errorf("buy flag: got %d, want 1", p[6])
	}
}

// TestEncodeRequestBuySellList_SellFlag verifies Type=0 for sell dialog.
func TestEncodeRequestBuySellList_SellFlag(t *testing.T) {
	req := send.RequestBuySellList{GID: 999, Type: 0}
	p := encode.EncodeRequestBuySellList(req, 20200401)
	if p[6] != 0 {
		t.Errorf("sell flag: got %d, want 0", p[6])
	}
}

// TestEncodeRequestBuySellList_GIDZero verifies GID=0 encodes cleanly.
func TestEncodeRequestBuySellList_GIDZero(t *testing.T) {
	req := send.RequestBuySellList{GID: 0, Type: 0}
	p := encode.EncodeRequestBuySellList(req, 20200401)
	gotGID := binary.LittleEndian.Uint32(p[2:6])
	if gotGID != 0 {
		t.Errorf("GID=0: got 0x%08X", gotGID)
	}
}

// TestEncodeRequestBuySellList_GIDMax verifies max uint32 GID.
func TestEncodeRequestBuySellList_GIDMax(t *testing.T) {
	const maxGID uint32 = 0xFFFFFFFF
	req := send.RequestBuySellList{GID: maxGID, Type: 1}
	p := encode.EncodeRequestBuySellList(req, 20200401)
	gotGID := binary.LittleEndian.Uint32(p[2:6])
	if gotGID != maxGID {
		t.Errorf("GID max: got 0x%08X, want 0xFFFFFFFF", gotGID)
	}
}

// TestEncodeRequestBuySellList_PacketverIndependent verifies the output is
// identical across packetvers (this packet has no version-conditional fields).
func TestEncodeRequestBuySellList_PacketverIndependent(t *testing.T) {
	req := send.RequestBuySellList{GID: 12345, Type: 1}
	p1 := encode.EncodeRequestBuySellList(req, 20030000)
	p2 := encode.EncodeRequestBuySellList(req, 20200401)
	if p1 != p2 {
		t.Errorf("output differs across packetvers: %v vs %v", p1, p2)
	}
}

func BenchmarkEncodeRequestBuySellList(b *testing.B) {
	req := send.RequestBuySellList{GID: 12345, Type: 1}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodeRequestBuySellList(req, 20200401)
	}
}
