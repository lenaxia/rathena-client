// Package decode contains generated and hand-written functions for decoding
// Ragnarok Online network packets from raw bytes into typed event structs.
//
// Generated files: one per semantic action (e.g. actor_moved.go).
// Hand-written helpers: this file (helpers.go).
//
// All reads are little-endian, matching the rAthena wire protocol.
// All functions are allocation-free on the happy path.
package decode

import (
	"encoding/binary"
	"unsafe"
)

// leU16 reads a little-endian uint16 from data at offset off.
func leU16(data []byte, off int) uint16 {
	return binary.LittleEndian.Uint16(data[off:])
}

// leI16 reads a little-endian int16 from data at offset off.
func leI16(data []byte, off int) int16 {
	return int16(binary.LittleEndian.Uint16(data[off:]))
}

// leU32 reads a little-endian uint32 from data at offset off.
func leU32(data []byte, off int) uint32 {
	return binary.LittleEndian.Uint32(data[off:])
}

// leI32 reads a little-endian int32 from data at offset off.
func leI32(data []byte, off int) int32 {
	return int32(binary.LittleEndian.Uint32(data[off:]))
}

// leU64 reads a little-endian uint64 from data at offset off.
func leU64(data []byte, off int) uint64 {
	return binary.LittleEndian.Uint64(data[off:])
}

// leI64 reads a little-endian int64 from data at offset off.
func leI64(data []byte, off int) int64 {
	return int64(binary.LittleEndian.Uint64(data[off:]))
}

// nullTermString returns a Go string from a null-terminated or fixed-length
// byte slice. Any bytes after the first null are ignored.
//
// Zero-alloc: uses unsafe.String to alias the input slice directly.
// The returned string is valid only as long as the underlying []byte is not
// modified or garbage-collected. This is safe because pkg/decode functions
// take a session-owned []byte that lives for the duration of event dispatch.
func nullTermString(b []byte) string {
	n := len(b)
	for i, c := range b {
		if c == 0 {
			n = i
			break
		}
	}
	if n == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(b), n)
}
