// Hand-written: adds ActorStatusActive_0x0983 which was never generated.
//
// status_changeType enum (packets_struct.hpp:39-46):
//   0x0196  pv < 20090121 (sc_notickType — no timer fields; 9 bytes)
//   0x043F  pv >= 20090121, < 20120618 → ActionActorStatusEffectExtended (has Left+vals)
//   0x0983  pv >= 20120618 (packet_status_change — adds Total before Left; 29 bytes)
//
// GCC-verified layout of packet_status_change at pv=20120618
// (packets_struct.hpp: PacketType(2)+index(2)+AID(4)+state(1)+Total(4)+Left(4)+val1(4)+val2(4)+val3(4)):
//   offset 2: index, offset 4: AID, offset 8: state,
//   offset 9: Total, offset 13: Left, offset 17: val1, offset 21: val2, offset 25: val3

package decode

import "github.com/lenaxia/rathena-client/pkg/events"

// ActorStatusActive_0x0196 decodes a 0x0196 packet (struct packet_sc_notick).
// Active: pv < 20090121. Wire size: 9 bytes.
func ActorStatusActive_0x0196(data []byte, packetver uint32) events.ActorStatusActive {
	var e events.ActorStatusActive
	_ = packetver
	e.Index = leI16(data, 2) // rAthena: index
	e.AID = leU32(data, 4)   // rAthena: AID
	e.State = data[8]        // rAthena: state
	return e
}

// ActorStatusActive_0x0983 decodes a 0x0983 packet (struct packet_status_change).
// Active: pv >= 20120618. Wire size: 29 bytes.
// Adds Total (total duration ms) before Left (remaining ms), plus val1-val3.
// Source: packets_struct.hpp at pv=20120618.
func ActorStatusActive_0x0983(data []byte, packetver uint32) events.ActorStatusActive {
	var e events.ActorStatusActive
	_ = packetver
	e.Index = leI16(data, 2) // rAthena: index
	e.AID = leU32(data, 4)   // rAthena: AID
	e.State = data[8]        // rAthena: state
	e.Total = leU32(data, 9) // rAthena: Total (added pv >= 20120618)
	e.Left = leU32(data, 13) // rAthena: Left
	e.Val1 = leI32(data, 17) // rAthena: val1
	e.Val2 = leI32(data, 21) // rAthena: val2
	e.Val3 = leI32(data, 25) // rAthena: val3
	return e
}
