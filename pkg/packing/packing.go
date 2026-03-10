// Package packing implements the two packed binary position formats used by
// rAthena's wire protocol: the 3-byte WBUFPOS / RBUFPOS format and the 6-byte
// WBUFPOS2 / RBUFPOS2 format.
//
// Source authority: src/map/clif.cpp lines 173–249 (rAthena).
// Both encode and decode functions are the exact inverse of the C originals.
//
// Design notes:
//   - All functions are pure (no state, no side effects).
//   - All functions are safe to call concurrently from multiple goroutines.
//   - No allocations: encode functions return fixed-size arrays by value.
package packing

// DecodePosDir decodes a 3-byte RBUFPOS-encoded position+direction.
//
// Wire layout (WBUFPOS):
//
//	p[0] = x >> 2
//	p[1] = (x << 6) | ((y >> 4) & 0x3f)
//	p[2] = (y << 4) | (dir & 0x0f)
//
// x and y are 10-bit map coordinates. dir is a 4-bit direction value (0–15,
// where rAthena uses 0=N, 1=NW, 2=W, 3=SW, 4=S, 5=SE, 6=E, 7=NE —
// source: src/map/path.hpp DIR_NORTH=0, DIR_NORTHWEST=1, etc. The
// library does not interpret direction values; it decodes the bits as-is).
//
// At least 3 bytes of data must be available; the caller is responsible for
// bounds checking at the call site (this is always known statically in
// generated decode functions).
func DecodePosDir(data []byte) (x, y uint16, dir uint8) {
	x = (uint16(data[0]&0xff) << 2) | uint16(data[1]>>6)
	y = (uint16(data[1]&0x3f) << 4) | uint16(data[2]>>4)
	dir = data[2] & 0x0f
	return
}

// EncodePosDir encodes x, y, and dir into a 3-byte WBUFPOS-format array.
// x and y must be valid 10-bit map coordinates (0–1023). dir must be a 4-bit
// value (0–15); bits above position 3 are masked off.
func EncodePosDir(x, y uint16, dir uint8) [3]byte {
	var p [3]byte
	p[0] = uint8(x >> 2)
	p[1] = uint8((x << 6) | ((y >> 4) & 0x3f))
	p[2] = uint8((y << 4) | (uint16(dir) & 0x0f))
	return p
}

// DecodeMoveData decodes a 6-byte RBUFPOS2-encoded movement record.
//
// Wire layout (WBUFPOS2):
//
//	p[0] = x0 >> 2
//	p[1] = (x0 << 6) | ((y0 >> 4) & 0x3f)
//	p[2] = (y0 << 4) | ((x1 >> 6) & 0x0f)
//	p[3] = (x1 << 2) | ((y1 >> 8) & 0x03)
//	p[4] = y1
//	p[5] = (sx0 << 4) | (sy0 & 0x0f)
//
// fromX/fromY are the origin cell; toX/toY are the destination cell.
// sx0/sy0 are sub-cell interpolation offsets for the visual client (each 4
// bits, range 0–15). For bot purposes sx0/sy0 can be ignored.
//
// IMPORTANT: byte 5 is NOT a direction value. The 6-byte format encodes no
// direction. Extracting (data[5] & 0xF0) >> 4 as direction is incorrect (this
// is a known bug in goKore v1 that this library fixes).
//
// At least 6 bytes of data must be available.
func DecodeMoveData(data []byte) (fromX, fromY, toX, toY uint16, sx0, sy0 uint8) {
	fromX = (uint16(data[0]&0xff) << 2) | uint16(data[1]>>6)
	fromY = (uint16(data[1]&0x3f) << 4) | uint16(data[2]>>4)
	toX = (uint16(data[2]&0x0f) << 6) | uint16(data[3]>>2)
	toY = (uint16(data[3]&0x03) << 8) | uint16(data[4])
	sx0 = (data[5] & 0xf0) >> 4
	sy0 = data[5] & 0x0f
	return
}

// EncodeMoveData encodes a movement record into a 6-byte WBUFPOS2-format array.
// fromX, fromY, toX, toY must be valid 10-bit map coordinates (0–1023).
// sx0 and sy0 are sub-cell interpolation offsets (normally 0–15); if either exceeds 15, the upper bits
// are masked off before encoding.
func EncodeMoveData(fromX, fromY, toX, toY uint16, sx0, sy0 uint8) [6]byte {
	var p [6]byte
	p[0] = uint8(fromX >> 2)
	p[1] = uint8((fromX << 6) | ((fromY >> 4) & 0x3f))
	p[2] = uint8((fromY << 4) | ((toX >> 6) & 0x0f))
	p[3] = uint8((toX << 2) | ((toY >> 8) & 0x03))
	p[4] = uint8(toY)
	p[5] = uint8(((sx0 & 0x0f) << 4) | (sy0 & 0x0f))
	return p
}
