// Hand-written: NewMapSession, Feed, RegisterHandler, EnableObfuscation, Encode.
package session

const mapRecvBufInitial = 65536

// MapSession handles the rAthena map server connection (CZ_/ZC_ packets).
// Source: src/map/packets.hpp + src/map/packets_struct.hpp.
//
// MapSession additionally supports C→S packet ID obfuscation via EnableObfuscation.
// S→C packets (received via Feed) are never obfuscated.
type MapSession struct {
	core   sessionCore
	oState obfuscationState
}

// NewMapSession creates a MapSession for the given PACKETVER.
// The lengths table is populated from the generated populateMapLengths function.
func NewMapSession(packetver uint32) *MapSession {
	s := &MapSession{}
	s.core.packetver = packetver
	s.core.buf = make([]byte, mapRecvBufInitial)
	s.core.recvBuf = s.core.buf[:0]
	populateMapLengths(packetver, &s.core.lengths)
	return s
}

// Feed processes raw bytes received from the map server.
// It is synchronous and not goroutine-safe.
func (s *MapSession) Feed(data []byte) error {
	return s.core.feed(data)
}

// RegisterHandler registers fn as the callback for the given packet ID.
// Overwrites any existing handler for that ID.
func (s *MapSession) RegisterHandler(id uint16, fn HandlerFunc) {
	s.core.registerHandler(id, fn)
}

// SetLength sets the frame length for a packet ID in the map lengths table.
// This is intended for FSM auth-phase setup and testing only.
// A length of -1 means variable-length.
func (s *MapSession) SetLength(id uint16, length int16) {
	s.core.lengths[id] = length
}

// SetUnknownPacketHandler registers fn as the callback invoked when Feed()
// encounters a packet ID not in the length table. The entire receive buffer is
// cleared after the callback returns. Pass nil to clear.
func (s *MapSession) SetUnknownPacketHandler(fn UnknownPacketFunc) {
	s.core.setUnknownPacketHandler(fn)
}

// EnableObfuscation activates C→S packet ID obfuscation for this session.
// Must be called before the first Encode call.
// key0, key1, key2 are clif_cryptKey[0], clif_cryptKey[1], clif_cryptKey[2] from
// src/map/clif_obfuscation.hpp (obtained via ObfuscationKeysFor).
//
// S→C packets received via Feed are never obfuscated.
func (s *MapSession) EnableObfuscation(key0, key1, key2 uint32) {
	s.oState.enabled = true
	s.oState.firstSent = false
	s.oState.key0 = key0
	s.oState.key1 = key1
	s.oState.key2 = key2
	// Precompute firstKey and rollingKey.
	// Source: clif.cpp:25702 (first packet), clif.cpp:10721 (rolling init).
	step1 := (uint64(key0)*uint64(key1) + uint64(key2)) & 0xFFFFFFFF
	s.oState.firstKey = uint16((step1 >> 16) & 0x7FFF)
	s.oState.rollingKey = uint32(((step1 * uint64(key1)) + uint64(key2)) & 0xFFFFFFFF)
}

// Encode applies C→S packet ID obfuscation (if enabled) to *pktID in-place.
// The caller builds the raw packet using a generated encode function, then calls
// Encode on the packet ID field before writing to the socket.
// Encode does not allocate.
func (s *MapSession) Encode(pktID *uint16) {
	if !s.oState.enabled {
		return
	}
	if !s.oState.firstSent {
		*pktID ^= s.oState.firstKey
		s.oState.firstSent = true
		return
	}
	// Subsequent packets use the rolling key.
	// Source: clif.cpp obfuscation logic — key advances after each C→S packet.
	key := uint16((s.oState.rollingKey >> 16) & 0x7FFF)
	*pktID ^= key
	s.oState.rollingKey = uint32((uint64(s.oState.rollingKey)*uint64(s.oState.key1) + uint64(s.oState.key2)) & 0xFFFFFFFF)
}
