// Manually implemented — see docs/BACKLOG/EPIC-03_gokore_integration_prereqs.md US-17.

package encode

import (
	"github.com/lenaxia/rathena-client/pkg/send"
)

// EncodeActorAction encodes a ActorAction (CZ_REQUEST_ACT / CZ_REQUEST_ACT2) packet.
//
// The wire packet ID is determined by a two-stage lookup:
//
//  1. For packetver < 20130515 (pre-shuffle era): a direct switch selects the
//     wire ID from explicit clif_packetdb.hpp per-block assignments. The ID
//     0x0089 is only ActionRequest in the baseline block — subsequent blocks
//     reassign it to 0x0193, 0x0085, 0x009f, 0x0190, 0x0437, etc.
//
//  2. For packetver >= 20130515 (shuffle era): shuffledCtoSID(pv, 0x0089) is
//     called. The shuffle table covers every weekly packetver from 20130515
//     onward with exact PACKETVER == matches. For packetver > 20180307 the
//     stable post-shuffle wire ID is 0x0437.
//
// Sources:
//
//	clif_packetdb.hpp — parseable_packet(0xNNNN,7,clif_parse_ActionRequest,2,6)
//	clif_shuffle.hpp  — per-week exact-match blocks with case 0x0089
func EncodeActorAction(req send.ActorAction, packetver uint32) [7]byte {
	var id uint16
	switch {
	// Pre-shuffle era: wire IDs from clif_packetdb.hpp explicit blocks.
	case packetver < 20040726:
		id = 0x0089 // clif_packetdb.hpp line 38: default baseline
	case packetver < 20040906:
		id = 0x0193 // >= 20040726
	case packetver < 20041129:
		id = 0x0085 // >= 20040906
	case packetver < 20050110:
		id = 0x009f // >= 20041129
	case packetver < 20080910:
		id = 0x0190 // >= 20050110
	case packetver < 20111102:
		id = 0x0437 // >= 20080910
	case packetver < 20120307:
		id = 0x08aa // >= 20111102
	case packetver < 20120410:
		id = 0x0885 // >= 20120307
	case packetver < 20120702:
		id = 0x0369 // >= 20120410
	case packetver < 20130320:
		id = 0x085a // >= 20120702
	case packetver < 20130515:
		id = 0x088e // >= 20130320 (clif_packetdb.hpp line 1622)
	default:
		// Shuffle era (>= 20130515): clif_shuffle.hpp covers every weekly
		// packetver with an exact PACKETVER == match; 0x0089 is present in
		// every block. For packetver > 20180307, returns 0x0437 (stable).
		id = shuffledCtoSID(packetver, 0x0089)
	}

	var p [7]byte
	p[0] = byte(id)
	p[1] = byte(id >> 8)
	leU32Put(p[2:], req.TargetGID) // rAthena: TargetGID
	p[6] = req.Action              // rAthena: Action
	return p
}
