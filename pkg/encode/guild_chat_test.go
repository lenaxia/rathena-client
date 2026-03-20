package encode_test

import (
	"bytes"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/encode"
	"github.com/lenaxia/rathena-client/pkg/send"
	"github.com/lenaxia/rathena-client/pkg/session"
)

// TestEncodeGuildChat_WireFormat verifies the guild chat encoder produces
// the correct wire format: packetType(2) + packetLength(2) + "Name : Message\0"
func TestEncodeGuildChat_WireFormat(t *testing.T) {
	req := send.GuildChat{Name: "Alice", Message: "hello"}
	p := encode.EncodeGuildChat(req, 20200401)

	// Packet ID must be 0x017E.
	if p[0] != 0x7E || p[1] != 0x01 {
		t.Errorf("packet ID: got [0x%02X, 0x%02X], want [0x7E, 0x01]", p[0], p[1])
	}

	// Body is "Alice : hello\0"
	wantBody := "Alice : hello\x00"
	wantTotal := 4 + len(wantBody)

	// Length field.
	gotLen := int(p[2]) | int(p[3])<<8
	if gotLen != wantTotal {
		t.Errorf("length field: got %d, want %d", gotLen, wantTotal)
	}

	// Total output size.
	if len(p) != wantTotal {
		t.Fatalf("output length: got %d, want %d", len(p), wantTotal)
	}

	// Body content.
	if string(p[4:]) != wantBody {
		t.Errorf("body: got %q, want %q", string(p[4:]), wantBody)
	}
}

// TestEncodeGuildChat_SendRegistered verifies that session.Send with ActionGuildChat
// succeeds without error. This test currently FAILS because ActionGuildChat has no
// RegisterSendEncoder entry. After Fix B it must succeed.
func TestEncodeGuildChat_SendRegistered(t *testing.T) {
	ms := session.NewMapSession(20200401)
	var buf bytes.Buffer

	err := session.Send(ms, &buf, session.ActionGuildChat, send.GuildChat{
		Name:    "Alice",
		Message: "hello",
	})
	if err != nil {
		t.Fatalf("Send(ActionGuildChat) returned error: %v (encoder not registered?)", err)
	}

	b := buf.Bytes()
	if len(b) < 4 {
		t.Fatalf("output too short: %d bytes", len(b))
	}
	if b[0] != 0x7E || b[1] != 0x01 {
		t.Errorf("packet ID via Send: got [0x%02X, 0x%02X], want [0x7E, 0x01]", b[0], b[1])
	}
}

func BenchmarkEncodeGuildChat(b *testing.B) {
	req := send.GuildChat{Name: "Alice", Message: "hello world"}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodeGuildChat(req, 20200401)
	}
}
