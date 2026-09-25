// Hand-written — DO NOT regenerate without reviewing changes.

package events

// ZcPartyJoinReqAck is the event emitted for the zc_party_join_req_ack action.
type ZcPartyJoinReqAck struct {
	CharacterName string
	Result        int32
}
