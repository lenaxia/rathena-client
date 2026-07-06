// Manually implemented — see GitHub issue #7 (EncodeRepairItem PACKETVER fix)
// and the related worklog for the repair_item PACKETVER fix.
//
// This file replaces the codegen output. The codegen produced a single-layout
// encoder because the semantics DB entry for repair_item (semantics/mappings.yaml
// line 3637) carries no field-level metadata and no packetver_range, so the
// generator could not see the PACKETVER-conditional widening in rAthena source.
// The DB cleanup is a separate (non-blocking) task tracked in the worklog.

package encode

import (
	"github.com/lenaxia/rathena-client/pkg/send"
)

// Packetver boundaries. Verified empirically by GCC preprocessing of
// src/map/packets_struct.hpp at PACKETVER in {20180307, 20181120, 20181121,
// 20191223, 20191224, 20200401, 20200916} and by reading the packetdb
// registration guards in src/map/clif_packetdb.hpp:
//
//   - EQUIPSLOTINFO.card widens from uint16[4] to uint32[4] when
//     PACKETVER_MAIN_NUM >= 20181121 (src/map/packets_struct.hpp:411).
//   - REPAIRITEM_INFO1.itemId widens from uint16 to uint32 under the same
//     condition (src/map/packets_struct.hpp:2927).
//   - PACKET_CZ_REQ_ITEMREPAIR2 (0x0B66) is defined AND registered only when
//     PACKETVER >= 20191224 (src/map/packets_struct.hpp:2936, 2942 and
//     src/map/clif_packetdb.hpp:1975).
//
// PACKET_CZ_REQ_ITEMREPAIR1 (0x01FD) is registered with no packetver guard
// (src/map/clif_packetdb.hpp:256) so it is always accepted by the server.
// 0x01FD and 0x0B66 are NOT in clif_shuffle.hpp, so no C→S shuffle applies.
// C→S packets are not subject to PACKET_OBFUSCATION (S→C only).
//
// Note on the clif.cpp dispatcher (src/map/clif.cpp:13271): it picks the
// struct pointer cast using #if PACKETVER_MAIN_NUM >= 20200916, which differs
// from the packetdb registration boundary (20191224). This is a server-side
// ambiguity but does not affect wire correctness because the dispatcher only
// reads p->item.index, which sits at offset 2 in both REPAIR1 and REPAIR2.
// We emit 0x0B66 starting at 20191224 to match the packetdb registration
// boundary (the binding contract for which packet IDs the server accepts).
const (
	repairItemWideFieldsPV uint32 = 20181121 // >= → itemId uint32, card uint32[4]
	repairItemRepair2PV    uint32 = 20191224 // >= → emit 0x0B66 (REPAIRITEM_INFO2)
)

// EncodeRepairItem encodes a repair_item request for sending to the server.
// The wire packet ID, total length, and field layout all depend on PACKETVER:
//
//	pv < 20181121:               PACKET_CZ_REQ_ITEMREPAIR1 (0x01FD), 15 bytes
//	20181121 <= pv < 20191224:   PACKET_CZ_REQ_ITEMREPAIR1 (0x01FD), 25 bytes
//	pv >= 20191224:              PACKET_CZ_REQ_ITEMREPAIR2 (0x0B66), 26 bytes
//
// The returned slice has length 15, 25, or 26 depending on packetver.
func EncodeRepairItem(req send.RepairItem, packetver uint32) []byte {
	switch {
	case packetver >= repairItemRepair2PV:
		// PACKET_CZ_REQ_ITEMREPAIR2 (0x0B66) with REPAIRITEM_INFO2.
		// Layout: id(2) index(2) itemId(4) slot.card[0..3](16) refine(1) grade(1) = 26
		// NOTE: slot comes BEFORE refine, and grade is appended — different
		// field order than REPAIR1.
		var p [26]byte
		p[0] = 0x66 // rAthena: packetType (0x0B66 LE)
		p[1] = 0x0b // rAthena: packetType
		leU16Put(p[2:], uint16(req.Index))           // rAthena: item.index
		leU32Put(p[4:], req.ItemId)                  // rAthena: item.itemId
		leU32Put(p[8:], req.Card[0])                 // rAthena: item.slot.card[0]
		leU32Put(p[12:], req.Card[1])                // rAthena: item.slot.card[1]
		leU32Put(p[16:], req.Card[2])                // rAthena: item.slot.card[2]
		leU32Put(p[20:], req.Card[3])                // rAthena: item.slot.card[3]
		p[24] = req.Refine                           // rAthena: item.refine
		p[25] = req.Grade                            // rAthena: item.grade
		return p[:]
	default:
		// PACKET_CZ_REQ_ITEMREPAIR1 (0x01FD) with REPAIRITEM_INFO1.
		// Layout: id(2) index(2) itemId(2 or 4) refine(1) slot(8 or 16).
		if packetver >= repairItemWideFieldsPV {
			// Wide: 25 bytes total.
			var p [25]byte
			p[0] = 0xfd // rAthena: packetType (0x01FD LE)
			p[1] = 0x01 // rAthena: packetType
			leU16Put(p[2:], uint16(req.Index))  // rAthena: item.index
			leU32Put(p[4:], req.ItemId)         // rAthena: item.itemId
			p[8] = req.Refine                   // rAthena: item.refine
			leU32Put(p[9:], req.Card[0])        // rAthena: item.slot.card[0]
			leU32Put(p[13:], req.Card[1])       // rAthena: item.slot.card[1]
			leU32Put(p[17:], req.Card[2])       // rAthena: item.slot.card[2]
			leU32Put(p[21:], req.Card[3])       // rAthena: item.slot.card[3]
			return p[:]
		}
		// Narrow: 15 bytes total. itemId and cards truncate to uint16
		// (matches rAthena EQUIPSLOTINFO uint16 encoding for pv < 20181121).
		var p [15]byte
		p[0] = 0xfd // rAthena: packetType (0x01FD LE)
		p[1] = 0x01 // rAthena: packetType
		leU16Put(p[2:], uint16(req.Index))                       // rAthena: item.index
		leU16Put(p[4:], uint16(req.ItemId))                      // rAthena: item.itemId
		p[6] = req.Refine                                        // rAthena: item.refine
		leU16Put(p[7:], uint16(req.Card[0]))                     // rAthena: item.slot.card[0]
		leU16Put(p[9:], uint16(req.Card[1]))                     // rAthena: item.slot.card[1]
		leU16Put(p[11:], uint16(req.Card[2]))                    // rAthena: item.slot.card[2]
		leU16Put(p[13:], uint16(req.Card[3]))                    // rAthena: item.slot.card[3]
		return p[:]
	}
}
