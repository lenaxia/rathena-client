// Package session provides PACKETVER-aware packet framing and dispatch for
// the three rAthena server types: login, char, and map.
//
// All three session types share a common framing engine (sessionCore) that reads
// little-endian 2-byte packet IDs, looks up frame lengths from a generated table,
// and dispatches to registered handler functions.
//
// Design invariants:
//   - Zero goroutines: Feed() is synchronous; callbacks fire in the caller's goroutine.
//   - Zero heap allocations in steady state: the backing receive buffer is reused
//     across Feed() calls via a copy-to-front strategy.
//   - Not goroutine-safe by design: callers must serialize access to each session.
package session

import (
	"encoding/binary"
	"fmt"
)

// HandlerFunc is a callback invoked synchronously by Feed() for each decoded frame.
// data is the complete frame bytes including the 2-byte packet ID header.
// packetver is the PACKETVER the session was constructed with.
//
// IMPORTANT: string fields in decoded events (e.g. event.Name) are zero-copy aliases
// into the session receive buffer. They are valid only for the duration of this callback.
// Do NOT store them past the return of HandlerFunc. To retain a string, copy it first:
//
//	name = decode.CopyString(event.Name)
type HandlerFunc func(data []byte, packetver uint32)

// ErrUnknownPacket is returned by Feed() when an unrecognised packet ID is
// encountered. The TCP stream is now irrecoverably desynced; the caller must
// close the connection.
//
// After ErrUnknownPacket is returned once, the session is marked faulted and
// all subsequent Feed() calls are silent no-ops returning nil. This is
// intentional: the caller should close the connection immediately on the first
// ErrUnknownPacket and not call Feed() again. Subsequent nil returns prevent
// error spam during connection teardown.
type ErrUnknownPacket struct {
	ID uint16
}

func (e ErrUnknownPacket) Error() string {
	return fmt.Sprintf("session: unknown packet ID %#04x — stream desynced", e.ID)
}

// sessionCore is the internal framing engine shared by all three session types.
type sessionCore struct {
	packetver uint32
	buf       []byte       // full backing array; owned exclusively by sessionCore
	recvBuf   []byte       // active sub-slice of buf; advances as frames are consumed
	lengths   [65536]int16 // packet length table: 0 = unknown, -1 = variable (bytes[2:4])
	handlers  [65536]HandlerFunc
	faulted   bool
}

// feed implements the core framing and dispatch loop.
//
// Algorithm (source: HLD §9):
//  1. Append data to recvBuf without allocating if capacity permits.
//  2. Loop while recvBuf contains a complete frame:
//     a. Read packetID = leU16(recvBuf[0:2]).
//     b. Look up frameLen in lengths[packetID].
//     If -1: read frameLen from recvBuf[2:4] (variable-length packet).
//     If 0:  unknown packet; set faulted = true; return ErrUnknownPacket.
//     c. If len(recvBuf) < frameLen: incomplete frame; break.
//     d. Dispatch: call handlers[packetID](recvBuf[:frameLen], packetver).
//     e. Advance: recvBuf = recvBuf[frameLen:].
//  3. Copy unconsumed bytes to front of buf to prevent unbounded backing-array growth.
func (c *sessionCore) feed(data []byte) error {
	if c.faulted {
		return nil
	}

	// Step 1: append data to recvBuf.
	c.recvBuf = append(c.recvBuf, data...)

	for len(c.recvBuf) >= 2 {
		// Step 2a: read packet ID.
		packetID := binary.LittleEndian.Uint16(c.recvBuf[:2])

		// Step 2b: determine frame length.
		frameLen := int(c.lengths[packetID])
		switch {
		case frameLen == -1:
			// Variable-length: bytes [2:4] carry the total frame length.
			if len(c.recvBuf) < 4 {
				goto done // not enough bytes to read the length field yet
			}
			frameLen = int(binary.LittleEndian.Uint16(c.recvBuf[2:4]))
			if frameLen < 4 {
				c.faulted = true
				return ErrUnknownPacket{ID: packetID}
			}
		case frameLen == 0:
			// Unknown packet: stream is desynced.
			c.faulted = true
			return ErrUnknownPacket{ID: packetID}
		}

		// Step 2c: wait for a complete frame.
		if len(c.recvBuf) < frameLen {
			break
		}

		// Step 2d: dispatch.
		if fn := c.handlers[packetID]; fn != nil {
			fn(c.recvBuf[:frameLen], c.packetver)
		}

		// Step 2e: advance.
		c.recvBuf = c.recvBuf[frameLen:]
	}

done:
	// Step 3: copy-to-front to prevent the backing array from being abandoned.
	n := copy(c.buf, c.recvBuf)
	c.recvBuf = c.buf[:n]
	return nil
}

// registerHandler registers fn as the handler for the given packet ID.
// A second call with the same ID overwrites the previous registration.
func (c *sessionCore) registerHandler(id uint16, fn HandlerFunc) {
	c.handlers[id] = fn
}
