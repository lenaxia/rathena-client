// Package session contains tests for the semantic action API
// (RegisterSemanticHandler and Send) defined in semantic.go.
package session

import (
	"bytes"
	"errors"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/events"
	"github.com/lenaxia/rathena-client/pkg/packing"
	"github.com/lenaxia/rathena-client/pkg/send"
)

// Note: send encoders (ActionMoveTo, ActionPublicChat, etc.) are registered by
// pkg/encode/register.go's init(), which runs because fsm_packets_test.go
// (package session_test, same test binary) imports pkg/encode. The internal
// package session tests cannot import pkg/encode directly due to the import
// cycle (pkg/encode/register.go imports pkg/session).

// TestRegisterSemanticHandler_ActorMoved verifies that a handler registered for
// ActionActorMoved fires with a correctly typed events.ActorMoved when a 0x09FD
// frame is fed to the session.
func TestRegisterSemanticHandler_ActorMoved(t *testing.T) {
	s := NewMapSession(20181121)

	called := 0
	var gotEvent events.ActorMoved
	RegisterSemanticHandler(s, ActionActorMoved, func(e events.ActorMoved) {
		called++
		gotEvent = e
	})

	// At pv >= 20181121, 0x09FD reads up to data[90:114] = 114 bytes minimum.
	s.setLength(0x09FD, 114)
	frame := makeFrame(0x09FD, 114)
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed returned error: %v", err)
	}
	if called != 1 {
		t.Fatalf("handler called %d times, want 1", called)
	}
	_ = gotEvent
}

// TestRegisterSemanticHandler_AllVariants verifies that registering a handler for
// ActionActorMoved covers all 5 packetver variants (0x007B, 0x01DA, 0x022C,
// 0x09DB, 0x09FD).
func TestRegisterSemanticHandler_AllVariants(t *testing.T) {
	s := NewMapSession(20181121)

	called := 0
	RegisterSemanticHandler(s, ActionActorMoved, func(e events.ActorMoved) {
		called++
	})

	// At pv >= 20181121, all ActorMoved decode functions read up to data[90:114].
	// We must set lengths accordingly so frames are large enough for decode.
	// Fixed-length IDs: use 114 bytes (covers all struct fields at modern packetver).
	// Variable-length IDs (0x09DB, 0x09FD): embed length in frame header.
	const frameSize = 114

	s.setLength(0x007B, frameSize)
	s.setLength(0x01DA, frameSize)
	s.setLength(0x022C, frameSize)
	s.setLength(0x09DB, -1)
	s.setLength(0x09FD, -1)

	// Feed one frame per fixed variant.
	for _, id := range []uint16{0x007B, 0x01DA, 0x022C} {
		frame := makeFrame(id, frameSize)
		if err := s.Feed(frame); err != nil {
			t.Fatalf("Feed(0x%04X) returned error: %v", id, err)
		}
	}

	// Feed one frame per variable-length variant.
	// Variable frames embed their total length in bytes 2–3.
	for _, id := range []uint16{0x09DB, 0x09FD} {
		frame := makeVarFrame(id, frameSize)
		if err := s.Feed(frame); err != nil {
			t.Fatalf("Feed(0x%04X) returned error: %v", id, err)
		}
	}

	if called != 5 {
		t.Errorf("handler called %d times, want 5 (one per variant)", called)
	}
}

// TestRegisterSemanticHandler_Overwrite verifies that a second RegisterSemanticHandler
// call for the same action silently overwrites the first, matching RegisterHandler contract.
func TestRegisterSemanticHandler_Overwrite(t *testing.T) {
	s := NewMapSession(20181121)

	first := 0
	second := 0

	RegisterSemanticHandler(s, ActionActorMoved, func(e events.ActorMoved) {
		first++
	})
	// Second registration overwrites the first for all covered packet IDs.
	RegisterSemanticHandler(s, ActionActorMoved, func(e events.ActorMoved) {
		second++
	})

	// 0x09FD at pv=20181121 needs at least 114 bytes.
	s.setLength(0x09FD, 114)
	frame := makeFrame(0x09FD, 114)
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed returned error: %v", err)
	}

	if first != 0 {
		t.Errorf("first handler called %d times after overwrite, want 0", first)
	}
	if second != 1 {
		t.Errorf("second handler called %d times, want 1", second)
	}
}

// TestRegisterSemanticHandler_PanicOnUnknownAction verifies that RegisterSemanticHandler
// panics immediately when given an out-of-range or send-only action.
func TestRegisterSemanticHandler_PanicOnUnknownAction(t *testing.T) {
	s := NewMapSession(20181121)

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for unknown action, got none")
		}
	}()

	// ActionUnknown (0) should not be in receiveDispatch.
	outOfRange := SemanticAction(9999)
	RegisterSemanticHandler(s, outOfRange, func(e events.ActorMoved) {})
}

// TestRegisterSemanticHandler_PanicOnTypeMismatch verifies that registering a handler
// with the wrong event type does not panic at registration, but panics at first dispatch.
func TestRegisterSemanticHandler_PanicOnTypeMismatch(t *testing.T) {
	s := NewMapSession(20181121)

	// Register a handler that expects events.ChatMessage for ActionActorMoved —
	// this is the wrong type. Panic should NOT occur at registration.
	RegisterSemanticHandler(s, ActionActorMoved, func(e events.ChatMessage) {})

	// 0x09FD at pv=20181121 needs at least 114 bytes.
	s.setLength(0x09FD, 114)
	frame := makeFrame(0x09FD, 114)

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic at dispatch due to type mismatch, got none")
		}
	}()
	_ = s.Feed(frame) // should panic here
}

// TestRegisterSemanticHandler_PanicOnPointerType verifies that registering a handler
// with a pointer type (*events.ActorMoved) panics at dispatch with a type mismatch.
func TestRegisterSemanticHandler_PanicOnPointerType(t *testing.T) {
	s := NewMapSession(20181121)

	// Using pointer type — should NOT panic at registration.
	RegisterSemanticHandler(s, ActionActorMoved, func(e *events.ActorMoved) {})

	// 0x09FD at pv=20181121 needs at least 114 bytes.
	s.setLength(0x09FD, 114)
	frame := makeFrame(0x09FD, 114)

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic at dispatch due to pointer type, got none")
		}
	}()
	_ = s.Feed(frame) // should panic here
}

// TestSend_MoveTo verifies that Send with ActionMoveTo produces the correct wire
// bytes for two different packetvers.
func TestSend_MoveTo(t *testing.T) {
	coords := packing.EncodePosDir(100, 200, 0)

	tests := []struct {
		name       string
		packetver  uint32
		wantIDLow  byte
		wantIDHigh byte
	}{
		{
			name:       "post-shuffle 20180308 → 0x035F",
			packetver:  20180308,
			wantIDLow:  0x5F,
			wantIDHigh: 0x03,
		},
		{
			name:       "pre-shuffle 20030000 → 0x0085",
			packetver:  20030000,
			wantIDLow:  0x85,
			wantIDHigh: 0x00,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewMapSession(tc.packetver)
			var buf bytes.Buffer
			err := Send(s, &buf, ActionMoveTo, send.MoveTo{X: 100, Y: 200})
			if err != nil {
				t.Fatalf("Send returned error: %v", err)
			}
			b := buf.Bytes()
			if len(b) < 5 {
				t.Fatalf("output too short: %d bytes", len(b))
			}
			if b[0] != tc.wantIDLow || b[1] != tc.wantIDHigh {
				t.Errorf("packet ID: got [0x%02X, 0x%02X], want [0x%02X, 0x%02X]",
					b[0], b[1], tc.wantIDLow, tc.wantIDHigh)
			}
			// Bytes 2–4 should match packing.EncodePosDir(100, 200, 0).
			if b[2] != coords[0] || b[3] != coords[1] || b[4] != coords[2] {
				t.Errorf("coords: got [0x%02X, 0x%02X, 0x%02X], want [0x%02X, 0x%02X, 0x%02X]",
					b[2], b[3], b[4], coords[0], coords[1], coords[2])
			}
		})
	}
}

// TestSend_VariableLengthAction verifies that Send with ActionPublicChat produces
// a correct variable-length frame (no [:] slice appended).
func TestSend_VariableLengthAction(t *testing.T) {
	s := NewMapSession(20030000)
	var buf bytes.Buffer

	err := Send(s, &buf, ActionPublicChat, send.PublicChat{
		Name:    "Alice",
		Message: "hi",
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	b := buf.Bytes()
	// Wire ID for public_chat at packetver < 20040726 is 0x008C.
	if b[0] != 0x8C || b[1] != 0x00 {
		t.Errorf("packet ID: got [0x%02X, 0x%02X], want [0x8C, 0x00]", b[0], b[1])
	}

	text := "Alice : hi\x00"
	wantLen := 4 + len(text)
	gotLen := int(b[2]) | int(b[3])<<8
	if gotLen != wantLen {
		t.Errorf("frame length: got %d, want %d", gotLen, wantLen)
	}
	if len(b) != wantLen {
		t.Errorf("output length: got %d, want %d", len(b), wantLen)
	}
	if string(b[4:]) != text {
		t.Errorf("body: got %q, want %q", string(b[4:]), text)
	}
}

// TestSend_ObfuscationApplied verifies that Send applies XOR obfuscation to the
// packet ID bytes when obfuscation is enabled.
func TestSend_ObfuscationApplied(t *testing.T) {
	// Use keys from packetver 20180307 — the last version with known obfuscation keys.
	k0, k1, k2 := obfuscationKeysFor(20180307)
	if k0 == 0 && k1 == 0 && k2 == 0 {
		t.Skip("no obfuscation keys for 20180307 — skipping")
	}

	s := NewMapSession(20180307)
	s.enableObfuscation(k0, k1, k2)

	// Compute expected first-packet obfuscated ID.
	// Wire ID for move_to at pv=20180307 is shuffledCtoSID(20180307, 0x0085).
	// Value hardcoded from clif_shuffle.hpp: 20180307 → 0x0085 maps to 0x0877.
	// Source: src/map/clif_shuffle.hpp #elif PACKETVER == 20180307
	const shuffledID = uint16(0x0877)
	firstKey := uint16(((uint64(k0)*uint64(k1) + uint64(k2)) >> 16) & 0x7FFF)
	expectedID := shuffledID ^ firstKey

	var buf bytes.Buffer
	err := Send(s, &buf, ActionMoveTo, send.MoveTo{X: 1, Y: 1})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	b := buf.Bytes()
	gotID := uint16(b[0]) | uint16(b[1])<<8
	if gotID != expectedID {
		t.Errorf("obfuscated ID: got 0x%04X, want 0x%04X (shuffled=0x%04X, firstKey=0x%04X)",
			gotID, expectedID, shuffledID, firstKey)
	}
}

// TestSend_UnknownAction verifies that Send returns an error (not panic) for an
// unknown action and for ActionUnknown (0).
func TestSend_UnknownAction(t *testing.T) {
	s := NewMapSession(20181121)
	var buf bytes.Buffer

	// Test out-of-range action.
	outOfRange := SemanticAction(9999)
	err := Send(s, &buf, outOfRange, nil)
	if err == nil {
		t.Error("expected error for out-of-range action, got nil")
	}

	// Test ActionUnknown (0) — sendRegistry[0] is nil.
	err = Send(s, &buf, ActionUnknown, nil)
	if err == nil {
		t.Error("expected error for ActionUnknown, got nil")
	}
}

// TestSend_WrongType verifies that Send returns ErrWrongSendType when req has
// the wrong concrete type for the action.
func TestSend_WrongType(t *testing.T) {
	s := NewMapSession(20181121)
	var buf bytes.Buffer

	// Pass a send.PublicChat where send.MoveTo is expected.
	err := Send(s, &buf, ActionMoveTo, send.PublicChat{Name: "bad"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrWrongSendType{}) {
		t.Errorf("expected ErrWrongSendType, got %v", err)
	}
}

// TestRegisterSendEncoder_DoublePanic verifies that calling RegisterSendEncoder
// twice for the same action panics (codegen-bug guard).
func TestRegisterSendEncoder_DoublePanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on double registration, got none")
		}
	}()

	// Register a dummy encoder for an action that is already registered.
	// ActionMoveTo should already be registered by the pkg/encode init().
	RegisterSendEncoder(ActionMoveTo, func(req interface{}, pv uint32) ([]byte, error) {
		return nil, nil
	})
}

// BenchmarkRegisterAndFeed_SemanticHandler benchmarks the steady-state dispatch path.
// Note: exactly 1 alloc/op is expected (not 0) because the receiveDispatch lambda
// boxes the decoded event struct into interface{}, which escapes to heap.
func BenchmarkRegisterAndFeed_SemanticHandler(b *testing.B) {
	s := NewMapSession(20181121)
	RegisterSemanticHandler(s, ActionActorMoved, func(e events.ActorMoved) {
		// Do not retain any fields — handler must not allocate.
		_ = e.GID
	})

	// At pv=20181121, 0x09FD needs 114 bytes for decode to succeed.
	s.setLength(0x09FD, 114)
	frame := makeFrame(0x09FD, 114)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = s.Feed(frame)
	}
}
