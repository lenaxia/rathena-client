// Hand-written: NewCharSession, Feed, RegisterHandler.
package session

const charRecvBufInitial = 4096

// CharSession handles the rAthena char server connection (CH_/HC_ packets).
// Source: src/common/packets.hpp (char server structs).
type CharSession struct {
	core sessionCore
}

// NewCharSession creates a CharSession for the given PACKETVER.
// The lengths table is populated from the generated populateCharLengths function.
func NewCharSession(packetver uint32) *CharSession {
	s := &CharSession{}
	s.core.packetver = packetver
	s.core.buf = make([]byte, charRecvBufInitial)
	s.core.recvBuf = s.core.buf[:0]
	populateCharLengths(packetver, &s.core.lengths)
	return s
}

// Feed processes raw bytes received from the char server.
// It is synchronous and not goroutine-safe.
func (s *CharSession) Feed(data []byte) error {
	return s.core.feed(data)
}

// RegisterHandler registers fn as the callback for the given packet ID.
// Overwrites any existing handler for that ID.
func (s *CharSession) RegisterHandler(id uint16, fn HandlerFunc) {
	s.core.registerHandler(id, fn)
}

// SetLength sets the frame length for a packet ID in the char lengths table.
// This is intended for testing only. A length of -1 means variable-length.
func (s *CharSession) SetLength(id uint16, length int16) {
	s.core.lengths[id] = length
}

// SetUnknownPacketHandler registers fn as the callback invoked when Feed()
// encounters a packet ID not in the length table. The entire receive buffer is
// cleared after the callback returns. Pass nil to clear.
func (s *CharSession) SetUnknownPacketHandler(fn UnknownPacketFunc) {
	s.core.setUnknownPacketHandler(fn)
}
