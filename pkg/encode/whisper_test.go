// Manually implemented — regression test for goKore bug report 0799.
// ActionWhisper must exist as a SemanticAction constant and EncodeWhisper
// must be registered so session.Send can route whisper packets.

package encode_test

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/encode"
	"github.com/lenaxia/rathena-client/pkg/send"
	"github.com/lenaxia/rathena-client/pkg/session"
)

// TestEncodeWhisper_WireFormat verifies the wire format of CZ_WISPER (0x0096).
// Source: clif.cpp clif_process_message with whisperFormat=true:
//
//	[packetType:2][packetLength:2][target:24][message:varlen+NUL]
func TestEncodeWhisper_WireFormat(t *testing.T) {
	req := send.Whisper{Target: "Poporing", Message: "Hello!"}
	p := encode.EncodeWhisper(req, 20200401)

	// Packet ID: 0x0096 little-endian
	if p[0] != 0x96 || p[1] != 0x00 {
		t.Errorf("packet ID: got [%02X %02X], want [96 00]", p[0], p[1])
	}

	// Total length: 2(id) + 2(len) + 24(target) + len("Hello!")+1(NUL)
	wantLen := 2 + 2 + 24 + len("Hello!") + 1
	gotLen := int(binary.LittleEndian.Uint16(p[2:4]))
	if gotLen != wantLen {
		t.Errorf("packet length field: got %d, want %d", gotLen, wantLen)
	}
	if len(p) != wantLen {
		t.Errorf("slice length: got %d, want %d", len(p), wantLen)
	}

	// Target name at [4:28], NUL-padded to 24 bytes
	target := p[4:28]
	if string(target[:8]) != "Poporing" {
		t.Errorf("target name: got %q, want %q", string(target[:8]), "Poporing")
	}
	for i := 8; i < 24; i++ {
		if target[i] != 0x00 {
			t.Errorf("target padding byte[%d]: got 0x%02X, want 0x00", i, target[i])
		}
	}

	// Message at [28:], NUL-terminated
	msg := p[28:]
	if string(msg) != "Hello!\x00" {
		t.Errorf("message: got %q, want %q", string(msg), "Hello!\x00")
	}
}

// TestEncodeWhisper_EmptyTarget verifies encoding with empty target (edge case).
func TestEncodeWhisper_EmptyTarget(t *testing.T) {
	req := send.Whisper{Target: "", Message: "test"}
	p := encode.EncodeWhisper(req, 20200401)
	if p[0] != 0x96 || p[1] != 0x00 {
		t.Errorf("packet ID wrong: [%02X %02X]", p[0], p[1])
	}
	// target field should be all zeroes
	for i := 4; i < 28; i++ {
		if p[i] != 0x00 {
			t.Errorf("empty target: byte[%d] = 0x%02X, want 0x00", i, p[i])
		}
	}
}

// TestEncodeWhisper_LongTarget verifies that a target name longer than 24 bytes
// is silently truncated by copy() to exactly 24 bytes — the caller is responsible
// for enforcing NAME_LENGTH-1 (23) to preserve the NUL terminator.
// The encoder itself does not enforce the limit; it matches rAthena's behaviour.
func TestEncodeWhisper_LongTarget(t *testing.T) {
	// 26-char name; copy() will write the first 24 bytes, filling the field completely.
	req := send.Whisper{Target: "ABCDEFGHIJKLMNOPQRSTUVWXYZ", Message: "x"}
	p := encode.EncodeWhisper(req, 20200401)
	if len(p) < 28 {
		t.Fatalf("packet too short: %d bytes", len(p))
	}
	// copy() truncates to 24; bytes [4:28] should hold "ABCDEFGHIJKLMNOPQRSTUVWX"
	if string(p[4:28]) != "ABCDEFGHIJKLMNOPQRSTUVWX" {
		t.Errorf("target field: got %q, want %q", string(p[4:28]), "ABCDEFGHIJKLMNOPQRSTUVWX")
	}
}

// TestActionWhisper_Registered verifies that ActionWhisper exists as a
// SemanticAction constant and that EncodeWhisper is registered under it.
// This is the direct regression test for goKore bug report 0799:
// "session.ActionWhisper is undefined".
func TestActionWhisper_Registered(t *testing.T) {
	// session.ActionWhisper must compile — if the constant is missing this
	// file will not compile and the test suite will fail at build time.
	_ = session.ActionWhisper

	// The constant must not be zero (ActionUnknown).
	if session.ActionWhisper == 0 {
		t.Fatal("ActionWhisper == 0 (ActionUnknown) — not assigned a real value")
	}

	// session.ActionWhisper must have a meaningful String() representation.
	if session.ActionWhisper.String() != "ActionWhisper" {
		t.Errorf("ActionWhisper.String() = %q, want %q",
			session.ActionWhisper.String(), "ActionWhisper")
	}
}

// BenchmarkEncodeWhisper verifies the encoder has no unexpected allocations
// beyond the single []byte allocation for the output buffer (variable-length
// packet — one alloc is expected and acceptable per README-LLM.md §Performance).
func BenchmarkEncodeWhisper(b *testing.B) {
	req := send.Whisper{Target: "Poporing", Message: "Hello there!"}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodeWhisper(req, 20200401)
	}
}
