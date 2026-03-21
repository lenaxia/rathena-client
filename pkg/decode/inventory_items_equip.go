// Hand-written: replaces codegen output which left EQUIPITEM_INFO list as []byte.
//
// Packet ID → rAthena inventorylistequipType enum (packets_struct.hpp:148–162):
//   0x00A4  pv < 20071002
//   0x0295  pv >= 20071002, < 20080102     (no decoder existed; now added)
//   0x02D0  pv >= 20080102, < 20120925     (no decoder existed; now added)
//   0x0992  pv >= 20120925, < 20150226
//   0x0A0D  pv >= 20150226, < 20181002 (MAIN)
//   0x0B0A  pv >= 20181002 (MAIN), < 20200916 (MAIN) ← invType at offset 4
//   0x0B39  pv >= 20200916 (MAIN)                    ← invType at offset 4
//
// EQUIPITEM_INFO wire sizes (packets_struct.hpp:457–507):
//   pv < 20071002:             20 bytes
//   pv >= 20071002, <20080102: 24 bytes
//   pv >= 20080102, <20100629: 26 bytes
//   pv >= 20100629, <20120925: 28 bytes
//   pv >= 20120925, <20150226: 31 bytes
//   pv >= 20150226, <20181121: 57 bytes
//   pv >= 20181121, <20200916: 67 bytes
//   pv >= 20200916:            68 bytes
//
// Allocation note: decodeEquipItems calls make([]EquipItemEntry, n) — one alloc
// per packet, unavoidable for variable-count arrays. Excluded from zero-alloc bench.

package decode

import "github.com/lenaxia/rathena-client/pkg/events"

// equipItemSize returns the wire size of one EQUIPITEM_INFO entry.
// Source: packets_struct.hpp:457–507, EQUIPSLOTINFO:410–416, ItemOptions:450–454.
func equipItemSize(pv uint32) int {
	switch {
	case pv >= 20200916:
		return 68 // +grade(1) relative to previous; RefiningLevel repositioned (net +1)
	case pv >= 20181121:
		return 67 // ITID uint32(+2), slot.card uint32[4](+8) relative to 57-byte layout
	case pv >= 20150226:
		return 57 // +option_count(1)+option_data[5*5](25) relative to 31-byte layout
	case pv >= 20120925:
		return 31 // index(2)+ITID(2)+type(1)+loc(4)+WearState(4)+RefiningLevel(1)+slot(8)+HireExpire(4)+bindOnEquip(2)+wItemSprite(2)+Flag(1)
	case pv >= 20100629:
		return 28 // +wItemSpriteNumber(2) relative to 26-byte layout
	case pv >= 20080102:
		return 26 // +bindOnEquipType(2) relative to 24-byte layout
	case pv >= 20071002:
		return 24 // +HireExpireDate(4) relative to 20-byte layout
	default:
		return 20 // index(2)+ITID(2)+type(1)+IsIdent(1)+loc(2)+WearState(2)+IsDamaged(1)+RefiningLevel(1)+slot(8)
	}
}

// decodeEquipItemEntry reads one EQUIPITEM_INFO from b into dst.
// b must be at least equipItemSize(pv) bytes long.
func decodeEquipItemEntry(dst *events.EquipItemEntry, b []byte, pv uint32) {
	dst.Index = leU16(b, 0) // rAthena: index (int16)

	off := 2
	if pv >= 20181121 {
		dst.ITID = leU32(b, off) // rAthena: ITID uint32
		off += 4
	} else {
		dst.ITID = uint32(leU16(b, off)) // rAthena: ITID uint16, widened
		off += 2
	}

	dst.Type = b[off] // rAthena: type
	off++

	if pv < 20120925 {
		dst.IsIdentified = b[off] // rAthena: IsIdentified (standalone field)
		off++
	}

	if pv >= 20120925 {
		dst.Location = leU32(b, off) // rAthena: location uint32
		off += 4
		dst.WearState = leU32(b, off) // rAthena: WearState uint32
		off += 4
	} else {
		dst.Location = uint32(leU16(b, off)) // rAthena: location uint16, widened
		off += 2
		dst.WearState = uint32(leU16(b, off)) // rAthena: WearState uint16, widened
		off += 2
	}

	if pv < 20120925 {
		dst.IsDamaged = b[off] // rAthena: IsDamaged (standalone field, pv<20120925)
		off++
	}

	if pv < 20200916 {
		dst.RefiningLevel = b[off] // rAthena: RefiningLevel (original position)
		off++
	}

	// EQUIPSLOTINFO slot — card[4]
	if pv >= 20181121 {
		dst.Cards[0] = leU32(b, off)
		dst.Cards[1] = leU32(b, off+4)
		dst.Cards[2] = leU32(b, off+8)
		dst.Cards[3] = leU32(b, off+12)
		off += 16
	} else {
		dst.Cards[0] = uint32(leU16(b, off))
		dst.Cards[1] = uint32(leU16(b, off+2))
		dst.Cards[2] = uint32(leU16(b, off+4))
		dst.Cards[3] = uint32(leU16(b, off+6))
		off += 8
	}

	if pv >= 20071002 {
		dst.HireExpireDate = leI32(b, off) // rAthena: HireExpireDate
		off += 4
	}

	if pv >= 20080102 {
		dst.BindOnEquipType = leU16(b, off) // rAthena: bindOnEquipType
		off += 2
	}

	if pv >= 20100629 {
		dst.SpriteNumber = leU16(b, off) // rAthena: wItemSpriteNumber
		off += 2
	}

	if pv >= 20150226 {
		dst.OptionCount = b[off] // rAthena: option_count
		off++
		for i := 0; i < 5; i++ {
			dst.Options[i].Index = leI16(b, off)   // rAthena: option_data[i].index
			dst.Options[i].Value = leI16(b, off+2) // rAthena: option_data[i].value
			dst.Options[i].Param = b[off+4]        // rAthena: option_data[i].param
			off += 5
		}
	}

	if pv >= 20200916 {
		dst.RefiningLevel = b[off] // rAthena: RefiningLevel (repositioned)
		off++
		dst.Grade = b[off] // rAthena: grade
		off++
	}

	if pv >= 20120925 {
		flag := b[off] // rAthena: Flag bitfield (1 byte)
		dst.IsIdentified = flag & 0x01
		dst.IsDamaged = (flag >> 1) & 0x01
		dst.PlaceETCTab = (flag >> 2) & 0x01
	}
}

// decodeEquipItems decodes the EQUIPITEM_INFO array in body into a slice.
func decodeEquipItems(body []byte, pv uint32) []events.EquipItemEntry {
	sz := equipItemSize(pv)
	n := len(body) / sz
	items := make([]events.EquipItemEntry, n)
	for i := range items {
		decodeEquipItemEntry(&items[i], body[i*sz:], pv)
	}
	return items
}

// InventoryItemsEquip_0x00A4 decodes a 0x00A4 packet (struct packet_itemlist_equip).
// Active: pv < 20071002.
func InventoryItemsEquip_0x00A4(data []byte, pv uint32) events.InventoryItemsEquip {
	var e events.InventoryItemsEquip
	e.PacketLength = leI16(data, 2) // rAthena: PacketLength
	e.Items = decodeEquipItems(data[4:], pv)
	return e
}

// InventoryItemsEquip_0x0295 decodes a 0x0295 packet (struct packet_itemlist_equip).
// Active: pv >= 20071002, < 20080102.
func InventoryItemsEquip_0x0295(data []byte, pv uint32) events.InventoryItemsEquip {
	var e events.InventoryItemsEquip
	e.PacketLength = leI16(data, 2) // rAthena: PacketLength
	e.Items = decodeEquipItems(data[4:], pv)
	return e
}

// InventoryItemsEquip_0x02D0 decodes a 0x02D0 packet (struct packet_itemlist_equip).
// Active: pv >= 20080102, < 20120925.
func InventoryItemsEquip_0x02D0(data []byte, pv uint32) events.InventoryItemsEquip {
	var e events.InventoryItemsEquip
	e.PacketLength = leI16(data, 2) // rAthena: PacketLength
	e.Items = decodeEquipItems(data[4:], pv)
	return e
}

// InventoryItemsEquip_0x0992 decodes a 0x0992 packet (struct packet_itemlist_equip).
// Active: pv >= 20120925, < 20150226.
func InventoryItemsEquip_0x0992(data []byte, pv uint32) events.InventoryItemsEquip {
	var e events.InventoryItemsEquip
	e.PacketLength = leI16(data, 2) // rAthena: PacketLength
	e.Items = decodeEquipItems(data[4:], pv)
	return e
}

// InventoryItemsEquip_0x0A0D decodes a 0x0A0D packet (struct packet_itemlist_equip).
// Active: pv >= 20150226, < 20181002 (MAIN).
func InventoryItemsEquip_0x0A0D(data []byte, pv uint32) events.InventoryItemsEquip {
	var e events.InventoryItemsEquip
	e.PacketLength = leI16(data, 2) // rAthena: PacketLength
	e.Items = decodeEquipItems(data[4:], pv)
	return e
}

// InventoryItemsEquip_0x0B0A decodes a 0x0B0A packet (struct packet_itemlist_equip).
// Active: pv >= 20181002 (MAIN), < 20200916. Adds invType at offset 4.
func InventoryItemsEquip_0x0B0A(data []byte, pv uint32) events.InventoryItemsEquip {
	var e events.InventoryItemsEquip
	e.PacketLength = leI16(data, 2) // rAthena: PacketLength
	e.InvType = data[4]             // rAthena: invType
	e.Items = decodeEquipItems(data[5:], pv)
	return e
}

// InventoryItemsEquip_0x0B39 decodes a 0x0B39 packet (struct packet_itemlist_equip).
// Active: pv >= 20200916 (MAIN). Adds invType at offset 4.
func InventoryItemsEquip_0x0B39(data []byte, pv uint32) events.InventoryItemsEquip {
	var e events.InventoryItemsEquip
	e.PacketLength = leI16(data, 2) // rAthena: PacketLength
	e.InvType = data[4]             // rAthena: invType
	e.Items = decodeEquipItems(data[5:], pv)
	return e
}
