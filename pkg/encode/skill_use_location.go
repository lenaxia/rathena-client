// Manually implemented — complex pre/shuffle wire ID history mirrors pickup_item.go.
// See docs/WORKLOG/0073 for full cross-validation against rAthena and OpenKore.

package encode

import "github.com/lenaxia/rathena-client/pkg/send"

// EncodeSkillUseLocation encodes a ground-targeted skill cast
// (CZ_USE_SKILL_TOGROUND / CZ_USE_SKILL_TOGROUND2).
//
// Wire packet ID is determined by a two-stage lookup:
//
//  1. pv < 20130515 (pre-shuffle): explicit per-block assignments from
//     clif_packetdb.hpp. All post-20101124 variants are 10 bytes with the same
//     field layout. The pre-20101124 era is out of scope; baseline 0x0116 as fallback.
//     Note: the 20120418 block has no UseSkillToPos entry — it inherits 0x0438 from
//     the 20120307/20120410 block.
//
//  2. pv >= 20130515 (shuffle era): shuffledCtoSID(pv, 0x0116).
//     For pv > 20180307 returns stable 0x0366.
//
// Sources:
//
//	clif_packetdb.hpp lines 114, 1388, 1406, 1445, 1498, 1583, 1637
//	clif_shuffle.hpp  — stable post-20180307: parseable_packet(0x0366, 10, clif_parse_UseSkillToPos, 2, 4, 6, 8)
//	worklog 0073      — rAthena + OpenKore cross-validation (ServerType0.pm '0366' ✓)
func EncodeSkillUseLocation(req send.SkillUseLocation, packetver uint32) [10]byte {
	var id uint16
	switch {
	case packetver < 20101124:
		id = 0x0116 // baseline — clif_packetdb.hpp:114
	case packetver < 20111005:
		id = 0x0366 // clif_packetdb.hpp:1388
	case packetver < 20120307:
		id = 0x0369 // clif_packetdb.hpp:1406
	case packetver < 20120702:
		id = 0x0438 // clif_packetdb.hpp:1445/1498 (20120418 block has no entry; inherits)
	case packetver < 20130320:
		id = 0x0863 // clif_packetdb.hpp:1583
	case packetver < 20130515:
		id = 0x0959 // clif_packetdb.hpp:1637
	default:
		id = shuffledCtoSID(packetver, 0x0116) // stable 0x0366 post-20180307
	}
	var p [10]byte
	p[0] = byte(id)
	p[1] = byte(id >> 8)
	leU16Put(p[2:], req.SkillLevel) // rAthena: skillLevel
	leU16Put(p[4:], req.SkillID)    // rAthena: skillID
	leU16Put(p[6:], req.XPos)       // rAthena: xPos
	leU16Put(p[8:], req.YPos)       // rAthena: yPos
	return p
}
