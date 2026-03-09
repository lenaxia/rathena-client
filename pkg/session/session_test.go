// Package session implements PACKETVER-aware network session framing and
// packet dispatch for the three rAthena server types.
package session_test

import (
	"errors"
	"testing"

	"github.com/lenaxia/ragnarok-go-client/pkg/session"
)

// makeFrame constructs a minimal fixed-length packet frame:
// bytes [0:2] = packetID (little-endian), bytes [2:n] = payload.
func makeFrame(packetID uint16, size int) []byte {
	b := make([]byte, size)
	b[0] = byte(packetID)
	b[1] = byte(packetID >> 8)
	return b
}

// makeVarFrame constructs a variable-length packet frame:
// bytes [0:2] = packetID, bytes [2:4] = total length (little-endian), bytes [4:] = payload.
func makeVarFrame(packetID uint16, totalLen int) []byte {
	b := make([]byte, totalLen)
	b[0] = byte(packetID)
	b[1] = byte(packetID >> 8)
	b[2] = byte(totalLen)
	b[3] = byte(totalLen >> 8)
	return b
}

// TestMapSession_Feed_DispatchesRegisteredHandler verifies that a registered
// On* callback fires once for a complete packet frame.
func TestMapSession_Feed_DispatchesRegisteredHandler(t *testing.T) {
	s := session.NewMapSession(20181002)

	called := 0
	var gotData []byte
	s.RegisterHandler(0x0069, func(data []byte, pv uint32) {
		called++
		gotData = append([]byte(nil), data...)
	})

	frame := makeVarFrame(0x0069, 12)
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed returned error: %v", err)
	}
	if called != 1 {
		t.Fatalf("handler called %d times, want 1", called)
	}
	if len(gotData) != 12 {
		t.Errorf("handler got %d bytes, want 12", len(gotData))
	}
}

// TestMapSession_Feed_AccumulatesPartialFrames verifies that a frame split
// across two Feed calls is dispatched only once the full frame arrives.
func TestMapSession_Feed_AccumulatesPartialFrames(t *testing.T) {
	s := session.NewMapSession(20181002)

	called := 0
	s.RegisterHandler(0x0069, func(data []byte, pv uint32) { called++ })

	frame := makeVarFrame(0x0069, 10)
	// Feed first 5 bytes — incomplete frame.
	if err := s.Feed(frame[:5]); err != nil {
		t.Fatalf("Feed(partial) returned error: %v", err)
	}
	if called != 0 {
		t.Fatalf("handler fired early on partial frame")
	}
	// Feed remaining 5 bytes — now complete.
	if err := s.Feed(frame[5:]); err != nil {
		t.Fatalf("Feed(remaining) returned error: %v", err)
	}
	if called != 1 {
		t.Fatalf("handler called %d times after full frame, want 1", called)
	}
}

// TestMapSession_Feed_MultipleFramesInOneBurst verifies that multiple complete
// frames in a single Feed call are all dispatched.
func TestMapSession_Feed_MultipleFramesInOneBurst(t *testing.T) {
	s := session.NewMapSession(20181002)

	var counts [3]int
	s.RegisterHandler(0x0069, func(data []byte, pv uint32) { counts[0]++ })
	s.RegisterHandler(0x006B, func(data []byte, pv uint32) { counts[1]++ })
	s.RegisterHandler(0x006B, func(data []byte, pv uint32) { counts[2]++ }) // overwrites previous

	// Build two back-to-back variable-length frames.
	f1 := makeVarFrame(0x0069, 8)
	f2 := makeVarFrame(0x006B, 10)
	burst := append(f1, f2...)

	if err := s.Feed(burst); err != nil {
		t.Fatalf("Feed(burst) returned error: %v", err)
	}
	if counts[0] != 1 {
		t.Errorf("0x0069 handler called %d times, want 1", counts[0])
	}
	if counts[2] != 1 {
		t.Errorf("0x006B handler called %d times, want 1", counts[2])
	}
}

// TestMapSession_Feed_UnknownPacket verifies that an unknown packet ID causes
// ErrUnknownPacket and subsequent Feed calls are no-ops.
func TestMapSession_Feed_UnknownPacket(t *testing.T) {
	s := session.NewMapSession(20181002)

	frame := makeFrame(0xFFFF, 4) // 0xFFFF is guaranteed not in lengths table
	err := s.Feed(frame)
	if err == nil {
		t.Fatal("Feed returned nil, want ErrUnknownPacket")
	}
	var e session.ErrUnknownPacket
	if !errors.As(err, &e) {
		t.Fatalf("error is %T, want ErrUnknownPacket", err)
	}
	if e.ID != 0xFFFF {
		t.Errorf("ErrUnknownPacket.ID = %#04x, want 0xFFFF", e.ID)
	}

	// After fault, subsequent Feed calls must be no-ops (return nil, no dispatch).
	called := 0
	s.RegisterHandler(0x0069, func(data []byte, pv uint32) { called++ })
	frame2 := makeVarFrame(0x0069, 8)
	if err := s.Feed(frame2); err != nil {
		t.Errorf("post-fault Feed returned %v, want nil", err)
	}
	if called != 0 {
		t.Errorf("post-fault handler called %d times, want 0", called)
	}
}

// TestMapSession_Feed_NoHandlerOK verifies that a known packet with no registered
// handler is silently consumed (no error, no panic).
func TestMapSession_Feed_NoHandlerOK(t *testing.T) {
	s := session.NewMapSession(20181002)
	// 0x0069 is in the lengths table (variable-length) but has no handler.
	frame := makeVarFrame(0x0069, 8)
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed returned error on unhandled known packet: %v", err)
	}
}

// TestMapSession_Feed_VariableLengthFrame verifies that a variable-length frame
// (lengths[id] == -1) uses bytes [2:4] as the frame length.
func TestMapSession_Feed_VariableLengthFrame(t *testing.T) {
	s := session.NewMapSession(20181002)

	called := 0
	var gotLen int
	s.RegisterHandler(0x0069, func(data []byte, pv uint32) {
		called++
		gotLen = len(data)
	})

	const totalLen = 47
	frame := makeVarFrame(0x0069, totalLen)
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed returned error: %v", err)
	}
	if called != 1 {
		t.Fatalf("handler called %d times, want 1", called)
	}
	if gotLen != totalLen {
		t.Errorf("handler got %d bytes, want %d", gotLen, totalLen)
	}
}

// TestLoginSession_Feed_Dispatch verifies basic dispatching for LoginSession.
// LoginSession's lengths table is empty until common/packets.hpp is in the
// codegen pipeline; SetLength is used here to register a test packet.
func TestLoginSession_Feed_Dispatch(t *testing.T) {
	s := session.NewLoginSession(20181002)

	const testID uint16 = 0x0069
	s.SetLength(testID, -1) // variable-length

	called := 0
	s.RegisterHandler(testID, func(data []byte, pv uint32) { called++ })
	frame := makeVarFrame(testID, 8)
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed returned error: %v", err)
	}
	if called != 1 {
		t.Fatalf("handler called %d times, want 1", called)
	}
}

// TestCharSession_Feed_Dispatch verifies basic dispatching for CharSession.
// CharSession's lengths table is empty until common/packets.hpp is in the
// codegen pipeline; SetLength is used here to register a test packet.
func TestCharSession_Feed_Dispatch(t *testing.T) {
	s := session.NewCharSession(20181002)

	const testID uint16 = 0x006B
	s.SetLength(testID, -1) // variable-length

	called := 0
	s.RegisterHandler(testID, func(data []byte, pv uint32) { called++ })
	frame := makeVarFrame(testID, 8)
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed returned error: %v", err)
	}
	if called != 1 {
		t.Fatalf("handler called %d times, want 1", called)
	}
}

// TestMapSession_Encode_NoObfuscation verifies that Encode is a no-op when
// obfuscation is not enabled.
func TestMapSession_Encode_NoObfuscation(t *testing.T) {
	s := session.NewMapSession(20181002)
	var id uint16 = 0x0085
	s.Encode(&id)
	if id != 0x0085 {
		t.Errorf("Encode without obfuscation changed packetID to %#04x, want 0x0085", id)
	}
}

// TestMapSession_Encode_Obfuscation verifies the first and second encoded packet
// IDs match the expected obfuscation formula.
//
// Test uses keys from clif_obfuscation.hpp at PACKETVER=20180307 (last real version).
// Source: src/map/clif_obfuscation.hpp, processed with -DPACKET_OBFUSCATION.
func TestMapSession_Encode_Obfuscation(t *testing.T) {
	// Lookup known keys for a packetver that has obfuscation.
	k0, k1, k2 := session.ObfuscationKeysFor(20180307)
	if k0 == 0 && k1 == 0 && k2 == 0 {
		t.Skip("no obfuscation keys found for 20180307 — skipping obfuscation test")
	}

	s := session.NewMapSession(20180307)
	s.EnableObfuscation(k0, k1, k2)

	// First packet: rawID XOR firstKey where firstKey = ((k0*k1+k2)>>16)&0x7FFF
	firstKey := uint16(((uint64(k0)*uint64(k1) + uint64(k2)) >> 16) & 0x7FFF)

	var rawID uint16 = 0x0436
	id := rawID
	s.Encode(&id)
	if id != rawID^firstKey {
		t.Errorf("first Encode: got %#04x, want %#04x (raw=%#04x firstKey=%#04x)",
			id, rawID^firstKey, rawID, firstKey)
	}

	// Second packet: rawID XOR ((rollingKey>>16)&0x7FFF)
	// rollingKey = ((k0*k1+k2)&0xFFFFFFFF)*k1+k2)&0xFFFFFFFF
	step1 := (uint64(k0)*uint64(k1) + uint64(k2)) & 0xFFFFFFFF
	rollingKey := uint32((step1*uint64(k1) + uint64(k2)) & 0xFFFFFFFF)
	rollKey := uint16((rollingKey >> 16) & 0x7FFF)

	var rawID2 uint16 = 0x007D
	id2 := rawID2
	s.Encode(&id2)
	if id2 != rawID2^rollKey {
		t.Errorf("second Encode: got %#04x, want %#04x (raw=%#04x rollKey=%#04x)",
			id2, rawID2^rollKey, rawID2, rollKey)
	}
}

// TestMapSession_Encode_Obfuscation_Oracle verifies the obfuscation output against
// concrete expected values derived directly from rAthena source keys.
//
// Source: src/map/clif_obfuscation.hpp at PACKETVER=20180307:
//
//	packet_keys(0x47DA10EB, 0x4B922CCF, 0x765C5055)
//
// Manually computed:
//
//	step1     = (0x47DA10EB * 0x4B922CCF + 0x765C5055) & 0xFFFFFFFF = 0x899E625A
//	firstKey  = (step1 >> 16) & 0x7FFF = 0x099E
//	rolling1  = (step1 * 0x4B922CCF + 0x765C5055) & 0xFFFFFFFF = 0x6BA94F1B
//	rollKey1  = (rolling1 >> 16) & 0x7FFF = 0x6BA9
//
// Expected encoded IDs:
//
//	0x0436 ^ 0x099E = 0x0DA8  (first C→S packet: CZ_ENTER2)
//	0x007D ^ 0x6BA9 = 0x6BD4  (second C→S packet: CZ_NOTIFY_ACTORINIT)
//
// These values are not derived from ObfuscationKeysFor — they are an external oracle.
func TestMapSession_Encode_Obfuscation_Oracle(t *testing.T) {
	// Keys from rAthena clif_obfuscation.hpp PACKETVER=20180307 (hardcoded oracle)
	const k0 = uint32(0x47DA10EB)
	const k1 = uint32(0x4B922CCF)
	const k2 = uint32(0x765C5055)

	// Expected encoded values computed independently from rAthena formula
	const wantFirst = uint16(0x0DA8)  // 0x0436 ^ 0x099E
	const wantSecond = uint16(0x6BD4) // 0x007D ^ 0x6BA9

	s := session.NewMapSession(20180307)
	s.EnableObfuscation(k0, k1, k2)

	var id1 uint16 = 0x0436
	s.Encode(&id1)
	if id1 != wantFirst {
		t.Errorf("first Encode(0x0436): got %#04x, want %#04x", id1, wantFirst)
	}

	var id2 uint16 = 0x007D
	s.Encode(&id2)
	if id2 != wantSecond {
		t.Errorf("second Encode(0x007D): got %#04x, want %#04x", id2, wantSecond)
	}
}
func TestErrUnknownPacket_Error(t *testing.T) {
	e := session.ErrUnknownPacket{ID: 0x1234}
	msg := e.Error()
	if msg == "" {
		t.Error("ErrUnknownPacket.Error() returned empty string")
	}
}

// TestMapSession_Feed_ZeroAlloc is a benchmark-style test that verifies the
// Feed hot path does not allocate on the heap in steady state.
// (Real alloc measurement is done in session_bench_test.go.)
func TestMapSession_Feed_ZeroAlloc(t *testing.T) {
	s := session.NewMapSession(20181002)
	s.RegisterHandler(0x0069, func(data []byte, pv uint32) {})

	// Prime: run once to warm up recvBuf backing array.
	frame := makeVarFrame(0x0069, 20)
	if err := s.Feed(frame); err != nil {
		t.Fatalf("warm-up Feed error: %v", err)
	}

	// Measure: expect 0 allocs.
	allocs := testing.AllocsPerRun(100, func() {
		if err := s.Feed(frame); err != nil {
			t.Errorf("Feed error in alloc test: %v", err)
		}
	})
	if allocs != 0 {
		t.Errorf("Feed allocates %.0f heap objects per call in steady state, want 0", allocs)
	}
}
