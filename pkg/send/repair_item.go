// Manually implemented — see GitHub issue #7 (EncodeRepairItem PACKETVER fix).
//
// RepairItem is the request struct for the repair_item action. Field types
// accommodate all three wire layouts derived from rAthena source
// (src/map/packets_struct.hpp):
//
//	REPAIRITEM_INFO1 narrow (PACKETVER < 20181121):
//	    index int16, itemId uint16, refine uint8, EQUIPSLOTINFO{uint16 card[4]}
//	REPAIRITEM_INFO1 wide (20181121 <= PACKETVER < 20191224):
//	    index int16, itemId uint32, refine uint8, EQUIPSLOTINFO{uint32 card[4]}
//	REPAIRITEM_INFO2 (PACKETVER >= 20191224):
//	    index int16, itemId uint32, EQUIPSLOTINFO{uint32 card[4]}, refine uint8, grade uint8
//
// The Go field types hold the widest representation needed. EncodeRepairItem
// narrows itemId and card[] to uint16/uint16[4] on the narrow wire layout
// (matching rAthena's uint16 truncation). Grade is only emitted on the
// REPAIR2 layout (PACKETVER >= 20191224); it is ignored for earlier packetvers.

package send

type RepairItem struct {
	Index  int16
	ItemId uint32
	Refine uint8
	Card   [4]uint32
	Grade  uint8
}
