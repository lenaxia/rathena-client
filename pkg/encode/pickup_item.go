// Manually implemented — complex per-week wire ID history mirrors move_to.go.
// See docs/WORKLOG/0070 for full cross-validation against rAthena and OpenKore.

package encode

import "github.com/lenaxia/rathena-client/pkg/send"

// EncodePickupItem encodes a floor item pickup request (CZ_ITEM_PICKUP / CZ_ITEM_PICKUP2).
//
// Wire packet ID is determined by a two-stage lookup:
//
//  1. pv < 20130515 (pre-shuffle): explicit per-block assignments from
//     clif_packetdb.hpp. All post-20101124 variants are 6 bytes, ITID at offset 2.
//     The pre-20101124 era has different sizes and field offsets and is out of scope
//     for production servers; the baseline 0x009F (6 bytes) is returned as fallback.
//
//  2. pv >= 20130515 (shuffle era): shuffledCtoSID(pv, 0x009F).
//     All 57 verifiable weekly entries match OpenKore exactly (0 mismatches).
//     For pv > 20180307 returns stable 0x0362.
//
// Sources:
//
//	clif_packetdb.hpp lines 50, 1384, 1402, 1441, 1494, 1560, 1587, 1631
//	clif_shuffle.hpp  — per-week exact-match blocks with case 0x009F
//	worklog 0070      — full OpenKore cross-validation
func EncodePickupItem(req send.PickupItem, packetver uint32) [6]byte {
	var id uint16
	switch {
	case packetver < 20101124:
		id = 0x009F // baseline — clif_packetdb.hpp:50
	case packetver < 20111005:
		id = 0x0362 // clif_packetdb.hpp:1384
	case packetver < 20120307:
		id = 0x0815 // clif_packetdb.hpp:1402
	case packetver < 20120410:
		id = 0x0865 // clif_packetdb.hpp:1441
	case packetver < 20120418:
		id = 0x0938 // clif_packetdb.hpp:1494
	case packetver < 20120702:
		id = 0x07E4 // clif_packetdb.hpp:1560
	case packetver < 20130320:
		id = 0x089F // clif_packetdb.hpp:1587
	case packetver < 20130515:
		id = 0x0933 // clif_packetdb.hpp:1631
	default:
		id = shuffledCtoSID(packetver, 0x009F) // clif_shuffle.hpp; stable 0x0362 post-20180307
	}
	var p [6]byte
	p[0] = byte(id)
	p[1] = byte(id >> 8)
	leU32Put(p[2:], req.ITID) // rAthena: ITID
	return p
}
