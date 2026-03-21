// Hand-written: replaces codegen output which left NORMALITEM_INFO list as []byte.
//
// Packet ID → rAthena inventorylistnormalType enum (packets_struct.hpp:137–147):
//   0x00A3  pv < 20071002
//   0x01EE  pv >= 20071002, < 20080102
//   0x02E8  pv >= 20080102, < 20120925
//   0x0991  pv >= 20120925, < 20181002 (MAIN)
//   0x0B09  pv >= 20181002 (MAIN)   ← adds invType byte at offset 4
//
// NORMALITEM_INFO wire sizes (packets_struct.hpp:418–448):
//   pv < 20080102:           18 bytes
//   pv >= 20080102, <20120925: 22 bytes
//   pv >= 20120925, <20181121: 24 bytes
//   pv >= 20181121:            34 bytes
//
// Allocation note: decodeNormalItems calls make([]NormalItemEntry, n) — one alloc
// per packet, unavoidable for variable-count arrays. Excluded from zero-alloc bench.

package decode

import "github.com/lenaxia/rathena-client/pkg/events"

// normalItemSize returns the wire size of one NORMALITEM_INFO entry.
// Source: packets_struct.hpp:418–448, EQUIPSLOTINFO:410–416.
func normalItemSize(pv uint32) int {
	switch {
	case pv >= 20181121:
		return 34 // index(2)+ITID(4)+type(1)+count(2)+WearState(4)+slot.card[4*4](16)+HireExpireDate(4)+Flag(1)
	case pv >= 20120925:
		return 24 // index(2)+ITID(2)+type(1)+count(2)+WearState(4)+slot.card[4*2](8)+HireExpireDate(4)+Flag(1)
	case pv >= 20080102:
		return 22 // index(2)+ITID(2)+type(1)+IsIdentified(1)+count(2)+WearState(2)+slot.card[4*2](8)+HireExpireDate(4)
	default:
		return 18 // index(2)+ITID(2)+type(1)+IsIdentified(1)+count(2)+WearState(2)+slot.card[4*2](8)
	}
}

// decodeNormalItemEntry reads one NORMALITEM_INFO from b into dst.
// b must be at least normalItemSize(pv) bytes long.
func decodeNormalItemEntry(dst *events.NormalItemEntry, b []byte, pv uint32) {
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

	dst.Count = uint16(leI16(b, off)) // rAthena: count (int16 on wire)
	off += 2

	if pv >= 20120925 {
		dst.WearState = leU32(b, off) // rAthena: WearState uint32
		off += 4
	} else {
		dst.WearState = uint32(leU16(b, off)) // rAthena: WearState uint16, widened
		off += 2
	}

	// EQUIPSLOTINFO slot — card[4] (packets_struct.hpp:410–416)
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

	if pv >= 20080102 {
		dst.HireExpireDate = leI32(b, off) // rAthena: HireExpireDate
		off += 4
	}

	if pv >= 20120925 {
		flag := b[off] // rAthena: Flag bitfield (1 byte)
		dst.IsIdentified = flag & 0x01
		dst.PlaceETCTab = (flag >> 1) & 0x01
	}
}

// decodeNormalItems decodes the NORMALITEM_INFO array in body into a slice.
// body is the packet bytes after the header (and after invType if present).
func decodeNormalItems(body []byte, pv uint32) []events.NormalItemEntry {
	sz := normalItemSize(pv)
	n := len(body) / sz
	items := make([]events.NormalItemEntry, n)
	for i := range items {
		decodeNormalItemEntry(&items[i], body[i*sz:], pv)
	}
	return items
}

// InventoryItemsStackable_0x00A3 decodes a 0x00A3 packet (struct packet_itemlist_normal).
// Active: pv < 20071002.
func InventoryItemsStackable_0x00A3(data []byte, pv uint32) events.InventoryItemsStackable {
	var e events.InventoryItemsStackable
	e.PacketLength = leI16(data, 2) // rAthena: PacketLength
	e.Items = decodeNormalItems(data[4:], pv)
	return e
}

// InventoryItemsStackable_0x01EE decodes a 0x01EE packet (struct packet_itemlist_normal).
// Active: pv >= 20071002, < 20080102.
func InventoryItemsStackable_0x01EE(data []byte, pv uint32) events.InventoryItemsStackable {
	var e events.InventoryItemsStackable
	e.PacketLength = leI16(data, 2) // rAthena: PacketLength
	e.Items = decodeNormalItems(data[4:], pv)
	return e
}

// InventoryItemsStackable_0x02E8 decodes a 0x02E8 packet (struct packet_itemlist_normal).
// Active: pv >= 20080102, < 20120925.
func InventoryItemsStackable_0x02E8(data []byte, pv uint32) events.InventoryItemsStackable {
	var e events.InventoryItemsStackable
	e.PacketLength = leI16(data, 2) // rAthena: PacketLength
	e.Items = decodeNormalItems(data[4:], pv)
	return e
}

// InventoryItemsStackable_0x0991 decodes a 0x0991 packet (struct packet_itemlist_normal).
// Active: pv >= 20120925, < 20181002 (MAIN).
func InventoryItemsStackable_0x0991(data []byte, pv uint32) events.InventoryItemsStackable {
	var e events.InventoryItemsStackable
	e.PacketLength = leI16(data, 2) // rAthena: PacketLength
	e.Items = decodeNormalItems(data[4:], pv)
	return e
}

// InventoryItemsStackable_0x0B09 decodes a 0x0B09 packet (struct packet_itemlist_normal).
// Active: pv >= 20181002 (MAIN). Adds invType byte at offset 4; items start at offset 5.
// Source: packets_struct.hpp:1187–1194.
func InventoryItemsStackable_0x0B09(data []byte, pv uint32) events.InventoryItemsStackable {
	var e events.InventoryItemsStackable
	e.PacketLength = leI16(data, 2) // rAthena: PacketLength
	e.InvType = data[4]             // rAthena: invType (pv >= 20181002 MAIN)
	e.Items = decodeNormalItems(data[5:], pv)
	return e
}
