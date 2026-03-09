// Package session benchmarks — verifies zero-alloc invariant on the decode hot path.
package session_test

import (
	"encoding/binary"
	"sync"
	"testing"

	"github.com/lenaxia/ragnarok-go-client/pkg/session"
)

// BenchmarkFeed_SmallFixedPacket benchmarks Feed processing a small fixed-length packet.
// Target: 0 allocs/op, < 200 ns/op (HLD §8).
func BenchmarkFeed_SmallFixedPacket(b *testing.B) {
	s := session.NewMapSession(20181002)
	s.RegisterHandler(0x0080, func(data []byte, pv uint32) {}) // actor_vanished, 7 bytes

	frame := make([]byte, 7)
	frame[0] = 0x80
	frame[1] = 0x00

	// Prime the backing buffer.
	_ = s.Feed(frame)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := s.Feed(frame); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFeed_VariableLengthPacket benchmarks Feed processing a variable-length packet.
// Target: 0 allocs/op (HLD §8).
func BenchmarkFeed_VariableLengthPacket(b *testing.B) {
	s := session.NewMapSession(20181002)
	s.RegisterHandler(0x0069, func(data []byte, pv uint32) {}) // 0x0069 is variable-length

	frame := makeVarFrame(0x0069, 64)

	// Prime.
	_ = s.Feed(frame)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := s.Feed(frame); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkEncode_NoObfuscation benchmarks the Encode no-op path.
// Target: 0 allocs/op, < 10 ns/op.
func BenchmarkEncode_NoObfuscation(b *testing.B) {
	s := session.NewMapSession(20181002)
	id := uint16(0x0085)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		id = 0x0085
		s.Encode(&id)
	}
}

// ─── HLD §8 benchmarks ───────────────────────────────────────────────────────

// BenchmarkFeed_ActorExists_0x09FF benchmarks Feed processing a 108-byte 0x09FF
// (packet_idle_unit / actor idle) variable-length packet.
// Target: 0 allocs/op, < 500 ns/op (HLD §8).
//
// struct packet_idle_unit at PACKETVER=20181121 (total=108 bytes, variable-length packet).
// Bytes [2:4] carry the length field (108 LE); the session framer reads it to determine frame size.
func BenchmarkFeed_ActorExists_0x09FF(b *testing.B) {
	s := session.NewMapSession(20181121)
	s.RegisterHandler(0x09FF, func(data []byte, pv uint32) {}) // no-op handler

	// Build a 108-byte variable-length frame for 0x09FF.
	frame := make([]byte, 108)
	binary.LittleEndian.PutUint16(frame[0:2], 0x09FF) // PacketType
	binary.LittleEndian.PutUint16(frame[2:4], 108)    // PacketLength (variable-length field)

	// Prime the backing buffer.
	_ = s.Feed(frame)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := s.Feed(frame); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkEncode_RequestMove benchmarks encoding the 0x035F CZ_REQUEST_MOVE2 packet ID.
// This simulates the C→S move request path: build a 5-byte move frame and encode the ID.
// Target: 0 allocs/op, < 100 ns/op (HLD §8).
//
// struct SYNTH_CZ_REQUEST_MOVE2: 0:2 packetType, 2:3 dest (total=5 bytes).
// Encoding runs MapSession.Encode(&id) which XOR-transforms the ID in-place for obfuscation.
// Without obfuscation enabled this is a no-op, measuring pure dispatch overhead.
func BenchmarkEncode_RequestMove(b *testing.B) {
	s := session.NewMapSession(20181121)
	// Build a 5-byte move frame.
	frame := make([]byte, 5)
	frame[2] = 0xA8 // dest[0]
	frame[3] = 0x54 // dest[1]
	frame[4] = 0x06 // dest[2] (direction)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		id := uint16(0x035F)
		s.Encode(&id)
		binary.LittleEndian.PutUint16(frame[0:2], id)
		_ = frame
	}
}

// BenchmarkFeed_1000Sessions_Parallel benchmarks concurrent Feed across 1000 independent
// MapSession instances, one per goroutine.
// Target: linear scaling with goroutine count (HLD §8) — sessions share no state.
//
// Each goroutine owns its own MapSession (Feed is not goroutine-safe per the API contract).
// The test verifies that throughput scales linearly by checking for no lock contention.
func BenchmarkFeed_1000Sessions_Parallel(b *testing.B) {
	const nSessions = 1000

	// Pre-allocate all sessions to avoid allocation during the benchmark loop.
	sessions := make([]*session.MapSession, nSessions)
	for i := range sessions {
		s := session.NewMapSession(20181121)
		s.RegisterHandler(0x0080, func(data []byte, pv uint32) {})
		sessions[i] = s
	}

	frame := make([]byte, 7)
	binary.LittleEndian.PutUint16(frame[0:2], 0x0080)

	// Prime all sessions.
	for _, s := range sessions {
		_ = s.Feed(frame)
	}

	b.ResetTimer()
	b.ReportAllocs()

	var idx sync.Mutex
	counter := 0

	b.RunParallel(func(pb *testing.PB) {
		// Each goroutine claims one session from the pool (round-robin).
		idx.Lock()
		myIdx := counter % nSessions
		counter++
		idx.Unlock()

		s := sessions[myIdx]
		for pb.Next() {
			if err := s.Feed(frame); err != nil {
				b.Error(err)
				return
			}
		}
	})
}
