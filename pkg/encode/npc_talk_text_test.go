package encode_test

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/encode"
	"github.com/lenaxia/rathena-client/pkg/send"
)

// TestEncodeNpcTalkText_TextWritten verifies that the text value appears in the output.
// This test currently FAILS because EncodeNpcTalkText returns [8]byte and
// copy(p[8:8], ...) is a no-op. After the fix it must return []byte with text at offset 8.
func TestEncodeNpcTalkText_TextWritten(t *testing.T) {
	req := send.NpcTalkText{GID: 12345, Value: "hello"}
	p := encode.EncodeNpcTalkText(req, 20200401)
	if len(p) < 8+5 {
		t.Fatalf("output too short: got %d bytes, want >= 13", len(p))
	}
	want := []byte("hello")
	for i, b := range want {
		if p[8+i] != b {
			t.Errorf("text byte[%d]: got 0x%02X (%q), want 0x%02X (%q)",
				i, p[8+i], rune(p[8+i]), b, rune(b))
		}
	}
}

// TestEncodeNpcTalkText_PacketID verifies the wire packet ID.
func TestEncodeNpcTalkText_PacketID(t *testing.T) {
	req := send.NpcTalkText{GID: 1, Value: "x"}
	p := encode.EncodeNpcTalkText(req, 20200401)
	if p[0] != 0xd5 || p[1] != 0x01 {
		t.Errorf("packet ID: got [0x%02X, 0x%02X], want [0xD5, 0x01]", p[0], p[1])
	}
}

// TestEncodeNpcTalkText_GIDField verifies the GID field is encoded at bytes 4-7.
func TestEncodeNpcTalkText_GIDField(t *testing.T) {
	const gid int32 = 0x7EADBEEF
	req := send.NpcTalkText{GID: gid, Value: "x"}
	p := encode.EncodeNpcTalkText(req, 20200401)
	got := int32(binary.LittleEndian.Uint32(p[4:8]))
	if got != gid {
		t.Errorf("GID: got 0x%08X, want 0x7EADBEEF", got)
	}
}

// TestEncodeNpcTalkText_LengthField verifies that bytes 2-3 contain the total packet length.
func TestEncodeNpcTalkText_LengthField(t *testing.T) {
	text := "hello"
	req := send.NpcTalkText{GID: 1, Value: text}
	p := encode.EncodeNpcTalkText(req, 20200401)
	gotLen := int(binary.LittleEndian.Uint16(p[2:4]))
	wantLen := 8 + len(text)
	if gotLen != wantLen {
		t.Errorf("length field: got %d, want %d (8 header + %d text)", gotLen, wantLen, len(text))
	}
}

// TestEncodeNpcTalkText_TotalLength verifies the total output slice length.
func TestEncodeNpcTalkText_TotalLength(t *testing.T) {
	text := "hello world"
	req := send.NpcTalkText{GID: 1, Value: text}
	p := encode.EncodeNpcTalkText(req, 20200401)
	want := 8 + len(text)
	if len(p) != want {
		t.Errorf("output length: got %d, want %d", len(p), want)
	}
}

// TestEncodeNpcTalkText_EmptyString verifies empty text produces an 8-byte packet.
func TestEncodeNpcTalkText_EmptyString(t *testing.T) {
	req := send.NpcTalkText{GID: 1, Value: ""}
	p := encode.EncodeNpcTalkText(req, 20200401)
	if len(p) != 8 {
		t.Errorf("empty text: got %d bytes, want 8", len(p))
	}
}

// TestEncodeNpcTalkText_Unicode verifies multi-byte UTF-8 text is written correctly.
func TestEncodeNpcTalkText_Unicode(t *testing.T) {
	text := "テスト" // 3 chars × 3 bytes = 9 bytes in UTF-8
	req := send.NpcTalkText{GID: 1, Value: text}
	p := encode.EncodeNpcTalkText(req, 20200401)
	wantLen := 8 + len(text)
	if len(p) != wantLen {
		t.Fatalf("length: got %d, want %d", len(p), wantLen)
	}
	if string(p[8:]) != text {
		t.Errorf("text: got %q, want %q", string(p[8:]), text)
	}
}

func BenchmarkEncodeNpcTalkText(b *testing.B) {
	req := send.NpcTalkText{GID: 12345, Value: "Hello, World!"}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodeNpcTalkText(req, 20200401)
	}
}
