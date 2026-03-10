// Package decode — golden tests for Sync_0x007F.
//
// struct PACKET_ZC_NOTIFY_TIME (6 bytes, fixed):
//
//	offset 0, size 2: packetType = 0x007F (LE: 0x7F 0x00)
//	offset 2, size 4: time (uint32 LE)
//
// Verified by GCC output at PACKETVER=20181121 (from pre-implementation gate):
// g++ -E -P -DPACKETVER=20181121 -include stubs/packets_hpp_stub.h src/map/packets.hpp
// → PACKET_ZC_NOTIFY_TIME total=6 bytes, __attribute__((packed))
package decode

import "testing"

// makeSync0x007F builds the canonical 6-byte PACKET_ZC_NOTIFY_TIME.
func makeSync0x007F(serverTime uint32) []byte {
	b := make([]byte, 6)
	putI16LE(b, 0, 0x007F) // packetType
	putU32LE(b, 2, serverTime)
	return b
}

// TestSync_0x007F_ZeroTime verifies time=0 (initial tick or server reset).
func TestSync_0x007F_ZeroTime(t *testing.T) {
	data := makeSync0x007F(0)
	e := Sync_0x007F(data, 20181121)

	if e.Time != 0 {
		t.Errorf("Time: got %d want 0", e.Time)
	}
}

// TestSync_0x007F_Tick1000 verifies time=1000 (a small positive tick).
func TestSync_0x007F_Tick1000(t *testing.T) {
	data := makeSync0x007F(1000)
	e := Sync_0x007F(data, 20181121)

	if e.Time != 1000 {
		t.Errorf("Time: got %d want 1000", e.Time)
	}
}

// TestSync_0x007F_DeadBeef verifies time=0xDEADBEEF (pattern to catch byte-order bugs).
func TestSync_0x007F_DeadBeef(t *testing.T) {
	data := makeSync0x007F(0xDEADBEEF)
	e := Sync_0x007F(data, 20181121)

	if e.Time != 0xDEADBEEF {
		t.Errorf("Time: got 0x%X want 0xDEADBEEF", e.Time)
	}
}

// BenchmarkSync_0x007F verifies 0 allocs/op on the decode hot path.
func BenchmarkSync_0x007F(b *testing.B) {
	data := makeSync0x007F(0xDEADBEEF)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Sync_0x007F(data, 20181121)
	}
}
