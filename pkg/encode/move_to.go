// Manually implemented — see docs/BACKLOG/EPIC-03_gokore_integration_prereqs.md US-16.

package encode

import (
	"github.com/lenaxia/rathena-client/pkg/packing"
	"github.com/lenaxia/rathena-client/pkg/send"
)

// EncodeMoveTo encodes a walk request (CZ_REQUEST_MOVE / CZ_REQUEST_MOVE2).
// The wire packet ID is resolved via shuffledCtoSID so that the correct ID is
// emitted for every PACKETVER, including shuffle-era clients.
//
// Base ID: 0x0085 (clif_packetdb.hpp line 37, first/canonical assignment).
// Post-shuffle stable wire ID: 0x035F (clif_shuffle.hpp PACKETVER > 20180307 block).
func EncodeMoveTo(req send.MoveTo, packetver uint32) [5]byte {
	id := shuffledCtoSID(packetver, 0x0085)
	coords := packing.EncodePosDir(req.X, req.Y, 0)
	var p [5]byte
	p[0] = byte(id)
	p[1] = byte(id >> 8)
	p[2] = coords[0]
	p[3] = coords[1]
	p[4] = coords[2]
	return p
}
