// Manually implemented — triple bug in generated file: wrong ID, wrong size, wrong layout.
// See docs/WORKLOG/0073 for full cross-validation against rAthena and OpenKore.

package encode

import "github.com/lenaxia/rathena-client/pkg/send"

// EncodeLook encodes a direction change request (CZ_CHANGE_DIRECTION / CZ_CHANGE_DIRECTION2).
//
// Wire packet ID is determined by a two-stage lookup. The generated encoder had
// three bugs:
//  1. Wrong packet ID — hardcoded 0x009B (the shuffle base), never dispatched
//  2. Wrong return size — [4]byte instead of [5]byte (dropped the Dir field)
//  3. Wrong Dir offset — Dir written at p[3] but server reads it at offset 4
//
// Wire layout (5 bytes, all post-20101124 variants):
//
//	[0:2] packet ID (uint16 LE)
//	[2]   HeadDir (uint8)
//	[3]   padding (0x00) — rAthena pos[] skips from offset 2 to offset 4
//	[4]   Dir (uint8)
//
// rAthena confirmation: parseable_packet(0x0361, 5, clif_parse_ChangeDir, 2, 4)
// — RFIFOB at pos[0]=2 (HeadDir), RFIFOB at pos[1]=4 (Dir). Offset 3 is never read.
// OpenKore confirmation: pack 'v C' = headDir(uint16 LE)@[2:4] + dir(uint8)@[4].
// Since headDir is always 0-2, the high byte at p[3] is always 0x00 (padding).
//
// Sources:
//
//	clif_packetdb.hpp lines 48, 1383, 1401, 1440, 1493, 1559, 1591, 1623
//	clif_shuffle.hpp  — stable post-20180307: parseable_packet(0x0361, 5, clif_parse_ChangeDir, 2, 4)
//	worklog 0073      — rAthena + OpenKore cross-validation (actor_look_at 0361 ✓)
func EncodeLook(req send.Look, packetver uint32) [5]byte {
	var id uint16
	switch {
	case packetver < 20101124:
		id = 0x009B // baseline — clif_packetdb.hpp:48
	case packetver < 20111005:
		id = 0x0361 // clif_packetdb.hpp:1383
	case packetver < 20120307:
		id = 0x0366 // clif_packetdb.hpp:1401
	case packetver < 20120410:
		id = 0x0890 // clif_packetdb.hpp:1440
	case packetver < 20120418:
		id = 0x0871 // clif_packetdb.hpp:1493
	case packetver < 20120702:
		id = 0x0202 // clif_packetdb.hpp:1559
	case packetver < 20130320:
		id = 0x0960 // clif_packetdb.hpp:1591
	case packetver < 20130515:
		id = 0x0897 // clif_packetdb.hpp:1623
	default:
		id = shuffledCtoSID(packetver, 0x009B) // stable 0x0361 post-20180307
	}
	var p [5]byte
	p[0] = byte(id)
	p[1] = byte(id >> 8)
	p[2] = req.HeadDir // rAthena: headDir (uint8 at pos[0]=2)
	p[3] = 0x00        // padding — offset 3 is never read by rAthena
	p[4] = req.Dir     // rAthena: dir (uint8 at pos[1]=4)
	return p
}
