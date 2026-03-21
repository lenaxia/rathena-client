// Hand-written: codegen leaves NORMALITEM_INFO list as []byte; this type decodes it.
// Source: rAthena src/map/packets_struct.hpp:418–448 (NORMALITEM_INFO)

package events

// NormalItemEntry is the decoded form of one NORMALITEM_INFO element from a
// packet_itemlist_normal packet (ActionInventoryItemsStackable).
//
// All packetver-conditional field widths are normalised to their widest form so
// the caller never has to branch on packetver.
//
// rAthena source: packets_struct.hpp:418–448
type NormalItemEntry struct {
	Index          uint16    // rAthena: index (int16, always present)
	ITID           uint32    // rAthena: ITID; uint16 on pv<20181121, widened to uint32
	Type           uint8     // rAthena: type
	Count          uint16    // rAthena: count (int16 on wire, narrowed to uint16)
	WearState      uint32    // rAthena: WearState; uint16 on pv<20120925, widened to uint32
	Cards          [4]uint32 // rAthena: slot.card[4]; uint16 per card on pv<20181121, widened
	HireExpireDate int32     // rAthena: HireExpireDate; 0 if pv<20080102
	IsIdentified   uint8     // from IsIdentified field (pv<20120925) or Flag.IsIdentified bit
	PlaceETCTab    uint8     // from Flag.PlaceETCTab bit (pv>=20120925), else 0
}
