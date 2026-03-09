// Package encode contains generated and hand-written functions for encoding
// send requests into raw Ragnarok Online network packet bytes.
//
// Generated files: one per semantic action (e.g. move_to.go).
// Hand-written helpers: this file (helpers.go).
//
// All writes are little-endian, matching the rAthena wire protocol.
// Fixed-size packets return [N]byte arrays to avoid heap allocation.
package encode

import "encoding/binary"

// leU16Put writes a little-endian uint16 into b[0:2].
func leU16Put(b []byte, v uint16) {
	binary.LittleEndian.PutUint16(b, v)
}

// leU32Put writes a little-endian uint32 into b[0:4].
func leU32Put(b []byte, v uint32) {
	binary.LittleEndian.PutUint32(b, v)
}

// leU64Put writes a little-endian uint64 into b[0:8].
func leU64Put(b []byte, v uint64) {
	binary.LittleEndian.PutUint64(b, v)
}
