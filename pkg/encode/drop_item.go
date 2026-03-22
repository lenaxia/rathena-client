// Manually implemented — complex pre/shuffle wire ID history mirrors pickup_item.go.
// See docs/WORKLOG/0073 for full cross-validation against rAthena and OpenKore.

package encode

import "github.com/lenaxia/rathena-client/pkg/send"

// EncodeDropItem encodes a floor item drop request (CZ_ITEM_THROW / CZ_ITEM_THROW2).
//
// Wire packet ID is determined by a two-stage lookup:
//
//  1. pv < 20130515 (pre-shuffle): explicit per-block assignments from
//     clif_packetdb.hpp. All post-20101124 variants are 6 bytes with the same
//     field layout. The pre-20101124 era has scrambled field offsets and is out
//     of scope for production servers; baseline 0x00A2 is returned as fallback.
//
//  2. pv >= 20130515 (shuffle era): shuffledCtoSID(pv, 0x00A2).
//     For pv > 20180307 returns stable 0x0363.
//
// Sources:
//
//	clif_packetdb.hpp lines 51, 1385, 1403, 1442, 1561, 1586, 1606
//	clif_shuffle.hpp  — stable post-20180307: parseable_packet(0x0363, 6, clif_parse_DropItem, 2, 4)
//	worklog 0073      — rAthena + OpenKore cross-validation (item_drop 0363 ✓)
func EncodeDropItem(req send.DropItem, packetver uint32) [6]byte {
	var id uint16
	switch {
	case packetver < 20101124:
		id = 0x00A2 // baseline — clif_packetdb.hpp:51
	case packetver < 20111005:
		id = 0x0363 // clif_packetdb.hpp:1385
	case packetver < 20120307:
		id = 0x0885 // clif_packetdb.hpp:1403
	case packetver < 20120418:
		id = 0x02C4 // clif_packetdb.hpp:1442
	case packetver < 20120702:
		id = 0x0362 // clif_packetdb.hpp:1561
	case packetver < 20130320:
		id = 0x089E // clif_packetdb.hpp:1586
	case packetver < 20130515:
		id = 0x0438 // clif_packetdb.hpp:1606
	default:
		id = shuffledCtoSID(packetver, 0x00A2) // stable 0x0363 post-20180307
	}
	var p [6]byte
	p[0] = byte(id)
	p[1] = byte(id >> 8)
	leU16Put(p[2:], req.Index)  // rAthena: Index
	leU16Put(p[4:], req.Amount) // rAthena: Amount
	return p
}
