// Hand-written: adds Total, Left, Val1-Val3 fields for 0x0983 (packet_status_change
// at pv >= 20120618 which adds Total before Left).
// 0x0196 (packet_sc_notick): only Index, AID, State — timer fields are zero.
// 0x0983 (packet_status_change pv >= 20120618): all fields populated.

package events

// ActorStatusActive is the event emitted for the actor_status_active action.
// For 0x0196 (sc_notickType, pv < 20090121): only Index, AID, State are populated.
// For 0x0983 (status_changeType, pv >= 20120618): all fields including Total, Left, Val1-Val3.
type ActorStatusActive struct {
	Index int16
	AID   uint32
	State uint8
	Total uint32 // rAthena: Total — duration ms; 0 for 0x0196
	Left  uint32 // rAthena: Left — remaining ms; 0 for 0x0196
	Val1  int32
	Val2  int32
	Val3  int32
}
