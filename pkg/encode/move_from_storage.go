// Manually implemented — complex pre/shuffle wire ID history mirrors pickup_item.go.
// See docs/WORKLOG/0073 for full cross-validation against rAthena and OpenKore.

package encode

import "github.com/lenaxia/rathena-client/pkg/send"

// EncodeMoveFromStorage encodes a storage→inventory item transfer
// (CZ_MOVE_ITEM_FROM_STORE_TO_BODY / CZ_MOVE_ITEM_FROM_STORE_TO_BODY2).
//
// Wire packet ID is determined by a two-stage lookup:
//
//  1. pv < 20130515 (pre-shuffle): explicit per-block assignments from
//     clif_packetdb.hpp. All post-20101124 variants are 8 bytes with the same
//     field layout. The pre-20101124 era is out of scope; baseline 0x00F5 as fallback.
//
//  2. pv >= 20130515 (shuffle era): shuffledCtoSID(pv, 0x00F5).
//     For pv > 20180307 returns stable 0x0365.
//
// Sources:
//
//	clif_packetdb.hpp lines 96, 1387, 1405, 1444, 1497, 1563, 1581, 1618
//	clif_shuffle.hpp  — stable post-20180307: parseable_packet(0x0365, 8, clif_parse_MoveFromKafra, 2, 4)
//	worklog 0073      — rAthena + OpenKore cross-validation (storage_item_remove 0365 ✓)
func EncodeMoveFromStorage(req send.MoveFromStorage, packetver uint32) [8]byte {
	var id uint16
	switch {
	case packetver < 20101124:
		id = 0x00F5 // baseline — clif_packetdb.hpp:96
	case packetver < 20111005:
		id = 0x0365 // clif_packetdb.hpp:1387
	case packetver < 20120307:
		id = 0x0897 // clif_packetdb.hpp:1405
	case packetver < 20120410:
		id = 0x0963 // clif_packetdb.hpp:1444
	case packetver < 20120418:
		id = 0x08A6 // clif_packetdb.hpp:1497
	case packetver < 20120702:
		id = 0x0364 // clif_packetdb.hpp:1563
	case packetver < 20130320:
		id = 0x0861 // clif_packetdb.hpp:1581
	case packetver < 20130515:
		id = 0x0874 // clif_packetdb.hpp:1618
	default:
		id = shuffledCtoSID(packetver, 0x00F5) // stable 0x0365 post-20180307
	}
	var p [8]byte
	p[0] = byte(id)
	p[1] = byte(id >> 8)
	leU16Put(p[2:], req.Index)  // rAthena: index (.W = uint16)
	leU32Put(p[4:], req.Amount) // rAthena: amount (.L = uint32)
	return p
}
