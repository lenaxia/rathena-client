// Manually implemented — see docs/BACKLOG/EPIC-03_gokore_integration_prereqs.md US-16.

package encode

import (
	"github.com/lenaxia/ragnarok-go-client/pkg/packing"
	"github.com/lenaxia/ragnarok-go-client/pkg/send"
)

// EncodeMoveTo encodes a walk request for the map server.
// 0x035F CZ_REQUEST_MOVE2: 5 bytes, fixed, all PACKETVER >= 20120307.
// Wire layout: [0-1] 0x5F 0x03 (packet ID LE), [2-4] packed pos from packing.EncodePosDir.
// Returns [5]byte to allow stack allocation — no heap allocation on the encode path.
func EncodeMoveTo(req send.MoveTo, packetver uint32) [5]byte {
	var p [5]byte
	p[0] = 0x5F
	p[1] = 0x03
	coords := packing.EncodePosDir(req.X, req.Y, 0)
	p[2] = coords[0]
	p[3] = coords[1]
	p[4] = coords[2]
	_ = packetver
	return p
}
