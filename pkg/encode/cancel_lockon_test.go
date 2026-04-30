// TDD test for ActionCancelLockon (CZ_CANCEL_LOCKON 0x0118) — worklog 0085.
// Same gap class as ActionWhisper (worklog 0076) and the chat actions (worklog 0077):
// encoder existed after codegen, test ensures the SemanticAction constant is
// emitted and the wire format matches rAthena source.

package encode_test

import (
	"testing"

	"github.com/lenaxia/rathena-client/pkg/encode"
	"github.com/lenaxia/rathena-client/pkg/send"
	"github.com/lenaxia/rathena-client/pkg/session"
)

// TestEncodeCancelLockon_WireFormat verifies CZ_CANCEL_LOCKON (0x0118).
// Source: clif_packetdb.hpp:115 parseable_packet(0x0118, 2, clif_parse_StopAttack, 0)
// Wire: [packetType:2] — header only, no payload, no length field.
func TestEncodeCancelLockon_WireFormat(t *testing.T) {
	req := send.CancelLockon{}
	p := encode.EncodeCancelLockon(req, 20200401)

	if len(p) != 2 {
		t.Fatalf("packet length: got %d, want 2", len(p))
	}
	if p[0] != 0x18 || p[1] != 0x01 {
		t.Errorf("packet ID: got [%02X %02X], want [18 01] (0x0118 little-endian)", p[0], p[1])
	}
}

// TestActionCancelLockon_Registered is the regression guard: compiles only if
// session.ActionCancelLockon exists. Asserts non-zero value and correct String().
func TestActionCancelLockon_Registered(t *testing.T) {
	_ = session.ActionCancelLockon
	if session.ActionCancelLockon == 0 {
		t.Fatal("ActionCancelLockon == 0 (ActionUnknown) — codegen did not emit the constant")
	}
	if got := session.ActionCancelLockon.String(); got != "ActionCancelLockon" {
		t.Errorf("String() = %q, want ActionCancelLockon", got)
	}
}

// BenchmarkEncodeCancelLockon verifies 0 allocs/op (fixed-size [2]byte return).
func BenchmarkEncodeCancelLockon(b *testing.B) {
	req := send.CancelLockon{}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodeCancelLockon(req, 20200401)
	}
}
