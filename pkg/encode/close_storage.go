// Manually maintained — protocol-removed in rAthena after pv=20050110.
// See docs/WORKLOG/0073 and semantics DB close_storage packetver_max=20050110.
//
// WARNING: clif_parse_CloseKafra was removed from rAthena's packet registration
// tables after pv=20050110 and is absent from clif_shuffle.hpp entirely. At any
// modern packetver (>= 20050110), 0x00F7 may be reassigned to a different handler.
// This encoder should only be used with servers running pv <= 20050110.

package encode

import "github.com/lenaxia/rathena-client/pkg/send"

// EncodeCloseStorage encodes a 0x00F7 (SYNTH_CZ_CLOSE_STORE) packet for sending to the server.
// Only valid for pv <= 20050110 — see package comment above.
func EncodeCloseStorage(req send.CloseStorage, packetver uint32) [2]byte {
	var p [2]byte
	// Packet ID: 0x00F7 (little-endian)
	p[0] = 0xf7
	p[1] = 0x00
	_ = req
	_ = packetver
	return p
}
