// Manually implemented — see docs/BACKLOG/EPIC-03_gokore_integration_prereqs.md US-18.
// GCC-verified: packetdb_addpacket(0x0862, 10, clif_parse_UseSkillToId, 2, 4, 6, 0)
// at PACKETVER=20200401.

package encode

import (
	"github.com/lenaxia/ragnarok-go-client/pkg/send"
)

// EncodeSkillUse encodes a skill-to-actor use request for the map server.
// 0x0862 clif_parse_UseSkillToId: 10 bytes, PACKETVER >= 20200401.
// Wire: [0-1] packet ID 0x0862 LE, [2-3] Lv uint16 LE, [4-5] SkillID uint16 LE, [6-9] TargetID uint32 LE.
//
// Returns [10]byte to allow stack allocation — no heap allocation on the encode path.
// Note: send.SkillUse is a server-notification struct reused here for the C→S
// case. Only Lv, SkillID, and TargetID are read; all other fields are ignored.
func EncodeSkillUse(req send.SkillUse, packetver uint32) [10]byte {
	var p [10]byte
	p[0] = 0x62
	p[1] = 0x08
	leU16Put(p[2:], req.Lv)       // rAthena: skilllv
	leU16Put(p[4:], req.SkillID)  // rAthena: skillid
	leU32Put(p[6:], req.TargetID) // rAthena: target_id
	_ = packetver
	return p
}
