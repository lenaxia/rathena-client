// Package decode — golden tests for SellResult_0x00CB.
//
// ZC_PC_SELL_RESULT (3 bytes, fixed):
//
//	offset 0, size 2: PacketType = 0x00CB (LE: 0xCB 0x00)
//	offset 2, size 1: result uint8 (0=success, 1=fail)
//
// rAthena source: clif.cpp clif_npc_sell_result — raw WFIFO macros:
//
//	WFIFOW(fd,0) = 0xcb;
//	WFIFOB(fd,2) = result;
//
// No struct definition — synthesized as SYNTH_ZC_PC_SELL_RESULT.
package decode

import "testing"

// TestSellResult_0x00CB_Success verifies result=0 (success).
func TestSellResult_0x00CB_Success(t *testing.T) {
	data := []byte{0xCB, 0x00, 0x00}
	e := SellResult_0x00CB(data, 20180307)

	if e.Result != 0 {
		t.Errorf("Result: got %d want 0", e.Result)
	}
}

// TestSellResult_0x00CB_Fail verifies result=1 (fail).
func TestSellResult_0x00CB_Fail(t *testing.T) {
	data := []byte{0xCB, 0x00, 0x01}
	e := SellResult_0x00CB(data, 20180307)

	if e.Result != 1 {
		t.Errorf("Result: got %d want 1", e.Result)
	}
}

// BenchmarkSellResult_0x00CB verifies 0 allocs/op on the decode hot path.
// SellResult has no string fields — expect 0 allocs/op.
func BenchmarkSellResult_0x00CB(b *testing.B) {
	data := []byte{0xCB, 0x00, 0x01}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = SellResult_0x00CB(data, 20180307)
	}
}
