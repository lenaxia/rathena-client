// Hand-written — packet 0x02C5/0x00FD added manually, regenerate when codegen is available.

package decode

import "github.com/lenaxia/rathena-client/pkg/events"

// ZcPartyJoinReqAck_0x02C5 decodes a 0x02C5 packet (struct PACKET_ZC_PARTY_JOIN_REQ_ACK).
func ZcPartyJoinReqAck_0x02C5(data []byte, packetver uint32) events.ZcPartyJoinReqAck {
	var e events.ZcPartyJoinReqAck
	_ = packetver
	e.CharacterName = nullTermString(data[2:26]) // rAthena: characterName (offset 2, size 24)
	e.Result = leI32(data, 26)                   // rAthena: result (offset 26, size 4)
	return e
}

// ZcPartyJoinReqAck_0x00FD decodes a 0x00FD packet (struct PACKET_ZC_PARTY_JOIN_REQ_ACK).
// The struct is identical at all packetvers (packets_struct.hpp:5093-5101) — only
// the header constant is gated (< 20070821 → 0x00fd); clif_party_invite_reply sends
// the 30-byte struct unconditionally (clif.cpp:7968-7974).
func ZcPartyJoinReqAck_0x00FD(data []byte, packetver uint32) events.ZcPartyJoinReqAck {
	var e events.ZcPartyJoinReqAck
	_ = packetver
	e.CharacterName = nullTermString(data[2:26]) // rAthena: characterName (offset 2, size 24)
	e.Result = leI32(data, 26)                   // rAthena: result (offset 26, size 4)
	return e
}
