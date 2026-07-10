// Hand-written: decoders for PACKET_ZC_GROUP_LIST (issue #13).
//
// PACKET_ZC_GROUP_LIST (rAthena src/map/packets_struct.hpp:2086-2091) is the
// packet a server sends to a client to deliver the full party roster when the
// client joins an existing party. Three packet IDs cover three PACKETVER-
// conditional SUB struct layouts — one decoder per packet ID.
//
// GCC-verified SUB layouts (see zc_group_list_test.go for the GCC commands
// and per-packetver field offsets):
//
//	pv < 20170524 (0x00FB):  SUB = 46 bytes
//	    AID(4) + playerName[24] + mapName[16] + leader(1) + offline(1)
//
//	20170524 ≤ pv < 20171207 (0x0A44):  SUB = 50 bytes
//	    AID(4) + playerName[24] + mapName[16] + leader(1) + offline(1)
//	    + class_(2) + baseLevel(2)
//
//	pv ≥ 20171207 (0x0AE5, production):  SUB = 54 bytes
//	    AID(4) + GID(4) + playerName[24] + mapName[16] + leader(1) + offline(1)
//	    + class_(2) + baseLevel(2)
//
// The outer PACKET_ZC_GROUP_LIST header is constant: packetType(2) +
// packetLen(2) + partyName[24] = 28 bytes before members[].
//
// Wire ID selection (rAthena src/map/packets_struct.hpp:274-283):
//
//	#if PACKETVER >= 20171207        → partyinfo = 0x0ae5
//	#elif PACKETVER_MAIN_NUM >= 20170524
//	   || PACKETVER_RE_NUM >= 20170502
//	   || defined(PACKETVER_ZERO)    → partyinfo = 0x0a44
//	#else                             → partyinfo = 0x00fb
//
// The codegen only defines PACKETVER_MAIN_NUM; the RE/ZERO branches are
// approximated by the MAIN boundary for the purpose of these decoders. The
// dispatch table registers all three IDs at all packetvers (the dispatcher
// selects by packet ID on the wire, not by packetver), so any rAthena build
// will route to the correct decoder for the wire ID it actually emits.
//
// Allocation note: each decoder calls make([]ZcGroupListMember, n) — one
// heap alloc per packet, unavoidable for a variable-count array. This is a
// documented exception to the 0-alloc decode hot-path contract, matching the
// inventory list events (worklog 0066). The event struct itself does not
// escape to the heap.

package decode

import "github.com/lenaxia/rathena-client/pkg/events"

// zcGroupListHeaderSize is the byte size of the outer PACKET_ZC_GROUP_LIST
// header before members[]: packetType(2) + packetLen(2) + partyName[24].
const zcGroupListHeaderSize = 28

// zcGroupListMemberSize returns the byte size of one PACKET_ZC_GROUP_LIST_SUB
// entry at the given packetver.
//
// Mirrors the #if ladder in rAthena src/map/packets_struct.hpp:2073-2083:
//   - pv >= 20171207 adds GID(4) after AID
//   - pv >= 20170524 (MAIN) adds class_(2) + baseLevel(2) at the end
func zcGroupListMemberSize(pv uint32) int {
	switch {
	case pv >= 20171207:
		return 54
	case pv >= 20170524:
		return 50
	default:
		return 46
	}
}

// decodeZcGroupListMember reads one PACKET_ZC_GROUP_LIST_SUB from b at offset
// off into dst. b must have at least zcGroupListMemberSize(pv) bytes
// available from off. Field offsets are packetver-conditional per the
// GCC-verified layouts above.
//
// rAthena encodes the leader/offline bytes inverted relative to the
// intuitive bool (clif.cpp:7892-7893):
//
//	member.leader  = (m.leader)  ? 0 : 1;   // 0 = leader, 1 = normal
//	member.offline = (m.online)  ? 0 : 1;   // 0 = online, 1 = offline
//
// decodeZcGroupListMember flips them back to the intuitive Go bool.
func decodeZcGroupListMember(dst *events.ZcGroupListMember, b []byte, pv uint32) {
	cur := 0
	dst.AID = leU32(b, cur) // rAthena: AID
	cur += 4
	if pv >= 20171207 {
		dst.GID = leU32(b, cur) // rAthena: GID
		cur += 4
	}
	dst.Name = nullTermString(b[cur : cur+24]) // rAthena: playerName
	cur += 24
	dst.MapName = nullTermString(b[cur : cur+16]) // rAthena: mapName
	cur += 16
	dst.Leader = b[cur] == 0 // rAthena: leader (0 = leader, 1 = normal)
	cur += 1
	dst.Offline = b[cur] != 0 // rAthena: offline (0 = online, 1 = offline)
	cur += 1
	if pv >= 20170524 {
		dst.Class = leI16(b, cur) // rAthena: class_
		cur += 2
		dst.BaseLevel = leI16(b, cur) // rAthena: baseLevel
		cur += 2
	}
}

// decodeZcGroupListRoster decodes the variable-length members[] slice from
// body (which starts at the first member). It drops any partial trailing
// member (when len(body) is not a clean multiple of the per-member size)
// rather than reading past the buffer.
func decodeZcGroupListRoster(body []byte, pv uint32) []events.ZcGroupListMember {
	sz := zcGroupListMemberSize(pv)
	n := len(body) / sz
	if n == 0 {
		return nil
	}
	members := make([]events.ZcGroupListMember, n)
	for i := range members {
		decodeZcGroupListMember(&members[i], body[i*sz:], pv)
	}
	return members
}

// ZcGroupList_0x00FB decodes a 0x00FB packet (PACKET_ZC_GROUP_LIST, oldest
// layout). Active for PACKETVER < 20170524 (MAIN) / 20170502 (RE).
func ZcGroupList_0x00FB(data []byte, pv uint32) events.ZcGroupList {
	var e events.ZcGroupList
	e.PacketLength = leI16(data, 2) // rAthena: packetLen
	e.PartyName = nullTermString(data[4 : 4+24])
	e.Members = decodeZcGroupListRoster(data[zcGroupListHeaderSize:], pv)
	return e
}

// ZcGroupList_0x0A44 decodes a 0x0A44 packet (PACKET_ZC_GROUP_LIST, mid
// layout). Active for 20170524 ≤ PACKETVER < 20171207 (MAIN) — RE/ZERO
// approximated by the MAIN boundary.
func ZcGroupList_0x0A44(data []byte, pv uint32) events.ZcGroupList {
	var e events.ZcGroupList
	e.PacketLength = leI16(data, 2) // rAthena: packetLen
	e.PartyName = nullTermString(data[4 : 4+24])
	e.Members = decodeZcGroupListRoster(data[zcGroupListHeaderSize:], pv)
	return e
}

// ZcGroupList_0x0AE5 decodes a 0x0AE5 packet (PACKET_ZC_GROUP_LIST, newest
// layout, production target). Active for PACKETVER ≥ 20171207 — this is the
// wire ID at production packetver pv=20200401.
func ZcGroupList_0x0AE5(data []byte, pv uint32) events.ZcGroupList {
	var e events.ZcGroupList
	e.PacketLength = leI16(data, 2) // rAthena: packetLen
	e.PartyName = nullTermString(data[4 : 4+24])
	e.Members = decodeZcGroupListRoster(data[zcGroupListHeaderSize:], pv)
	return e
}
