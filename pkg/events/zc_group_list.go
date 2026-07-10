// Hand-written: event struct for the zc_group_list action (issue #13).
//
// PACKET_ZC_GROUP_LIST (rAthena src/map/packets_struct.hpp:2086-2091) is the
// packet a server sends to a client to deliver the full party roster when the
// client joins an existing party (rAthena src/map/party.cpp:676,
// party_member_added → clif_party_info). Three packet IDs cover three eras of
// PACKETVER-conditional struct layouts (see pkg/decode/zc_group_list.go for
// the per-era SUB struct sizes).
//
// Allocation note: like the inventory list events (worklog 0066), the
// Members slice is a single make([]ZcGroupListMember, n) per packet — one
// heap alloc, unavoidable for a variable-count array. Documented as a known
// exception to the 0-alloc decode hot-path contract.

package events

// ZcGroupList is the event emitted when a ZC_GROUP_LIST packet is received.
//
// The server sends this to a client to announce the full party roster at the
// moment the client joins an existing party (party_member_added →
// clif_party_info, rAthena src/map/clif.cpp:7853). It is the only way to
// learn about members who were already in the party when the local player
// joined — the per-member spawn packets (0x00AB / 0x0ABD
// ZC_NOTIFY_MEMBERINFO_TO_GROUPM) are NOT re-sent for pre-existing members,
// so without decoding this packet those members remain invisible to the bot.
type ZcGroupList struct {
	// PacketLength is the wire-declared packet length (int16 at offset 2).
	// Carried through to consumers for diagnostics; not needed for parsing
	// since Members is already decoded.
	PacketLength int16
	// PartyName is the party's display name (char[24], NUL-padded).
	PartyName string
	// Members is the decoded roster. Order matches the wire order, which
	// rAthena fills by slot index (skipping empty slots).
	Members []ZcGroupListMember
}

// ZcGroupListMember is one entry in the PACKET_ZC_GROUP_LIST roster.
//
// Fields absent at older PACKETVERs decode to their zero value:
//   - GID is zero for PACKETVER < 20171207 (the GID field was added in the
//     0x0AE5 layout; the older 0x00FB and 0x0A44 layouts have no GID).
//   - Class and BaseLevel are zero for PACKETVER < 20170524 (MAIN) /
//     20170502 (RE) — those fields were added in the 0x0A44 layout; the
//     oldest 0x00FB layout has neither.
type ZcGroupListMember struct {
	// AID is the member's account ID (rAthena: AID, uint32). Always present.
	AID uint32
	// GID is the member's character ID (rAthena: GID, uint32). Present only
	// at PACKETVER >= 20171207 (0x0AE5 layout); zero otherwise.
	GID uint32
	// Name is the member's character name (rAthena: playerName, char[24]).
	Name string
	// MapName is the member's current map (rAthena: mapName,
	// char[MAP_NAME_LENGTH_EXT=16]). rAthena fills this via
	// mapindex_getmapname_ext, which produces a ".gat"-suffixed map name
	// at older packetvers and a bare map name at newer ones.
	MapName string
	// Leader reports whether this member is the party leader. rAthena's
	// encoding is inverted (leader byte is 0 for the leader, 1 for normal
	// members — see clif.cpp:7892); the decoder flips it back to the
	// intuitive bool.
	Leader bool
	// Offline reports whether this member is currently offline. rAthena's
	// encoding is inverted (offline byte is 0 for online, 1 for offline —
	// see clif.cpp:7893); the decoder flips it back to the intuitive bool.
	Offline bool
	// Class is the member's job class ID (rAthena: class_, int16). Present
	// only at PACKETVER >= 20170524 (MAIN) / 20170502 (RE); zero otherwise.
	Class int16
	// BaseLevel is the member's base level (rAthena: baseLevel, int16).
	// Present only at PACKETVER >= 20170524 (MAIN) / 20170502 (RE); zero
	// otherwise.
	BaseLevel int16
}
