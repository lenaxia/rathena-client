// Hand-written: Characters field replaced with []CharacterInfoEntry.
// Codegen left Characters as []byte (flex array of nested struct — known codegen gap).
// Source: rAthena src/common/packets.hpp PACKET_HC_ACCEPT_ENTER (0x006B).

package events

// ReceivedCharacters is the event emitted for the received_characters action.
type ReceivedCharacters struct {
	PacketLength  int16
	Extension     string
	Characters    []CharacterInfoEntry
	Total         uint8
	Premium_start uint8
	Premium_end   uint8
}
