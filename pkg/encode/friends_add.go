// Manually implemented — 0x0202 base ID is remapped in the shuffle era.
// See docs/WORKLOG/0074 for cross-validation against rAthena and OpenKore.

package encode

import "github.com/lenaxia/rathena-client/pkg/send"

// EncodeFriendsAdd encodes a friend request (CZ_ADD_FRIENDS).
//
// Wire packet ID:
//   - pv < 20130515: 0x0202 — single stable entry in clif_packetdb.hpp:259
//   - pv >= 20130515: shuffledCtoSID(pv, 0x0202)
//   - pv > 20180307: 0x0202 (post-shuffle stable — baseID returned for unknown entries)
//
// Wire format: always 26 bytes, name(char[24])@[2:26].
//
// Sources:
//
//	clif_packetdb.hpp:259 — parseable_packet(0x0202, 26, clif_parse_FriendsListAdd, 2)
//	clif_shuffle.hpp      — 0x0202 shuffled in every weekly block 20130515–20180307
//	worklog 0074          — rAthena + shuffle_map.go cross-validation
func EncodeFriendsAdd(req send.FriendsAdd, packetver uint32) [26]byte {
	var id uint16
	switch {
	case packetver < 20130515:
		id = 0x0202 // clif_packetdb.hpp:259 — stable pre-shuffle
	default:
		id = shuffledCtoSID(packetver, 0x0202) // stable 0x0202 post-20180307
	}
	var p [26]byte
	p[0] = byte(id)
	p[1] = byte(id >> 8)
	copy(p[2:26], req.Name) // rAthena: name (char[24])
	return p
}
