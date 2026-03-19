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
	"time"
)

// TraceEvent is the interface implemented by all observable session events.
// The concrete types are WireInbound, WireOutbound, SemanticIn, SemanticOut,
// and UnknownPacketEvent.
type TraceEvent interface{ traceEvent() }

// WireInbound is emitted by feed() for every complete inbound frame whose packet
// ID is found in the length table (known packet ID). Frame is a heap-allocated
// copy safe to retain beyond the TraceFunc call. Fired before dispatch.
type WireInbound struct {
	ID        uint16
	Frame     []byte
	Packetver uint32
	Time      time.Time
}

func (WireInbound) traceEvent() {}

// WireOutbound is emitted by Send() after a successful write. Frame is a
// heap-allocated copy of the post-obfuscation wire bytes, safe to retain.
type WireOutbound struct {
	Action    SemanticAction
	Frame     []byte
	Packetver uint32
	Time      time.Time
}

func (WireOutbound) traceEvent() {}

// SemanticIn is emitted inside the RegisterSemanticHandler closure after the
// decode function runs and before the user handler is called. Frame is the same
// heap-allocated copy as the paired WireInbound (an independent copy).
type SemanticIn struct {
	Action SemanticAction
	ID     uint16
	Event  interface{}
	Frame  []byte
}

func (SemanticIn) traceEvent() {}

// SemanticOut is emitted by Send() after a successful write. Frame is the same
// heap-allocated slice as the paired WireOutbound.Frame.
type SemanticOut struct {
	Action  SemanticAction
	Request interface{}
	Frame   []byte
}

func (SemanticOut) traceEvent() {}

// TraceFunc is the unified trace hook. Set via SetTraceFunc on any session type.
// Called synchronously in the caller's goroutine. Must not block.
type TraceFunc func(TraceEvent)

// handlerFunc is a callback invoked synchronously by Feed() for each decoded frame.
// data is the complete frame bytes including the 2-byte packet ID header.
// packetver is the PACKETVER the session was constructed with.
//
// IMPORTANT: string fields in decoded events (e.g. event.Name) are zero-copy aliases
// into the session receive buffer. They are valid only for the duration of this callback.
// Do NOT store them past the return of handlerFunc. To retain a string, copy it first:
//
//	name = decode.CopyString(event.Name)
type handlerFunc func(data []byte, packetver uint32)

func (UnknownPacketEvent) traceEvent() {}

// recentPacketDepth is the number of preceding dispatched packets captured in
// the ring buffer and included in UnknownPacketEvent.RecentPackets.
const recentPacketDepth = 3

// DispatchedPacket is a record of a single successfully dispatched packet,
// stored in the session's recent-packet ring buffer for diagnostic purposes.
//
// If the frame exceeded recentMaxFrameBytes, Frame contains a truncated prefix
// and Truncated is true; FrameTotal gives the actual frame length.
type DispatchedPacket struct {
	ID         uint16 // packet ID
	Frame      []byte // heap-allocated copy of the frame bytes (may be truncated)
	FrameTotal int    // actual frame length in bytes
	Truncated  bool   // true if Frame is a truncated prefix of the full frame
}

// UnknownPacketEvent carries full diagnostic context for an unknown packet ID.
// It is passed to the UnknownPacketFunc callback. The library performs no I/O —
// the caller (goKore bot manager) is responsible for all observability.
//
// RecentPackets contains the last up to 3 successfully dispatched packets before
// the unknown ID was encountered, in chronological order (oldest first). Each
// entry holds a heap-allocated copy of the complete frame bytes so the bot
// manager can fully decode them offline.
//
// RawBuffer is a heap-allocated snapshot of the full receive buffer starting at
// the unknown packet ID — the 2-byte unknown ID followed by all bytes that
// trailed it in the current TCP read. The session's receive buffer is cleared
// after the callback returns, so RawBuffer is the only record of those bytes.
type UnknownPacketEvent struct {
	ID            uint16             // the unrecognised packet ID
	Packetver     uint32             // PACKETVER this session was constructed with
	Time          time.Time          // wall time at the moment of detection
	RecentPackets []DispatchedPacket // last ≤3 dispatched packets, oldest first
	RawBuffer     []byte             // snapshot copy of recvBuf from the unknown ID onward
}

// UnknownPacketFunc is a callback invoked synchronously by Feed() when a packet
// ID is not found in the length table.
//
// Since the frame length is unknown, the entire receive buffer is cleared after
// the callback returns — all bytes following the unknown ID in the current TCP
// read are discarded. The next call to Feed() starts with a clean buffer and may
// resume normal framing if the server sends a known packet ID.
//
// The event is fully self-contained and heap-allocated — the caller may retain
// it for as long as needed. The library performs no I/O of any kind.
//
// If nil, unknown packets are silently cleared.
type UnknownPacketFunc func(event UnknownPacketEvent)

// ErrUnknownPacket is returned by Feed() when a variable-length packet carries an
// embedded length value less than 4 (the minimum valid frame size). This indicates
// genuine stream corruption; the caller must close the connection.
//
// After ErrUnknownPacket is returned once, the session is marked faulted and all
// subsequent Feed() calls are silent no-ops returning nil.
type ErrUnknownPacket struct {
	ID uint16
}

func (e ErrUnknownPacket) Error() string {
	return fmt.Sprintf("session: packet ID %#04x has corrupt embedded length — stream desynced", e.ID)
}

// recentMaxFrameBytes is the maximum number of frame bytes stored per ring slot.
// Frames larger than this are stored truncated (the ID is always preserved).
// 4096 bytes covers all common game packets; at 3 slots per session this is
// 12 KB of fixed overhead per MapSession.
const recentMaxFrameBytes = 4096

// recentRing is a fixed-depth ring buffer of the last recentPacketDepth
// dispatched packets. All storage is inline — push() never allocates.
// snapshot() allocates only when an UnknownPacketEvent is being built, which
// is the exceptional path and therefore allocation is acceptable.
type recentRing struct {
	slots [recentPacketDepth]recentSlot
	head  int // index of the next slot to write
	count int // number of valid entries (saturates at recentPacketDepth)
}

// recentSlot is a single pre-allocated ring slot.
type recentSlot struct {
	id         uint16
	buf        [recentMaxFrameBytes]byte
	frameN     int // number of valid bytes in buf (may be < actual frame length if truncated)
	frameTotal int // actual frame length (may be > recentMaxFrameBytes)
}

// push records a dispatched packet. Copies up to recentMaxFrameBytes of the
// frame. Never allocates.
func (r *recentRing) push(id uint16, frame []byte) {
	s := &r.slots[r.head]
	s.id = id
	s.frameTotal = len(frame)
	s.frameN = copy(s.buf[:], frame) // copy returns min(len(frame), recentMaxFrameBytes)
	r.head = (r.head + 1) % recentPacketDepth
	if r.count < recentPacketDepth {
		r.count++
	}
}

// snapshot returns heap-allocated copies of the ring contents in chronological
// order (oldest first). Called only on the exceptional unknown-packet path.
func (r *recentRing) snapshot() []DispatchedPacket {
	if r.count == 0 {
		return nil
	}
	out := make([]DispatchedPacket, r.count)
	start := (r.head - r.count + recentPacketDepth) % recentPacketDepth
	for i := 0; i < r.count; i++ {
		s := &r.slots[(start+i)%recentPacketDepth]
		out[i] = DispatchedPacket{
			ID:         s.id,
			Frame:      append([]byte(nil), s.buf[:s.frameN]...),
			FrameTotal: s.frameTotal,
			Truncated:  s.frameN < s.frameTotal,
		}
	}
	return out
}

// sessionCore is the internal framing engine shared by all three session types.
type sessionCore struct {
	packetver        uint32
	buf              []byte       // full backing array; owned exclusively by sessionCore
	recvBuf          []byte       // active sub-slice of buf; advances as frames are consumed
	lengths          [65536]int16 // packet length table: 0 = unknown, -1 = variable (bytes[2:4])
	handlers         [65536]handlerFunc
	onUnknownPacket  UnknownPacketFunc
	recent           recentRing // ring buffer of last recentPacketDepth dispatched packets
	faulted          bool
	trace            TraceFunc
	unhandledPackets uint64
}

// feed implements the core framing and dispatch loop.
//
// Algorithm (source: HLD §9):
//  1. Append data to recvBuf without allocating if capacity permits.
//  2. Loop while recvBuf contains a complete frame:
//     a. Read packetID = leU16(recvBuf[0:2]).
//     b. Look up frameLen in lengths[packetID].
//     If -1: read frameLen from recvBuf[2:4] (variable-length packet).
//     If -1 and embedded length < 4: stream corrupt; fault and return ErrUnknownPacket.
//     If 0:  unknown packet ID; emit UnknownPacketEvent, clear buffer, stop.
//     c. If len(recvBuf) < frameLen: incomplete frame; break.
//     d. Dispatch: call handlers[packetID](recvBuf[:frameLen], packetver).
//     e. Advance: recvBuf = recvBuf[frameLen:]; push frame into recent ring.
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
			// Packet ID not in length table. Since the frame length is unknown we
			// cannot safely advance past it — any following byte could be payload.
			// Emit a fully-populated event with the recent-packet history and a
			// raw buffer snapshot, then clear the buffer. The next TCP read starts
			// clean and may recover if the server sends a known packet ID.
			// Matches OpenKore MessageTokenizer behavior.
			ev := UnknownPacketEvent{
				ID:            packetID,
				Packetver:     c.packetver,
				Time:          time.Now(),
				RecentPackets: c.recent.snapshot(),
				RawBuffer:     append([]byte(nil), c.recvBuf...),
			}
			if c.onUnknownPacket != nil {
				c.onUnknownPacket(ev)
			}
			if c.trace != nil {
				c.trace(ev)
			}
			c.recvBuf = c.recvBuf[:0]
			goto done
		}

		// Step 2c: wait for a complete frame.
		if len(c.recvBuf) < frameLen {
			break
		}

		// Step 2d: emit WireInbound trace (before dispatch), then dispatch.
		if c.trace != nil {
			frameCopy := append([]byte(nil), c.recvBuf[:frameLen]...)
			c.trace(WireInbound{
				ID:        packetID,
				Frame:     frameCopy,
				Packetver: c.packetver,
				Time:      time.Now(),
			})
		}

		if fn := c.handlers[packetID]; fn != nil {
			fn(c.recvBuf[:frameLen], c.packetver)
		} else {
			c.unhandledPackets++
		}

		// Step 2e: advance and push the frame into the recent-packet ring.
		c.recent.push(packetID, c.recvBuf[:frameLen])
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
func (c *sessionCore) registerHandler(id uint16, fn handlerFunc) {
	c.handlers[id] = fn
}

// setUnknownPacketHandler registers fn as the callback for packet IDs not found
// in the length table. Pass nil to clear a previously registered callback.
func (c *sessionCore) setUnknownPacketHandler(fn UnknownPacketFunc) {
	c.onUnknownPacket = fn
}

// setTraceFunc sets the unified trace hook. Pass nil to disable.
func (c *sessionCore) setTraceFunc(fn TraceFunc) {
	c.trace = fn
}

// isFaulted returns true after a corrupt embedded-length error.
func (c *sessionCore) isFaulted() bool {
	return c.faulted
}

// unhandledCount returns the cumulative count of frames that arrived with a known
// packet ID but no registered handler.
func (c *sessionCore) unhandledCount() uint64 {
	return c.unhandledPackets
}
