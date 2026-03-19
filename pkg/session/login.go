// Code generated in part by internal/codegen. DO NOT EDIT generated tables.
// Hand-written: NewLoginSession, Feed, registerHandler.
package session

const loginRecvBufInitial = 4096

// LoginSession handles the rAthena login server connection (CA_/AC_ packets).
// Source: src/common/packets.hpp (login server structs).
type LoginSession struct {
	core sessionCore
}

// NewLoginSession creates a LoginSession for the given PACKETVER.
// The lengths table is populated from the generated populateLoginLengths function.
func NewLoginSession(packetver uint32) *LoginSession {
	s := &LoginSession{}
	s.core.packetver = packetver
	s.core.buf = make([]byte, loginRecvBufInitial)
	s.core.recvBuf = s.core.buf[:0]
	populateLoginLengths(packetver, &s.core.lengths)
	return s
}

// Feed processes raw bytes received from the login server.
// It is synchronous and not goroutine-safe.
func (s *LoginSession) Feed(data []byte) error {
	return s.core.feed(data)
}

// setLength sets the frame length for a packet ID in the login lengths table.
// This is intended for testing only. A length of -1 means variable-length.
func (s *LoginSession) setLength(id uint16, length int16) {
	s.core.lengths[id] = length
}

// SetUnknownPacketHandler registers fn as the callback invoked when Feed()
// encounters a packet ID not in the length table. The entire receive buffer is
// cleared after the callback returns. Pass nil to clear.
func (s *LoginSession) SetUnknownPacketHandler(fn UnknownPacketFunc) {
	s.core.setUnknownPacketHandler(fn)
}

// registerHandler registers fn as the callback for the given packet ID.
// Overwrites any existing handler for that ID.
func (s *LoginSession) registerHandler(id uint16, fn handlerFunc) {
	s.core.registerHandler(id, fn)
}
