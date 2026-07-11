// Tests for the modern packet ID decoders added in worklog 0092.
// Each test synthesises a wire frame from the rAthena struct layout,
// decodes it, and asserts field values.

package decode_test

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/decode"
)

// TestPrivateMessage_0x09DE_WireFormat verifies the 0x09DE (modern whisper)
// decoder. rAthena packets_struct.hpp:5348-5358 at pv >= 20131204:
//
//	int16  PacketType    // offset 0
//	int16  PacketLength  // offset 2
//	uint32 senderGID     // offset 4
//	char   sender[24]    // offset 8
//	uint8  isAdmin       // offset 32
//	char   message[]     // offset 33
func TestPrivateMessage_0x09DE_WireFormat(t *testing.T) {
	const sender = "Alice"
	const msg = "Hello, secret"
	const totalLen = 33 + len(msg) + 1

	buf := make([]byte, totalLen)
	binary.LittleEndian.PutUint16(buf[0:2], 0x09DE)
	binary.LittleEndian.PutUint16(buf[2:4], uint16(totalLen))
	binary.LittleEndian.PutUint32(buf[4:8], 0xDEADBEEF)
	copy(buf[8:32], sender)
	buf[32] = 1 // isAdmin
	copy(buf[33:], msg)

	e := decode.PrivateMessage_0x09DE(buf, 20200401)
	if e.PacketLength != int16(totalLen) {
		t.Errorf("PacketLength: got %d, want %d", e.PacketLength, totalLen)
	}
	if e.SenderGID != 0xDEADBEEF {
		t.Errorf("SenderGID: got 0x%08X, want 0xDEADBEEF", e.SenderGID)
	}
	if e.Sender != sender {
		t.Errorf("Sender: got %q, want %q", e.Sender, sender)
	}
	if e.IsAdmin != 1 {
		t.Errorf("IsAdmin: got %d, want 1", e.IsAdmin)
	}
	if e.Message != msg {
		t.Errorf("Message: got %q, want %q", e.Message, msg)
	}
}

// TestZcAckReqnameall_0x0A30_WireFormat verifies the 0x0A30 (modern name
// resolution) decoder. rAthena packets_struct.hpp:3564-3573 at pv >= 20150225:
//
//	uint16 packet_id       // offset 0
//	int32  gid             // offset 2
//	char   name[24]        // offset 6
//	char   party_name[24]  // offset 30
//	char   guild_name[24]  // offset 54
//	char   position_name[24] // offset 78
//	int32  title_id        // offset 102
//	total = 106 bytes
func TestZcAckReqnameall_0x0A30_WireFormat(t *testing.T) {
	buf := make([]byte, 106)
	binary.LittleEndian.PutUint16(buf[0:2], 0x0A30)
	binary.LittleEndian.PutUint32(buf[2:6], 100001)
	copy(buf[6:30], "Alice")
	copy(buf[30:54], "MyParty")
	copy(buf[54:78], "MyGuild")
	copy(buf[78:102], "Member")
	binary.LittleEndian.PutUint32(buf[102:106], 42)

	e := decode.ZcAckReqnameall_0x0A30(buf, 20200401)
	if e.Gid != 100001 {
		t.Errorf("Gid: got %d, want 100001", e.Gid)
	}
	if e.Name != "Alice" {
		t.Errorf("Name: got %q, want Alice", e.Name)
	}
	if e.Party_name != "MyParty" {
		t.Errorf("Party_name: got %q, want MyParty", e.Party_name)
	}
	if e.Guild_name != "MyGuild" {
		t.Errorf("Guild_name: got %q, want MyGuild", e.Guild_name)
	}
	if e.Position_name != "Member" {
		t.Errorf("Position_name: got %q, want Member", e.Position_name)
	}
	if e.Title_id != 42 {
		t.Errorf("Title_id: got %d, want 42", e.Title_id)
	}
}

// TestZcChangeGuild_0x0B1F_WireFormat and _0x0B47_WireFormat both exercise
// the same 14-byte layout that appears at pv 20190703 (0x0B1F) and again
// at pv 20190807 (0x0B47, per RE 20190731 / MAIN 20190807).
//
// rAthena packets_struct.hpp guard at pv >= 20190703:
//
//	int16  packetType   // offset 0
//	int32  guild_id     // offset 2
//	uint32 emblem_id    // offset 6
//	uint32 AID          // offset 10
func TestZcChangeGuild_0x0B1F_WireFormat(t *testing.T) {
	buf := make([]byte, 14)
	binary.LittleEndian.PutUint16(buf[0:2], 0x0B1F)
	binary.LittleEndian.PutUint32(buf[2:6], 500)
	binary.LittleEndian.PutUint32(buf[6:10], 42)
	binary.LittleEndian.PutUint32(buf[10:14], 2000001)

	e := decode.ZcChangeGuild_0x0B1F(buf, 20190800)
	if e.Guild_id != 500 {
		t.Errorf("Guild_id: got %d, want 500", e.Guild_id)
	}
	if e.Emblem_id != 42 {
		t.Errorf("Emblem_id: got %d, want 42", e.Emblem_id)
	}
	if e.AID != 2000001 {
		t.Errorf("AID: got %d, want 2000001", e.AID)
	}
}

func TestZcChangeGuild_0x0B47_WireFormat(t *testing.T) {
	buf := make([]byte, 14)
	binary.LittleEndian.PutUint16(buf[0:2], 0x0B47)
	binary.LittleEndian.PutUint32(buf[2:6], 500)
	binary.LittleEndian.PutUint32(buf[6:10], 42)
	binary.LittleEndian.PutUint32(buf[10:14], 2000001)

	e := decode.ZcChangeGuild_0x0B47(buf, 20200401)
	if e.Guild_id != 500 {
		t.Errorf("Guild_id: got %d, want 500", e.Guild_id)
	}
	if e.Emblem_id != 42 {
		t.Errorf("Emblem_id: got %d, want 42", e.Emblem_id)
	}
	if e.AID != 2000001 {
		t.Errorf("AID: got %d, want 2000001", e.AID)
	}
}

// Benchmarks — Rule 4 target: 0 allocs/op.

func BenchmarkPrivateMessage_0x09DE(b *testing.B) {
	buf := make([]byte, 50)
	binary.LittleEndian.PutUint16(buf[0:2], 0x09DE)
	binary.LittleEndian.PutUint16(buf[2:4], 50)
	binary.LittleEndian.PutUint32(buf[4:8], 0xDEADBEEF)
	copy(buf[8:32], "Alice")
	buf[32] = 1
	copy(buf[33:], "Hello world")
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = decode.PrivateMessage_0x09DE(buf, 20200401)
	}
}

func BenchmarkZcAckReqnameall_0x0A30(b *testing.B) {
	buf := make([]byte, 106)
	binary.LittleEndian.PutUint16(buf[0:2], 0x0A30)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = decode.ZcAckReqnameall_0x0A30(buf, 20200401)
	}
}

func BenchmarkZcChangeGuild_0x0B47(b *testing.B) {
	buf := make([]byte, 14)
	binary.LittleEndian.PutUint16(buf[0:2], 0x0B47)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = decode.ZcChangeGuild_0x0B47(buf, 20200401)
	}
}
