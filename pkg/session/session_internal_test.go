// Package session — internal tests that require access to unexported fields.
package session

import (
	"encoding/binary"
	"testing"
	"unsafe"
)

// bufStartPtr returns the address of the first element of sl's backing array,
// or 0 if the slice has zero capacity.
func bufStartPtr(sl []byte) uintptr {
	if cap(sl) == 0 {
		return 0
	}
	return uintptr(unsafe.Pointer(unsafe.SliceData(sl)))
}

// TestFeed_CopyToFront_PartialFrames verifies that copy-to-front runs
// unconditionally (not only when consumed > 0).
//
// The invariant: after any Feed() call where recvBuf is non-empty, recvBuf[0]
// must equal buf[0]. This ensures the partial-frame accumulation region always
// starts at the beginning of buf rather than drifting forward each time a frame
// is consumed.
//
// Test scenario:
//  1. Feed a complete 7-byte fixed-length frame (0x0080).
//     consumed=7 → copy-to-front runs → recvBuf = buf[0:0].
//  2. Feed the SAME frame again immediately, all 7 bytes as one Feed call.
//     After consuming the second frame: recvBuf = buf[0:0] again (copy-to-front).
//  3. Feed 3 partial bytes one at a time (consumed==0 for each).
//     With the fix: copy-to-front runs after each, recvBuf stays anchored at buf[0].
//     Without the fix: copy-to-front is skipped — recvBuf stays at buf[0] here too
//     because append in-place is anchored to buf[0]. The REAL problem only manifests
//     after enough data to overflow buf capacity.
//
// To force the issue without feeding 65536+ bytes, we simulate the drift by
// directly manipulating recvBuf to start at a non-zero offset in buf (as would
// happen after frame consumption without copy-to-front) and then feed a partial byte.
// Without the fix: consumed==0 → copy-to-front skipped → recvBuf stays at offset.
// With the fix: copy-to-front moves data to buf[0] → recvBuf anchored.
func TestFeed_CopyToFront_PartialFrames(t *testing.T) {
	s := NewMapSession(20181121)
	dispatched := 0
	s.registerHandler(0x0080, func(data []byte, pv uint32) { dispatched++ })

	frame := make([]byte, 7)
	binary.LittleEndian.PutUint16(frame[0:2], 0x0080)

	// Warm-up: feed complete frame so recvBuf = buf[0:0].
	if err := s.Feed(frame); err != nil {
		t.Fatalf("warm-up: %v", err)
	}
	if dispatched != 1 {
		t.Fatalf("warm-up: dispatched %d, want 1", dispatched)
	}
	dispatched = 0

	bufBase := bufStartPtr(s.core.buf)

	// Directly set recvBuf to start mid-buf (simulating a post-consume drift
	// that did NOT get copy-to-front-ed). We put 3 "stale" bytes at offset 7
	// in buf — as if a previous frame was consumed advancing the slice but
	// copy-to-front was skipped.
	//
	// Feed a partial byte: the framing loop cannot complete a 7-byte frame from
	// 4 bytes, so consumed==0. With fix: copy-to-front resets recvBuf to buf[0:4].
	// Without fix: recvBuf stays at buf[7:11] — pointer != buf[0].
	s.core.buf[7] = 0x80 // simulated partial 0x0080 frame
	s.core.buf[8] = 0x00
	s.core.buf[9] = 0x00
	s.core.recvBuf = s.core.buf[7:10] // 3 bytes at offset 7

	if err := s.Feed([]byte{0x00}); err != nil { // 4th byte, still partial
		t.Fatalf("partial feed: %v", err)
	}

	if len(s.core.recvBuf) == 0 {
		t.Fatal("recvBuf empty after partial feed")
	}
	gotPtr := bufStartPtr(s.core.recvBuf)
	if gotPtr != bufBase {
		t.Errorf("recvBuf[0] at %x, want buf[0] at %x — copy-to-front skipped on consumed==0",
			gotPtr, bufBase)
	}

	// Verify steady-state 0 allocations.
	dispatched = 0
	// Reset to known state.
	_ = s.Feed(frame[3:]) // complete the partial frame (bytes 4-6)
	// Now feed complete frames to get to steady state.
	for i := 0; i < 3; i++ {
		_ = s.Feed(frame)
	}
	dispatched = 0

	allocs := testing.AllocsPerRun(100, func() {
		_ = s.Feed(frame)
	})
	if allocs != 0 {
		t.Errorf("steady-state Feed allocates %.0f objects/call, want 0", allocs)
	}
}
