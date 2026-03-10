// Manually implemented — see docs/BACKLOG/EPIC-03_gokore_integration_prereqs.md US-17.
// GCC-verified: packetdb_addpacket(0x085a, 7, clif_parse_ActionRequest, 2, 6, 0)
// at PACKETVER=20200401.

package encode

import (
	"github.com/lenaxia/ragnarok-go-client/pkg/send"
)

// EncodeActorAction encodes an attack or action request for the map server.
// 0x085a clif_parse_ActionRequest: 7 bytes, PACKETVER >= 20200401.
// Wire: [0-1] packet ID 0x085A LE, [2-5] TargetID uint32 LE, [6] Type uint8.
//
// Returns [7]byte to allow stack allocation — no heap allocation on the encode path.
// Note: send.ActorAction is a server-notification struct reused here for the C→S
// case. Only TargetID and Type are read; all other fields are ignored.
func EncodeActorAction(req send.ActorAction, packetver uint32) [7]byte {
	var p [7]byte
	p[0] = 0x5A
	p[1] = 0x08
	leU32Put(p[2:], req.TargetID) // rAthena: target_id
	p[6] = req.Type               // rAthena: action
	_ = packetver
	return p
}
