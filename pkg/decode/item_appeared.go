// Hand-written additions: ItemAppeared_0x084B and ItemAppeared_0x0ADD were never
// generated. The decode logic exists inside ItemAppeared_0x009E's branches but is
// dead code there because the newer packet IDs are not dispatched to that function.
//
// dropflooritemType enum (packets_struct.hpp:130-136):
//   0x009E  pv <= ~20130000 (17 bytes — no type field)
//   0x084B  pv > 20130000, < 20180418 (19 bytes — adds type field)
//   0x0ADD  pv >= 20180418 (22 bytes — adds showdropeffect + dropeffectmode;
//            24 bytes at pv >= 20181121 via lengths_map_overrides.go override)
//
// All three IDs map to struct packet_dropflooritem (packets_struct.hpp:597-620).
// GCC-verified sizes confirmed; ITID uint16→uint32 change at pv >= 20181121
// is handled via the lengths_map_overrides.go override for 0x0ADD.

package decode

import "github.com/lenaxia/rathena-client/pkg/events"

// ItemAppeared_0x009E decodes a 0x009E packet (struct packet_dropflooritem).
// Active: pv <= ~20130000. Wire size: 17 bytes.
func ItemAppeared_0x009E(data []byte, packetver uint32) events.ItemAppeared {
	var e events.ItemAppeared
	e.ITAID = leU32(data, 2)        // rAthena: ITAID
	e.ITID = uint32(leU16(data, 6)) // rAthena: ITID (uint16)
	e.IsIdentified = data[8]        // rAthena: IsIdentified
	e.XPos = leI16(data, 9)         // rAthena: xPos
	e.YPos = leI16(data, 11)        // rAthena: yPos
	e.SubX = data[13]               // rAthena: subX
	e.SubY = data[14]               // rAthena: subY
	e.Count = leI16(data, 15)       // rAthena: count
	return e
}

// ItemAppeared_0x084B decodes a 0x084B packet (struct packet_dropflooritem).
// Active: pv > 20130000, < 20180418. Wire size: 19 bytes (adds type field).
// Source: packets_struct.hpp:597-620 at pv=20130320.
func ItemAppeared_0x084B(data []byte, packetver uint32) events.ItemAppeared {
	var e events.ItemAppeared
	_ = packetver
	e.ITAID = leU32(data, 2)        // rAthena: ITAID
	e.ITID = uint32(leU16(data, 6)) // rAthena: ITID (uint16 at this pv)
	e.Type = leU16(data, 8)         // rAthena: type (added pv > 20130000)
	e.IsIdentified = data[10]       // rAthena: IsIdentified
	e.XPos = leI16(data, 11)        // rAthena: xPos
	e.YPos = leI16(data, 13)        // rAthena: yPos
	e.SubX = data[15]               // rAthena: subX
	e.SubY = data[16]               // rAthena: subY
	e.Count = leI16(data, 17)       // rAthena: count
	return e
}

// ItemAppeared_0x0ADD decodes a 0x0ADD packet (struct packet_dropflooritem).
// Active: pv >= 20180418. Wire size: 22 bytes (ITID uint16, +showdropeffect +dropeffectmode);
// 24 bytes at pv >= 20181121 (ITID expands to uint32).
// The length table correctly reflects this via lengths_map_overrides.go.
// Source: packets_struct.hpp:597-620 at pv=20180418 and pv=20181121.
func ItemAppeared_0x0ADD(data []byte, packetver uint32) events.ItemAppeared {
	var e events.ItemAppeared
	e.ITAID = leU32(data, 2) // rAthena: ITAID
	if packetver >= 20181121 {
		e.ITID = leU32(data, 6)            // rAthena: ITID (uint32 at pv >= 20181121)
		e.Type = leU16(data, 10)           // rAthena: type
		e.IsIdentified = data[12]          // rAthena: IsIdentified
		e.XPos = leI16(data, 13)           // rAthena: xPos
		e.YPos = leI16(data, 15)           // rAthena: yPos
		e.SubX = data[17]                  // rAthena: subX
		e.SubY = data[18]                  // rAthena: subY
		e.Count = leI16(data, 19)          // rAthena: count
		e.Showdropeffect = int8(data[21])  // rAthena: showdropeffect
		e.Dropeffectmode = leI16(data, 22) // rAthena: dropeffectmode
	} else {
		e.ITID = uint32(leU16(data, 6))    // rAthena: ITID (uint16 at pv < 20181121)
		e.Type = leU16(data, 8)            // rAthena: type
		e.IsIdentified = data[10]          // rAthena: IsIdentified
		e.XPos = leI16(data, 11)           // rAthena: xPos
		e.YPos = leI16(data, 13)           // rAthena: yPos
		e.SubX = data[15]                  // rAthena: subX
		e.SubY = data[16]                  // rAthena: subY
		e.Count = leI16(data, 17)          // rAthena: count
		e.Showdropeffect = int8(data[19])  // rAthena: showdropeffect
		e.Dropeffectmode = leI16(data, 20) // rAthena: dropeffectmode
	}
	return e
}
