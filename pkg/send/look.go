// Manually maintained — hand-written to preserve field documentation.
// (The encoder look.go is hand-written; the send struct must match.)

package send

// Look is the request struct for CZ_CHANGE_DIRECTION / CZ_CHANGE_DIRECTION2.
type Look struct {
	// HeadDir is the head facing direction.
	// Valid values: 0 (N/up), 1 (NW), 2 (W). Most clients only use 0–2.
	// rAthena field: headdir. Wire position: offset 2 (uint8).
	HeadDir uint8

	// Dir is the body facing direction, 0–7 clockwise from North.
	//   0=N, 1=NW, 2=W, 3=SW, 4=S, 5=SE, 6=E, 7=NE
	// rAthena field: dir. Wire position: offset 4 (uint8, after 1 byte padding at offset 3).
	Dir uint8
}
