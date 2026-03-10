// Package decode — golden tests for CharacterMoves_0x0087.
//
// struct PACKET_ZC_NOTIFY_PLAYERMOVE (12 bytes, fixed):
//
//	offset 0, size 2: packetType = 0x0087 (LE: 0x87 0x00)
//	offset 2, size 4: moveStartTime (uint32 LE)
//	offset 6, size 6: moveData[6]   (raw bytes)
//
// Verified by GCC output at PACKETVER=20181121 (from pre-implementation gate):
// g++ -E -P -DPACKETVER=20181121 -include stubs/packets_hpp_stub.h src/map/packets.hpp
// → PACKET_ZC_NOTIFY_PLAYERMOVE total=12 bytes, __attribute__((packed))
package decode

import "testing"

// makeCharacterMoves0x0087 builds the canonical 12-byte PACKET_ZC_NOTIFY_PLAYERMOVE.
func makeCharacterMoves0x0087(moveStartTime uint32, moveData [6]byte) []byte {
	b := make([]byte, 12)
	putI16LE(b, 0, 0x0087) // packetType
	putU32LE(b, 2, moveStartTime)
	copy(b[6:], moveData[:])
	return b
}

// TestCharacterMoves_0x0087_Standard verifies a standard movement packet.
func TestCharacterMoves_0x0087_Standard(t *testing.T) {
	moveData := [6]byte{0x19, 0x03, 0x28, 0x96, 0xFA, 0x00}
	data := makeCharacterMoves0x0087(0x12345678, moveData)
	e := CharacterMoves_0x0087(data, 20181121)

	if e.Time != 0x12345678 {
		t.Errorf("Time: got 0x%X want 0x12345678", e.Time)
	}
	if e.MoveData != moveData {
		t.Errorf("MoveData: got %v want %v", e.MoveData, moveData)
	}
}

// TestCharacterMoves_0x0087_ZeroTime verifies moveStartTime=0 and all-zero moveData.
func TestCharacterMoves_0x0087_ZeroTime(t *testing.T) {
	moveData := [6]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	data := makeCharacterMoves0x0087(0, moveData)
	e := CharacterMoves_0x0087(data, 20181121)

	if e.Time != 0 {
		t.Errorf("Time: got %d want 0", e.Time)
	}
	if e.MoveData != moveData {
		t.Errorf("MoveData: got %v want %v", e.MoveData, moveData)
	}
}

// TestCharacterMoves_0x0087_MaxTime verifies moveStartTime=0xFFFFFFFF (max uint32).
func TestCharacterMoves_0x0087_MaxTime(t *testing.T) {
	moveData := [6]byte{0xAB, 0xCD, 0xEF, 0x12, 0x34, 0x56}
	data := makeCharacterMoves0x0087(0xFFFFFFFF, moveData)
	e := CharacterMoves_0x0087(data, 20181121)

	if e.Time != 0xFFFFFFFF {
		t.Errorf("Time: got 0x%X want 0xFFFFFFFF", e.Time)
	}
	if e.MoveData != moveData {
		t.Errorf("MoveData: got %v want %v", e.MoveData, moveData)
	}
}

// BenchmarkCharacterMoves_0x0087 verifies 0 allocs/op on the decode hot path.
func BenchmarkCharacterMoves_0x0087(b *testing.B) {
	data := makeCharacterMoves0x0087(0x12345678, [6]byte{0x19, 0x03, 0x28, 0x96, 0xFA, 0x00})
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = CharacterMoves_0x0087(data, 20181121)
	}
}
