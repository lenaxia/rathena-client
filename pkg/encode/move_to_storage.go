// Manually implemented — complex pre/shuffle wire ID history mirrors pickup_item.go.
// See docs/WORKLOG/0073 for full cross-validation against rAthena and OpenKore.

package encode

import "github.com/lenaxia/rathena-client/pkg/send"

// EncodeMoveToStorage encodes an inventory→storage item transfer
// (CZ_MOVE_ITEM_FROM_BODY_TO_STORE / CZ_MOVE_ITEM_FROM_BODY_TO_STORE2).
//
// Wire packet ID is determined by a two-stage lookup:
//
//  1. pv < 20130515 (pre-shuffle): explicit per-block assignments from
//     clif_packetdb.hpp. All post-20101124 variants are 8 bytes with the same
//     field layout. The pre-20101124 era is out of scope; baseline 0x00F3 as fallback.
//
//  2. pv >= 20130515 (shuffle era): shuffledCtoSID(pv, 0x00F3).
//     For pv > 20180307 returns stable 0x0364.
//
// Sources:
//
//	clif_packetdb.hpp lines 95, 1386, 1404, 1443, 1496, 1562, 1588, 1626
//	clif_shuffle.hpp  — stable post-20180307: parseable_packet(0x0364, 8, clif_parse_MoveToKafra, 2, 4)
//	worklog 0073      — rAthena + OpenKore cross-validation (storage_item_add 0364 ✓)
func EncodeMoveToStorage(req send.MoveToStorage, packetver uint32) [8]byte {
	var id uint16
	switch {
	case packetver < 20101124:
		id = 0x00F3 // baseline — clif_packetdb.hpp:95
	case packetver < 20111005:
		id = 0x0364 // clif_packetdb.hpp:1386
	case packetver < 20120307:
		id = 0x0893 // clif_packetdb.hpp:1404
	case packetver < 20120410:
		id = 0x093B // clif_packetdb.hpp:1443
	case packetver < 20120418:
		id = 0x086C // clif_packetdb.hpp:1496
	case packetver < 20120702:
		id = 0x07EC // clif_packetdb.hpp:1562
	case packetver < 20130320:
		id = 0x08A0 // clif_packetdb.hpp:1588
	case packetver < 20130515:
		id = 0x08AC // clif_packetdb.hpp:1626
	default:
		id = shuffledCtoSID(packetver, 0x00F3) // stable 0x0364 post-20180307
	}
	var p [8]byte
	p[0] = byte(id)
	p[1] = byte(id >> 8)
	leU16Put(p[2:], req.Index)  // rAthena: index (.W = uint16)
	leU32Put(p[4:], req.Amount) // rAthena: amount (.L = uint32)
	return p
}
