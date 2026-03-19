// Package decode — golden tests for GuildChat_0x017F.
//
// ZC_GUILD_CHAT (variable length):
//
//	offset 0, size 2: packetType int16 = 0x017F (LE: 0x7F 0x01)
//	offset 2, size 2: packetLength int16 (total packet length including header)
//	offset 4, size N: message char[] (null-terminated, format "name : text\x00")
//
// rAthena source: packets.hpp PACKET_ZC_GUILD_CHAT
//
//	struct PACKET_ZC_GUILD_CHAT { int16 packetType; int16 packetLength; char message[]; }
//	DEFINE_PACKET_HEADER(ZC_GUILD_CHAT, 0x17f)
package decode

import "testing"

// TestGuildChat_0x017F_Basic verifies basic guild chat message decoding.
func TestGuildChat_0x017F_Basic(t *testing.T) {
	data := []byte{0x7F, 0x01, 0x0F, 0x00, 'A', 'l', 'i', 'c', 'e', ' ', ':', ' ', 'h', 'i', 0x00}
	e := GuildChat_0x017F(data, 20180307)

	if e.Message != "Alice : hi" {
		t.Errorf("Message: got %q want %q", e.Message, "Alice : hi")
	}
}

// TestGuildChat_0x017F_EmptyBody verifies that a 4-byte packet (header only) produces an empty message.
func TestGuildChat_0x017F_EmptyBody(t *testing.T) {
	data := []byte{0x7F, 0x01, 0x04, 0x00}
	e := GuildChat_0x017F(data, 20180307)

	if e.Message != "" {
		t.Errorf("Message: got %q want %q", e.Message, "")
	}
}

// BenchmarkGuildChat_0x017F benchmarks the decode hot path.
// NOTE: GuildChat.Message is a string built via nullTermString (unsafe.String alias).
// The unsafe.String call itself is zero-alloc, but the Go compiler may escape the
// string to the heap when it is stored in the GuildChat struct and passed via
// interface{} through the dispatch table (1 alloc expected for the boxing, not
// this function). This benchmark exercises the decode function in isolation —
// expect 0 allocs/op here since the returned struct never touches interface{}.
func BenchmarkGuildChat_0x017F(b *testing.B) {
	data := []byte{0x7F, 0x01, 0x0F, 0x00, 'A', 'l', 'i', 'c', 'e', ' ', ':', ' ', 'h', 'i', 0x00}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = GuildChat_0x017F(data, 20180307)
	}
}
