// Manually implemented — worklog 0072.
// EncodeEnterWorld is the public encoder for CZ_NOTIFY_ACTORINIT (0x007D).
// It replaces the FSM-internal fsmEncodeMapLoaded() for the goKore consumer.

package encode

import "github.com/lenaxia/rathena-client/pkg/send"

// EncodeEnterWorld encodes a CZ_NOTIFY_ACTORINIT (0x007D) map-loaded signal.
//
// This is a fixed 2-byte signal packet with no payload fields. It is sent by
// the client to confirm receipt of ZC_ACCEPT_ENTER and trigger the server to
// begin the initial data burst (inventory, skills, actor spawns, stats).
//
// 0x007D is never shuffled:
//   - Single entry in clif_packetdb.hpp:32 with no PACKETVER overrides
//   - Absent from clif_shuffle.hpp
//   - Stable across all known packetvers
//
// The packetver argument is accepted for API consistency but is unused.
//
// Sources:
//
//	clif_packetdb.hpp:32 — packet(0x7d, 2)
//	clif.cpp:10742       — clif_parse_LoadEndAck (the triggered handler)
//	pkg/session/fsm.go:fsmEncodeMapLoaded — internal FSM equivalent
func EncodeEnterWorld(_ send.EnterWorld, _ uint32) [2]byte {
	return [2]byte{0x7D, 0x00}
}
