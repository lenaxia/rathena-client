// Package session — trace API tests (Feature 1-6 debuggability).
// Tests are written FIRST per TDD contract; they must fail before implementation.
package session

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/events"
	"github.com/lenaxia/rathena-client/pkg/send"
)

// ─── WireInbound tests ────────────────────────────────────────────────────────

// TestTraceFunc_WireInbound_FiresForKnownPacketWithHandler verifies that WireInbound
// fires with correct ID, frame bytes, and packetver for a known packet with a handler.
func TestTraceFunc_WireInbound_FiresForKnownPacketWithHandler(t *testing.T) {
	const pv = uint32(20181002)
	s := NewMapSession(pv)

	var got []TraceEvent
	s.SetTraceFunc(func(e TraceEvent) { got = append(got, e) })

	called := 0
	s.registerHandler(0x0069, func(data []byte, pv uint32) { called++ })

	frame := makeVarFrame(0x0069, 12)
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed error: %v", err)
	}

	if called != 1 {
		t.Fatalf("handler called %d times, want 1", called)
	}

	var wi WireInbound
	found := false
	for _, e := range got {
		if w, ok := e.(WireInbound); ok {
			wi = w
			found = true
			break
		}
	}
	if !found {
		t.Fatal("WireInbound event not fired")
	}
	if wi.ID != 0x0069 {
		t.Errorf("WireInbound.ID = %#04x, want 0x0069", wi.ID)
	}
	if wi.Packetver != pv {
		t.Errorf("WireInbound.Packetver = %d, want %d", wi.Packetver, pv)
	}
	if len(wi.Frame) != 12 {
		t.Errorf("WireInbound.Frame len = %d, want 12", len(wi.Frame))
	}
	if wi.Frame[0] != 0x69 || wi.Frame[1] != 0x00 {
		t.Errorf("WireInbound.Frame ID bytes wrong: [0x%02x,0x%02x]", wi.Frame[0], wi.Frame[1])
	}
	if wi.Time.IsZero() {
		t.Error("WireInbound.Time is zero")
	}
}

// TestTraceFunc_WireInbound_FiresForKnownPacketWithoutHandler verifies that WireInbound
// still fires for a known packet even when no handler is registered.
func TestTraceFunc_WireInbound_FiresForKnownPacketWithoutHandler(t *testing.T) {
	s := NewMapSession(20181002)

	var got []TraceEvent
	s.SetTraceFunc(func(e TraceEvent) { got = append(got, e) })

	// 0x0080 is a 7-byte fixed-length packet in the map lengths table; no handler registered.
	frame := make([]byte, 7)
	frame[0] = 0x80
	frame[1] = 0x00

	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed error: %v", err)
	}

	found := false
	for _, e := range got {
		if w, ok := e.(WireInbound); ok && w.ID == 0x0080 {
			found = true
		}
	}
	if !found {
		t.Fatal("WireInbound not fired for known packet without handler")
	}
}

// TestTraceFunc_WireInbound_NotFiredForUnknownPacketID verifies that WireInbound
// does NOT fire for an unknown packet ID — only UnknownPacketEvent fires.
func TestTraceFunc_WireInbound_NotFiredForUnknownPacketID(t *testing.T) {
	s := NewMapSession(20181002)

	var got []TraceEvent
	s.SetTraceFunc(func(e TraceEvent) { got = append(got, e) })

	if err := s.Feed(makeFrame(0xFFFF, 2)); err != nil {
		t.Fatalf("Feed error: %v", err)
	}

	for _, e := range got {
		if _, ok := e.(WireInbound); ok {
			t.Error("WireInbound fired for unknown packet ID — must NOT fire")
		}
	}

	// UnknownPacketEvent should have fired.
	found := false
	for _, e := range got {
		if u, ok := e.(UnknownPacketEvent); ok && u.ID == 0xFFFF {
			found = true
		}
	}
	if !found {
		t.Error("UnknownPacketEvent not fired for unknown packet ID")
	}
}

// TestTraceFunc_WireInbound_FrameIsSafeToRetainAfterFeed verifies that the frame
// from WireInbound is a heap-allocated copy safe to retain after Feed returns.
func TestTraceFunc_WireInbound_FrameIsSafeToRetainAfterFeed(t *testing.T) {
	s := NewMapSession(20181002)

	var retained []byte
	captured := false
	s.SetTraceFunc(func(e TraceEvent) {
		if w, ok := e.(WireInbound); ok && w.ID == 0x0069 && !captured {
			retained = w.Frame
			captured = true
		}
	})
	s.registerHandler(0x0069, func(data []byte, pv uint32) {})

	frame := makeVarFrame(0x0069, 8)
	frame[4] = 0xAB
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed error: %v", err)
	}
	if len(retained) == 0 {
		t.Fatal("no frame retained")
	}
	original := append([]byte(nil), retained...)

	// Feed more packets — this overwrites the session buffer.
	for i := 0; i < 5; i++ {
		_ = s.Feed(makeVarFrame(0x0069, 8))
	}

	if !bytes.Equal(retained, original) {
		t.Errorf("retained frame changed after more Feeds: was %x, now %x", original, retained)
	}
}

// ─── UnknownPacketEvent via TraceFunc tests ───────────────────────────────────

// TestTraceFunc_UnknownPacket_FiresViaTraceFunc verifies that an unknown packet ID
// fires UnknownPacketEvent via TraceFunc even without SetUnknownPacketHandler.
func TestTraceFunc_UnknownPacket_FiresViaTraceFunc(t *testing.T) {
	s := NewMapSession(20181002)

	var got []TraceEvent
	s.SetTraceFunc(func(e TraceEvent) { got = append(got, e) })
	// No SetUnknownPacketHandler set.

	if err := s.Feed(makeFrame(0xFFFF, 2)); err != nil {
		t.Fatalf("Feed error: %v", err)
	}

	found := false
	for _, e := range got {
		if u, ok := e.(UnknownPacketEvent); ok && u.ID == 0xFFFF {
			found = true
		}
	}
	if !found {
		t.Error("UnknownPacketEvent not received via TraceFunc")
	}
}

// TestTraceFunc_UnknownPacket_FiresBothCallbacks verifies that BOTH TraceFunc
// AND SetUnknownPacketHandler fire for the same unknown packet event.
func TestTraceFunc_UnknownPacket_FiresBothCallbacks(t *testing.T) {
	s := NewMapSession(20181002)

	traceFired := 0
	handlerFired := 0

	s.SetTraceFunc(func(e TraceEvent) {
		if _, ok := e.(UnknownPacketEvent); ok {
			traceFired++
		}
	})
	s.SetUnknownPacketHandler(func(ev UnknownPacketEvent) {
		handlerFired++
	})

	if err := s.Feed(makeFrame(0xFFFF, 2)); err != nil {
		t.Fatalf("Feed error: %v", err)
	}

	if traceFired != 1 {
		t.Errorf("TraceFunc fired %d times for UnknownPacketEvent, want 1", traceFired)
	}
	if handlerFired != 1 {
		t.Errorf("SetUnknownPacketHandler fired %d times, want 1", handlerFired)
	}
}

// TestTraceFunc_UnknownPacket_OnlyUnknownPacketHandlerWired verifies that when
// TraceFunc is nil, only SetUnknownPacketHandler fires (no panic).
func TestTraceFunc_UnknownPacket_OnlyUnknownPacketHandlerWired(t *testing.T) {
	s := NewMapSession(20181002)

	// TraceFunc is nil (not set).
	handlerFired := 0
	s.SetUnknownPacketHandler(func(ev UnknownPacketEvent) {
		handlerFired++
	})

	if err := s.Feed(makeFrame(0xFFFF, 2)); err != nil {
		t.Fatalf("Feed error: %v", err)
	}
	if handlerFired != 1 {
		t.Errorf("handler fired %d times, want 1", handlerFired)
	}
}

// ─── SemanticIn tests ─────────────────────────────────────────────────────────

// TestTraceFunc_SemanticIn_FiresAfterDispatch verifies that SemanticIn fires with
// correct Action, ID, and Event type after RegisterSemanticHandler dispatch.
func TestTraceFunc_SemanticIn_FiresAfterDispatch(t *testing.T) {
	s := NewMapSession(20181121)

	var got []TraceEvent
	s.SetTraceFunc(func(e TraceEvent) { got = append(got, e) })

	RegisterSemanticHandler(s, ActionActorMoved, func(e events.ActorMoved) {})

	s.setLength(0x09FD, 114)
	frame := makeFrame(0x09FD, 114)
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed error: %v", err)
	}

	var si SemanticIn
	found := false
	for _, e := range got {
		if v, ok := e.(SemanticIn); ok {
			si = v
			found = true
			break
		}
	}
	if !found {
		t.Fatal("SemanticIn not fired")
	}
	if si.Action != ActionActorMoved {
		t.Errorf("SemanticIn.Action = %v, want ActionActorMoved", si.Action)
	}
	if si.ID != 0x09FD {
		t.Errorf("SemanticIn.ID = %#04x, want 0x09FD", si.ID)
	}
	if _, ok := si.Event.(events.ActorMoved); !ok {
		t.Errorf("SemanticIn.Event type = %T, want events.ActorMoved", si.Event)
	}
}

// TestTraceFunc_SemanticIn_PairedWithWireInbound verifies that WireInbound and
// SemanticIn both fire for the same packet and carry the same ID and frame bytes
// (but different copies — they should be independent byte slices).
func TestTraceFunc_SemanticIn_PairedWithWireInbound(t *testing.T) {
	s := NewMapSession(20181121)

	var wireEvents []WireInbound
	var semEvents []SemanticIn
	s.SetTraceFunc(func(e TraceEvent) {
		switch v := e.(type) {
		case WireInbound:
			wireEvents = append(wireEvents, v)
		case SemanticIn:
			semEvents = append(semEvents, v)
		}
	})

	RegisterSemanticHandler(s, ActionActorMoved, func(e events.ActorMoved) {})

	s.setLength(0x09FD, 114)
	frame := makeFrame(0x09FD, 114)
	frame[4] = 0xAB // put a marker in payload
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed error: %v", err)
	}

	if len(wireEvents) != 1 {
		t.Fatalf("WireInbound count = %d, want 1", len(wireEvents))
	}
	if len(semEvents) != 1 {
		t.Fatalf("SemanticIn count = %d, want 1", len(semEvents))
	}

	wi := wireEvents[0]
	si := semEvents[0]

	if wi.ID != si.ID {
		t.Errorf("WireInbound.ID %#04x != SemanticIn.ID %#04x", wi.ID, si.ID)
	}
	if !bytes.Equal(wi.Frame, si.Frame) {
		t.Errorf("WireInbound.Frame != SemanticIn.Frame")
	}
	// They should be the same bytes but different backing arrays (independent copies).
	if len(wi.Frame) > 0 && len(si.Frame) > 0 {
		if &wi.Frame[0] == &si.Frame[0] {
			t.Error("WireInbound.Frame and SemanticIn.Frame share the same backing array — must be independent copies")
		}
	}
}

// TestTraceFunc_SemanticIn_NotFiredIfNoSemanticHandler verifies that SemanticIn
// does NOT fire when using registerHandler directly (not RegisterSemanticHandler),
// but WireInbound DOES fire.
func TestTraceFunc_SemanticIn_NotFiredIfNoSemanticHandler(t *testing.T) {
	s := NewMapSession(20181002)

	var wireCount, semCount int
	s.SetTraceFunc(func(e TraceEvent) {
		switch e.(type) {
		case WireInbound:
			wireCount++
		case SemanticIn:
			semCount++
		}
	})

	// Use raw registerHandler, NOT RegisterSemanticHandler.
	s.registerHandler(0x0069, func(data []byte, pv uint32) {})

	if err := s.Feed(makeVarFrame(0x0069, 8)); err != nil {
		t.Fatalf("Feed error: %v", err)
	}

	if wireCount != 1 {
		t.Errorf("WireInbound count = %d, want 1", wireCount)
	}
	if semCount != 0 {
		t.Errorf("SemanticIn count = %d, want 0 (raw handler, not semantic)", semCount)
	}
}

// ─── WireOutbound + SemanticOut tests ─────────────────────────────────────────

// TestTraceFunc_WireOutbound_FiresOnSend verifies that WireOutbound fires after Send
// with correct Action, non-empty Frame, and correct Packetver.
func TestTraceFunc_WireOutbound_FiresOnSend(t *testing.T) {
	const pv = uint32(20181002)
	s := NewMapSession(pv)

	var got []TraceEvent
	s.SetTraceFunc(func(e TraceEvent) { got = append(got, e) })

	var buf bytes.Buffer
	if err := Send(s, &buf, ActionMoveTo, send.MoveTo{X: 100, Y: 200}); err != nil {
		t.Fatalf("Send error: %v", err)
	}

	var wo WireOutbound
	found := false
	for _, e := range got {
		if w, ok := e.(WireOutbound); ok {
			wo = w
			found = true
			break
		}
	}
	if !found {
		t.Fatal("WireOutbound not fired after Send")
	}
	if wo.Action != ActionMoveTo {
		t.Errorf("WireOutbound.Action = %v, want ActionMoveTo", wo.Action)
	}
	if len(wo.Frame) == 0 {
		t.Error("WireOutbound.Frame is empty")
	}
	if wo.Packetver != pv {
		t.Errorf("WireOutbound.Packetver = %d, want %d", wo.Packetver, pv)
	}
	if wo.Time.IsZero() {
		t.Error("WireOutbound.Time is zero")
	}
}

// TestTraceFunc_SemanticOut_FiresOnSend verifies that SemanticOut fires after Send
// with correct Action and Request struct.
func TestTraceFunc_SemanticOut_FiresOnSend(t *testing.T) {
	s := NewMapSession(20181002)

	var got []TraceEvent
	s.SetTraceFunc(func(e TraceEvent) { got = append(got, e) })

	req := send.MoveTo{X: 55, Y: 77}
	var buf bytes.Buffer
	if err := Send(s, &buf, ActionMoveTo, req); err != nil {
		t.Fatalf("Send error: %v", err)
	}

	var so SemanticOut
	found := false
	for _, e := range got {
		if v, ok := e.(SemanticOut); ok {
			so = v
			found = true
			break
		}
	}
	if !found {
		t.Fatal("SemanticOut not fired after Send")
	}
	if so.Action != ActionMoveTo {
		t.Errorf("SemanticOut.Action = %v, want ActionMoveTo", so.Action)
	}
	gotReq, ok := so.Request.(send.MoveTo)
	if !ok {
		t.Fatalf("SemanticOut.Request type = %T, want send.MoveTo", so.Request)
	}
	if gotReq.X != req.X || gotReq.Y != req.Y {
		t.Errorf("SemanticOut.Request = %+v, want %+v", gotReq, req)
	}
}

// TestTraceFunc_WireOutbound_SemanticOut_ShareSameFrame verifies that WireOutbound.Frame
// and SemanticOut.Frame are the same slice (same pointer/backing array).
func TestTraceFunc_WireOutbound_SemanticOut_ShareSameFrame(t *testing.T) {
	s := NewMapSession(20181002)

	var wireOut *WireOutbound
	var semOut *SemanticOut
	s.SetTraceFunc(func(e TraceEvent) {
		switch v := e.(type) {
		case WireOutbound:
			cp := v
			wireOut = &cp
		case SemanticOut:
			cp := v
			semOut = &cp
		}
	})

	var buf bytes.Buffer
	if err := Send(s, &buf, ActionMoveTo, send.MoveTo{X: 1, Y: 2}); err != nil {
		t.Fatalf("Send error: %v", err)
	}

	if wireOut == nil {
		t.Fatal("WireOutbound not received")
	}
	if semOut == nil {
		t.Fatal("SemanticOut not received")
	}

	if len(wireOut.Frame) == 0 || len(semOut.Frame) == 0 {
		t.Fatal("frames are empty")
	}
	if &wireOut.Frame[0] != &semOut.Frame[0] {
		t.Error("WireOutbound.Frame and SemanticOut.Frame are NOT the same slice — must share backing array")
	}
}

// TestTraceFunc_WireOutbound_FrameIsSafeToRetain verifies that the frame from
// WireOutbound is safe to retain after subsequent Send calls.
func TestTraceFunc_WireOutbound_FrameIsSafeToRetain(t *testing.T) {
	s := NewMapSession(20181002)

	var retained []byte
	s.SetTraceFunc(func(e TraceEvent) {
		if w, ok := e.(WireOutbound); ok && retained == nil {
			retained = w.Frame
		}
	})

	var buf bytes.Buffer
	if err := Send(s, &buf, ActionMoveTo, send.MoveTo{X: 1, Y: 2}); err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if len(retained) == 0 {
		t.Fatal("no frame retained")
	}
	original := append([]byte(nil), retained...)

	// More sends — retained frame must not change.
	for i := 0; i < 5; i++ {
		buf.Reset()
		_ = Send(s, &buf, ActionMoveTo, send.MoveTo{X: uint16(i), Y: uint16(i)})
	}

	if !bytes.Equal(retained, original) {
		t.Errorf("retained WireOutbound.Frame changed after more Sends: was %x, now %x", original, retained)
	}
}

// TestTraceFunc_Send_NoTraceWhenNil verifies that Send works correctly and does
// not panic when TraceFunc is nil.
func TestTraceFunc_Send_NoTraceWhenNil(t *testing.T) {
	s := NewMapSession(20181002)
	// TraceFunc intentionally not set (nil).

	var buf bytes.Buffer
	if err := Send(s, &buf, ActionMoveTo, send.MoveTo{X: 10, Y: 20}); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("Send produced no output")
	}
}

// ─── IsFaulted tests ─────────────────────────────────────────────────────────

// TestIsFaulted_FalseInitially verifies that a new MapSession is not faulted.
func TestIsFaulted_FalseInitially(t *testing.T) {
	s := NewMapSession(20181002)
	if s.IsFaulted() {
		t.Error("IsFaulted() = true on new session, want false")
	}
}

// TestIsFaulted_TrueAfterCorruptEmbeddedLength verifies that IsFaulted() returns
// true after Feed() receives a variable-length packet with embedded length < 4.
func TestIsFaulted_TrueAfterCorruptEmbeddedLength(t *testing.T) {
	// Packet 0x09FF is variable-length at pv=20141023.
	s := NewMapSession(20141023)

	frame := makeVarFrameEmbedLen(0x09FF, 10, 2) // embedded length = 2 < 4
	err := s.Feed(frame)
	if err == nil {
		t.Fatal("Feed returned nil for corrupt embedded length, want error")
	}
	if !s.IsFaulted() {
		t.Error("IsFaulted() = false after corrupt embedded length, want true")
	}
}

// TestIsFaulted_FeedSilentAfterFault verifies that after a fault, subsequent
// Feed calls return nil and IsFaulted() stays true.
func TestIsFaulted_FeedSilentAfterFault(t *testing.T) {
	s := NewMapSession(20141023)

	frame := makeVarFrameEmbedLen(0x09FF, 10, 2)
	_ = s.Feed(frame)

	if !s.IsFaulted() {
		t.Fatal("prerequisite: session not faulted after corrupt frame")
	}

	// Subsequent Feeds must return nil.
	for i := 0; i < 3; i++ {
		if err := s.Feed(makeVarFrame(0x0069, 8)); err != nil {
			t.Errorf("Feed after fault returned error: %v", err)
		}
		if !s.IsFaulted() {
			t.Error("IsFaulted() became false after subsequent Feed")
		}
	}
}

// ─── UnhandledPackets tests ───────────────────────────────────────────────────

// TestUnhandledPackets_ZeroInitially verifies that a new session has 0 unhandled packets.
func TestUnhandledPackets_ZeroInitially(t *testing.T) {
	s := NewMapSession(20181002)
	if s.UnhandledPackets() != 0 {
		t.Errorf("UnhandledPackets() = %d on new session, want 0", s.UnhandledPackets())
	}
}

// TestUnhandledPackets_IncrementWhenNoHandler verifies that feeding 3 packets
// with no handler increments UnhandledPackets() to 3.
func TestUnhandledPackets_IncrementWhenNoHandler(t *testing.T) {
	s := NewMapSession(20181002)
	// 0x0080 is in the lengths table (7 bytes) but has no registered handler.
	frame := make([]byte, 7)
	frame[0] = 0x80
	frame[1] = 0x00

	for i := 0; i < 3; i++ {
		if err := s.Feed(frame); err != nil {
			t.Fatalf("Feed[%d] error: %v", i, err)
		}
	}

	if s.UnhandledPackets() != 3 {
		t.Errorf("UnhandledPackets() = %d, want 3", s.UnhandledPackets())
	}
}

// TestUnhandledPackets_NotIncrementedWhenHandlerPresent verifies that packets
// with a registered handler do NOT increment UnhandledPackets().
func TestUnhandledPackets_NotIncrementedWhenHandlerPresent(t *testing.T) {
	s := NewMapSession(20181002)
	s.registerHandler(0x0080, func(data []byte, pv uint32) {})

	frame := make([]byte, 7)
	frame[0] = 0x80
	frame[1] = 0x00
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed error: %v", err)
	}

	if s.UnhandledPackets() != 0 {
		t.Errorf("UnhandledPackets() = %d, want 0 (handler was registered)", s.UnhandledPackets())
	}
}

// TestUnhandledPackets_NotIncrementedForUnknownPacketID verifies that unknown
// packet IDs (not in the lengths table) do NOT increment UnhandledPackets().
// Those are separate — they trigger UnknownPacketEvent instead.
func TestUnhandledPackets_NotIncrementedForUnknownPacketID(t *testing.T) {
	s := NewMapSession(20181002)

	if err := s.Feed(makeFrame(0xFFFF, 2)); err != nil {
		t.Fatalf("Feed error: %v", err)
	}

	if s.UnhandledPackets() != 0 {
		t.Errorf("UnhandledPackets() = %d for unknown packet ID, want 0", s.UnhandledPackets())
	}
}

// ─── ErrWrongSendType tests ───────────────────────────────────────────────────

// TestErrWrongSendType_IncludesActionName verifies that the error message from
// Send with the wrong type includes the action name string (e.g. "ActionMoveTo").
func TestErrWrongSendType_IncludesActionName(t *testing.T) {
	s := NewMapSession(20181002)
	var buf bytes.Buffer
	err := Send(s, &buf, ActionMoveTo, send.PublicChat{Name: "bad"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !containsString(msg, "ActionMoveTo") {
		t.Errorf("error message %q does not contain action name 'ActionMoveTo'", msg)
	}
}

// TestErrWrongSendType_ErrorsIsMatch verifies that errors.Is(err, ErrWrongSendType{})
// returns true for a wrong-type error.
func TestErrWrongSendType_ErrorsIsMatch(t *testing.T) {
	s := NewMapSession(20181002)
	var buf bytes.Buffer
	err := Send(s, &buf, ActionMoveTo, send.PublicChat{Name: "bad"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrWrongSendType{}) {
		t.Errorf("errors.Is(err, ErrWrongSendType{}) = false, want true; err = %v", err)
	}
}

// TestErrWrongSendType_ErrorsIsMatchAnyAction verifies that ErrWrongSendType{Action: ActionMoveTo}
// matches ErrWrongSendType{} (zero value sentinel via Is).
func TestErrWrongSendType_ErrorsIsMatchAnyAction(t *testing.T) {
	specific := ErrWrongSendType{Action: ActionMoveTo}
	zero := ErrWrongSendType{}
	if !errors.Is(specific, zero) {
		t.Error("ErrWrongSendType{Action: ActionMoveTo} should match ErrWrongSendType{} via errors.Is")
	}
}

// TestErrWrongSendType_NotMatchOtherErrors verifies that errors.Is does not match
// unrelated errors as ErrWrongSendType.
func TestErrWrongSendType_NotMatchOtherErrors(t *testing.T) {
	other := fmt.Errorf("some other error")
	if errors.Is(other, ErrWrongSendType{}) {
		t.Error("errors.Is(fmt.Errorf(...), ErrWrongSendType{}) should be false")
	}
}

// ─── Feature 5 explicit tests: SetUnknownPacketHandler fires even without TraceFunc ──

// TestUnknownPacketHandler_FiresWithoutTraceFunc is already covered by
// TestTraceFunc_UnknownPacket_OnlyUnknownPacketHandlerWired above.
// No duplicate needed.

// ─── Feature 6: SetTraceFunc on LoginSession and CharSession ──────────────────

// TestLoginSession_SetTraceFunc_WireInbound verifies that LoginSession.SetTraceFunc
// fires WireInbound for a known packet.
func TestLoginSession_SetTraceFunc_WireInbound(t *testing.T) {
	s := NewLoginSession(20181002)

	var got []TraceEvent
	s.SetTraceFunc(func(e TraceEvent) { got = append(got, e) })

	const testID uint16 = 0x0069
	s.setLength(testID, -1)
	s.registerHandler(testID, func(data []byte, pv uint32) {})

	if err := s.Feed(makeVarFrame(testID, 8)); err != nil {
		t.Fatalf("Feed error: %v", err)
	}

	found := false
	for _, e := range got {
		if w, ok := e.(WireInbound); ok && w.ID == testID {
			found = true
		}
	}
	if !found {
		t.Error("WireInbound not fired on LoginSession with SetTraceFunc")
	}
}

// TestLoginSession_SetTraceFunc_UnknownPacket verifies that LoginSession.SetTraceFunc
// fires UnknownPacketEvent for an unknown packet ID.
func TestLoginSession_SetTraceFunc_UnknownPacket(t *testing.T) {
	s := NewLoginSession(20181002)

	var got []TraceEvent
	s.SetTraceFunc(func(e TraceEvent) { got = append(got, e) })

	if err := s.Feed(makeFrame(0xFFFF, 2)); err != nil {
		t.Fatalf("Feed error: %v", err)
	}

	found := false
	for _, e := range got {
		if u, ok := e.(UnknownPacketEvent); ok && u.ID == 0xFFFF {
			found = true
		}
	}
	if !found {
		t.Error("UnknownPacketEvent not fired via TraceFunc on LoginSession")
	}
}

// TestCharSession_SetTraceFunc_WireInbound verifies that CharSession.SetTraceFunc
// fires WireInbound for a known packet.
func TestCharSession_SetTraceFunc_WireInbound(t *testing.T) {
	s := NewCharSession(20181002)

	var got []TraceEvent
	s.SetTraceFunc(func(e TraceEvent) { got = append(got, e) })

	const testID uint16 = 0x006B
	s.setLength(testID, -1)
	s.registerHandler(testID, func(data []byte, pv uint32) {})

	if err := s.Feed(makeVarFrame(testID, 8)); err != nil {
		t.Fatalf("Feed error: %v", err)
	}

	found := false
	for _, e := range got {
		if w, ok := e.(WireInbound); ok && w.ID == testID {
			found = true
		}
	}
	if !found {
		t.Error("WireInbound not fired on CharSession with SetTraceFunc")
	}
}

// TestCharSession_SetTraceFunc_UnknownPacket verifies that CharSession.SetTraceFunc
// fires UnknownPacketEvent for an unknown packet ID.
func TestCharSession_SetTraceFunc_UnknownPacket(t *testing.T) {
	s := NewCharSession(20181002)

	var got []TraceEvent
	s.SetTraceFunc(func(e TraceEvent) { got = append(got, e) })

	if err := s.Feed(makeFrame(0xFFFF, 2)); err != nil {
		t.Fatalf("Feed error: %v", err)
	}

	found := false
	for _, e := range got {
		if u, ok := e.(UnknownPacketEvent); ok && u.ID == 0xFFFF {
			found = true
		}
	}
	if !found {
		t.Error("UnknownPacketEvent not fired via TraceFunc on CharSession")
	}
}

// ─── Benchmarks ───────────────────────────────────────────────────────────────

// BenchmarkFeed_WithNilTrace benchmarks Feed with no TraceFunc set.
// Must show 0 allocs/op (baseline unchanged).
func BenchmarkFeed_WithNilTrace(b *testing.B) {
	s := NewMapSession(20181002)
	s.registerHandler(0x0080, func(data []byte, pv uint32) {})

	frame := make([]byte, 7)
	frame[0] = 0x80
	frame[1] = 0x00

	_ = s.Feed(frame) // prime

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := s.Feed(frame); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFeed_WithTraceFunc benchmarks Feed with a TraceFunc set.
// Expected: 1 alloc/op for the frame copy.
func BenchmarkFeed_WithTraceFunc(b *testing.B) {
	s := NewMapSession(20181002)
	s.registerHandler(0x0080, func(data []byte, pv uint32) {})
	s.SetTraceFunc(func(e TraceEvent) { _ = e })

	frame := make([]byte, 7)
	frame[0] = 0x80
	frame[1] = 0x00

	_ = s.Feed(frame) // prime

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := s.Feed(frame); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSend_WithNilTrace benchmarks Send with no TraceFunc set.
// Must show 0 allocs/op for fixed-size packet (baseline unchanged).
func BenchmarkSend_WithNilTrace(b *testing.B) {
	s := NewMapSession(20181002)
	// TraceFunc intentionally nil.

	var buf bytes.Buffer
	req := send.MoveTo{X: 100, Y: 200}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := Send(s, &buf, ActionMoveTo, req); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSend_WithTraceFunc benchmarks Send with TraceFunc set.
// Expected: 1-2 allocs/op for frame copy.
func BenchmarkSend_WithTraceFunc(b *testing.B) {
	s := NewMapSession(20181002)
	s.SetTraceFunc(func(e TraceEvent) { _ = e })

	var buf bytes.Buffer
	req := send.MoveTo{X: 100, Y: 200}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := Send(s, &buf, ActionMoveTo, req); err != nil {
			b.Fatal(err)
		}
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func containsString(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
