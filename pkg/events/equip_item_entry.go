// Hand-written: codegen leaves EQUIPITEM_INFO list as []byte; this type decodes it.
// Source: rAthena src/map/packets_struct.hpp:457–507 (EQUIPITEM_INFO, ItemOptions)

package events

// ItemOption is the decoded form of one ItemOptions element.
//
// rAthena source: packets_struct.hpp:450–454
type ItemOption struct {
	Index int16 // rAthena: index
	Value int16 // rAthena: value
	Param uint8 // rAthena: param
}

// EquipItemEntry is the decoded form of one EQUIPITEM_INFO element from a
// packet_itemlist_equip packet (ActionInventoryItemsEquip).
//
// All packetver-conditional field widths are normalised to their widest form.
// Fields absent at a given packetver are zero-valued.
//
// rAthena source: packets_struct.hpp:457–507
type EquipItemEntry struct {
	Index           uint16        // rAthena: index (int16 on wire)
	ITID            uint32        // rAthena: ITID; uint16 on pv<20181121, widened
	Type            uint8         // rAthena: type
	Location        uint32        // rAthena: location; uint16 on pv<20120925, widened
	WearState       uint32        // rAthena: WearState; uint16 on pv<20120925, widened
	RefiningLevel   uint8         // rAthena: RefiningLevel; repositioned at pv>=20200916
	Cards           [4]uint32     // rAthena: slot.card[4]; uint16 per card on pv<20181121, widened
	HireExpireDate  int32         // rAthena: HireExpireDate; 0 if pv<20071002
	BindOnEquipType uint16        // rAthena: bindOnEquipType; 0 if pv<20080102
	SpriteNumber    uint16        // rAthena: wItemSpriteNumber; 0 if pv<20100629
	OptionCount     uint8         // rAthena: option_count; 0 if pv<20150226
	Options         [5]ItemOption // rAthena: option_data[MAX_ITEM_OPTIONS=5]; zero if pv<20150226
	Grade           uint8         // rAthena: grade; 0 if pv<20200916
	IsIdentified    uint8         // from IsIdentified field (pv<20120925) or Flag.IsIdentified bit
	IsDamaged       uint8         // from IsDamaged field (pv<20120925) or Flag.IsDamaged bit
	PlaceETCTab     uint8         // from Flag.PlaceETCTab bit (pv>=20120925), else 0
}
