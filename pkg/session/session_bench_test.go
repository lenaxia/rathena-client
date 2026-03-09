// Package session benchmarks — verifies zero-alloc invariant on the decode hot path.
package session_test

import (
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
