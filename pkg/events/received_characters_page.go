// Hand-written: Characters field replaced with []CharacterInfoEntry.
// Codegen left Characters as []byte (flex array of nested struct — known codegen gap).
// Source: rAthena src/common/packets.hpp PACKET_HC_ACK_CHARINFO_PER_PAGE (0x099D/0x0B72).

package events

// ReceivedCharactersPage is the event emitted for the received_characters_page action.
type ReceivedCharactersPage struct {
	PacketLength int16
	Characters   []CharacterInfoEntry
}
