// Manually implemented — see docs/BACKLOG/EPIC-03_gokore_integration_prereqs.md US-16.

package encode

import (
	"github.com/lenaxia/rathena-client/pkg/packing"
	"github.com/lenaxia/rathena-client/pkg/send"
)

// EncodeMoveTo encodes a walk request (CZ_REQUEST_MOVE / CZ_REQUEST_MOVE2).
//
// The wire packet ID is determined by a two-stage lookup:
//
//  1. For packetver < 20130515 (pre-shuffle era): a direct switch selects the
//     wire ID from the explicit clif_packetdb.hpp per-block assignments. Each
//     block reassigns the WalkToXY handler to a new ID; there is no canonical
//     stable base — 0x0085 was reassigned to ActionRequest, UseSkill, etc. in
//     later blocks.
//
//  2. For packetver >= 20130515 (shuffle era): shuffledCtoSID(pv, 0x0085) is
//     called. The shuffle table in clif_shuffle.hpp uses exact PACKETVER ==
//     matches from 20130515 onward and always has a correct mapping for 0x0085.
//     For packetver > 20180307 the stable post-shuffle wire ID is 0x035F.
//
// Sources:
//
//	clif_packetdb.hpp — parseable_packet(0xNNNN,5,clif_parse_WalkToXY,2)
//	clif_shuffle.hpp  — per-week exact-match blocks with case 0x0085
func EncodeMoveTo(req send.MoveTo, packetver uint32) [5]byte {
	var id uint16
	switch {
	// Pre-shuffle era: wire IDs from clif_packetdb.hpp explicit blocks.
	// Each block overrides the previous with a new ID assignment.
	case packetver < 20040726:
		id = 0x0085 // clif_packetdb.hpp line 37: default baseline
	case packetver < 20040906:
		id = 0x009b // >= 20040726
	case packetver < 20041129:
		id = 0x0089 // >= 20040906
	case packetver < 20101124:
		id = 0x00a7 // >= 20041129
	case packetver < 20111005:
		id = 0x035f // >= 20101124
	case packetver < 20120307:
		id = 0x0364 // >= 20111005
	case packetver < 20120702:
		id = 0x0437 // >= 20120307
	case packetver < 20130320:
		id = 0x0953 // >= 20120702
	case packetver < 20130515:
		id = 0x0881 // >= 20130320 (clif_packetdb.hpp line 1619)
	default:
		// Shuffle era (>= 20130515): clif_shuffle.hpp covers every weekly
		// packetver with an exact PACKETVER == match; 0x0085 is present in
		// every block. For packetver > 20180307, returns 0x035F (stable).
		id = shuffledCtoSID(packetver, 0x0085)
	}

	coords := packing.EncodePosDir(req.X, req.Y, 0)
	var p [5]byte
	p[0] = byte(id)
	p[1] = byte(id >> 8)
	p[2] = coords[0]
	p[3] = coords[1]
	p[4] = coords[2]
	return p
}
