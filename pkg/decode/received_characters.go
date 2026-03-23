// Hand-written: Characters field now decoded via decodeCharacterInfoList.
// Codegen emitted Characters = data[27:] (raw []byte — known codegen gap for nested struct arrays).
// Source: rAthena src/common/packets.hpp PACKET_HC_ACCEPT_ENTER (0x006B).

package decode

import "github.com/lenaxia/rathena-client/pkg/events"

// ReceivedCharacters_0x006B decodes a 0x006B packet (struct PACKET_HC_ACCEPT_ENTER).
func ReceivedCharacters_0x006B(data []byte, packetver uint32) events.ReceivedCharacters {
	var e events.ReceivedCharacters
	if packetver >= 20100413 {
		e.PacketLength = leI16(data, 2)                              // rAthena: packetLength (offset 2, size 2)
		e.Total = data[4]                                            // rAthena: total (offset 4, size 1)
		e.Premium_start = data[5]                                    // rAthena: premium_start (offset 5, size 1)
		e.Premium_end = data[6]                                      // rAthena: premium_end (offset 6, size 1)
		e.Extension = nullTermString(data[7:27])                     // rAthena: extension (offset 7, size 20)
		e.Characters = decodeCharacterInfoList(data[27:], packetver) // rAthena: characters[]
	} else {
		e.PacketLength = leI16(data, 2)                              // rAthena: packetLength (offset 2, size 2)
		e.Extension = nullTermString(data[4:24])                     // rAthena: extension (offset 4, size 20)
		e.Characters = decodeCharacterInfoList(data[24:], packetver) // rAthena: characters[]
	}
	return e
}
