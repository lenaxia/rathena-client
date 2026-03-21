// Hand-written: replaces generated AreaSpell_0x08C7 which used wrong offsets,
// and adds 0x099F and 0x09CA which were never generated.
//
// skill_entryType enum (packets_struct.hpp:121-127):
//   0x011F  pv < 20110718
//   0x08C7  pv >= 20110718, < 20121212   (19 bytes; fix: was reading 20)
//   0x099F  pv >= 20121212, < 20130731   (22 bytes)
//   0x09CA  pv >= 20130731               (23 bytes)
//
// GCC-verified sizes (packets_struct.hpp:1434-1454):
//   pv=20110718: PacketType(2)+PacketLength(2)+AID(4)+creatorAID(4)+xPos(2)+yPos(2)+job(1)+RadiusRange(1)+isVisible(1) = 19
//   pv=20121212: job expands to int32(+3) = 22
//   pv=20130731: +level(1) = 23

package decode

import "github.com/lenaxia/rathena-client/pkg/events"

// AreaSpell_0x011F decodes a 0x011F packet (struct packet_skill_entry).
func AreaSpell_0x011F(data []byte, packetver uint32) events.AreaSpell {
	var e events.AreaSpell
	if packetver >= 20130731 {
		e.PacketLength = leI16(data, 2) // rAthena: PacketLength
		e.AID = leU32(data, 4)          // rAthena: AID
		e.CreatorAID = leU32(data, 8)   // rAthena: creatorAID
		e.XPos = leI16(data, 12)        // rAthena: xPos
		e.YPos = leI16(data, 14)        // rAthena: yPos
		e.Job = leI32(data, 16)         // rAthena: job (int32)
		e.RadiusRange = int8(data[20])  // rAthena: RadiusRange
		e.IsVisible = data[21]          // rAthena: isVisible
		e.Level = data[22]              // rAthena: level
	} else if packetver >= 20121212 {
		e.PacketLength = leI16(data, 2) // rAthena: PacketLength
		e.AID = leU32(data, 4)          // rAthena: AID
		e.CreatorAID = leU32(data, 8)   // rAthena: creatorAID
		e.XPos = leI16(data, 12)        // rAthena: xPos
		e.YPos = leI16(data, 14)        // rAthena: yPos
		e.Job = leI32(data, 16)         // rAthena: job (int32)
		e.RadiusRange = int8(data[20])  // rAthena: RadiusRange
		e.IsVisible = data[21]          // rAthena: isVisible
	} else if packetver >= 20110718 {
		e.PacketLength = leI16(data, 2) // rAthena: PacketLength
		e.AID = leU32(data, 4)          // rAthena: AID
		e.CreatorAID = leU32(data, 8)   // rAthena: creatorAID
		e.XPos = leI16(data, 12)        // rAthena: xPos
		e.YPos = leI16(data, 14)        // rAthena: yPos
		e.Job = int32(int8(data[16]))   // rAthena: job (uint8)
		e.RadiusRange = int8(data[17])  // rAthena: RadiusRange
		e.IsVisible = data[18]          // rAthena: isVisible
	} else {
		e.AID = leU32(data, 2)        // rAthena: AID
		e.CreatorAID = leU32(data, 6) // rAthena: creatorAID
		e.XPos = leI16(data, 10)      // rAthena: xPos
		e.YPos = leI16(data, 12)      // rAthena: yPos
		e.Job = int32(int8(data[14])) // rAthena: job (uint8)
		e.IsVisible = data[15]        // rAthena: isVisible
	}
	return e
}

// AreaSpell_0x08C7 decodes a 0x08C7 packet (struct packet_skill_entry).
// Active: pv >= 20110718, < 20121212. Wire size: 19 bytes.
// Fix: replaces generated decoder that used SYNTH_ZC_SKILL_ENTRY3 offsets —
// read Range as leU16 and IsVisible from data[19] on a 19-byte packet (OOB).
// Source: packets_struct.hpp:1434-1454 at pv=20110718.
func AreaSpell_0x08C7(data []byte, packetver uint32) events.AreaSpell {
	var e events.AreaSpell
	_ = packetver
	e.PacketLength = leI16(data, 2) // rAthena: PacketLength
	e.AID = leU32(data, 4)          // rAthena: AID
	e.CreatorAID = leU32(data, 8)   // rAthena: creatorAID
	e.XPos = leI16(data, 12)        // rAthena: xPos
	e.YPos = leI16(data, 14)        // rAthena: yPos
	e.Job = int32(int8(data[16]))   // rAthena: job (uint8 at this pv)
	e.RadiusRange = int8(data[17])  // rAthena: RadiusRange (int8)
	e.IsVisible = data[18]          // rAthena: isVisible
	return e
}

// AreaSpell_0x099F decodes a 0x099F packet (struct packet_skill_entry).
// Active: pv >= 20121212, < 20130731. Wire size: 22 bytes (job expanded to int32).
// Source: packets_struct.hpp:1434-1454 at pv=20121212.
func AreaSpell_0x099F(data []byte, packetver uint32) events.AreaSpell {
	var e events.AreaSpell
	_ = packetver
	e.PacketLength = leI16(data, 2) // rAthena: PacketLength
	e.AID = leU32(data, 4)          // rAthena: AID
	e.CreatorAID = leU32(data, 8)   // rAthena: creatorAID
	e.XPos = leI16(data, 12)        // rAthena: xPos
	e.YPos = leI16(data, 14)        // rAthena: yPos
	e.Job = leI32(data, 16)         // rAthena: job (int32 at pv >= 20121212)
	e.RadiusRange = int8(data[20])  // rAthena: RadiusRange
	e.IsVisible = data[21]          // rAthena: isVisible
	return e
}

// AreaSpell_0x09CA decodes a 0x09CA packet (struct packet_skill_entry).
// Active: pv >= 20130731. Wire size: 23 bytes (adds level field).
// Source: packets_struct.hpp:1434-1454 at pv=20130731.
func AreaSpell_0x09CA(data []byte, packetver uint32) events.AreaSpell {
	var e events.AreaSpell
	_ = packetver
	e.PacketLength = leI16(data, 2) // rAthena: PacketLength
	e.AID = leU32(data, 4)          // rAthena: AID
	e.CreatorAID = leU32(data, 8)   // rAthena: creatorAID
	e.XPos = leI16(data, 12)        // rAthena: xPos
	e.YPos = leI16(data, 14)        // rAthena: yPos
	e.Job = leI32(data, 16)         // rAthena: job (int32)
	e.RadiusRange = int8(data[20])  // rAthena: RadiusRange
	e.IsVisible = data[21]          // rAthena: isVisible
	e.Level = data[22]              // rAthena: level (pv >= 20130731)
	return e
}
