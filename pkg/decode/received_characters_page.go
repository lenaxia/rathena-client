// Hand-written: Characters field now decoded via decodeCharacterInfoList.
// Codegen emitted Characters = data[4:] (raw []byte — known codegen gap for nested struct arrays).
// Source: rAthena src/common/packets.hpp PACKET_HC_ACK_CHARINFO_PER_PAGE (0x099D/0x0B72).

package decode

import "github.com/lenaxia/rathena-client/pkg/events"

// ReceivedCharactersPage_0x099D decodes a 0x099D packet (struct PACKET_HC_ACK_CHARINFO_PER_PAGE).
func ReceivedCharactersPage_0x099D(data []byte, packetver uint32) events.ReceivedCharactersPage {
	var e events.ReceivedCharactersPage
	e.PacketLength = leI16(data, 2)                             // rAthena: packetLength (offset 2, size 2)
	e.Characters = decodeCharacterInfoList(data[4:], packetver) // rAthena: characters[]
	return e
}

// ReceivedCharactersPage_0x0B72 decodes a 0x0B72 packet (struct PACKET_HC_ACK_CHARINFO_PER_PAGE).
func ReceivedCharactersPage_0x0B72(data []byte, packetver uint32) events.ReceivedCharactersPage {
	var e events.ReceivedCharactersPage
	e.PacketLength = leI16(data, 2)                             // rAthena: packetLength (offset 2, size 2)
	e.Characters = decodeCharacterInfoList(data[4:], packetver) // rAthena: characters[]
	return e
}
