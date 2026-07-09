// Package decode — hand-maintained (codegen pipeline deprecated for this packet).
package decode

import "github.com/lenaxia/rathena-client/pkg/events"

// ZcNotifyMapproperty2_0x01D6 decodes a 0x01D6 packet (struct
// PACKET_ZC_NOTIFY_MAPPROPERTY2, packets.hpp:966-969) sent by clif_map_type
// (clif.cpp:6907-6914). 4 bytes: packetType(W) + type(W).
func ZcNotifyMapproperty2_0x01D6(data []byte, packetver uint32) events.ZcNotifyMapproperty2 {
	var e events.ZcNotifyMapproperty2
	_ = packetver
	e.Type = leI16(data, 2) // rAthena: type (clif.cpp:6911)
	return e
}

// ZcNotifyMapproperty2_0x099B decodes a 0x099B packet (ZC_MAPPROPERTY_R2) sent by
// clif_map_property (clif.cpp:6871-6903) for PACKETVER >= 20121010. Built with raw
// WBUFW/WBUFL macros into an 8-byte buffer (no C struct). Layout (clif.cpp:6881-6898):
// cmd(W,0) + property(W,2) + flags(L,4).
func ZcNotifyMapproperty2_0x099B(data []byte, packetver uint32) events.ZcNotifyMapproperty2 {
	var e events.ZcNotifyMapproperty2
	_ = packetver
	e.Type = leI16(data, 2)  // rAthena: property (clif.cpp:6882)
	e.Flags = leU32(data, 4) // rAthena: flags (clif.cpp:6888)
	return e
}

// ZcNotifyMapproperty2_0x0199 decodes a 0x0199 packet (ZC_NOTIFY_MAPPROPERTY) sent
// by clif_map_property (clif.cpp:6871-6903) for PACKETVER < 20121010. Built with
// raw WBUFW macros into a 4-byte buffer (no C struct). Layout (clif.cpp:6881-6882):
// cmd(W,0) + property(W,2).
func ZcNotifyMapproperty2_0x0199(data []byte, packetver uint32) events.ZcNotifyMapproperty2 {
	var e events.ZcNotifyMapproperty2
	_ = packetver
	e.Type = leI16(data, 2) // rAthena: property (clif.cpp:6882)
	return e
}
