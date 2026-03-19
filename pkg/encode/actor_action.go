// Manually implemented — see docs/BACKLOG/EPIC-03_gokore_integration_prereqs.md US-17.

package encode

import (
	"github.com/lenaxia/rathena-client/pkg/send"
)

// EncodeActorAction encodes a ActorAction (CZ_REQUEST_ACT / CZ_REQUEST_ACT2) packet.
// The wire packet ID is resolved via shuffledCtoSID so that the correct ID is
// emitted for every PACKETVER, including shuffle-era clients.
//
// Base ID: 0x0089 (clif_packetdb.hpp line 38, first/canonical assignment).
// Post-shuffle stable wire ID: 0x0437 (clif_shuffle.hpp PACKETVER > 20180307 block).
func EncodeActorAction(req send.ActorAction, packetver uint32) [7]byte {
	id := shuffledCtoSID(packetver, 0x0089)
	var p [7]byte
	p[0] = byte(id)
	p[1] = byte(id >> 8)
	leU32Put(p[2:], req.TargetGID) // rAthena: TargetGID
	p[6] = req.Action              // rAthena: Action
	return p
}
