// Hand-written: List []byte replaced with Items []EquipItemEntry.
// See pkg/events/equip_item_entry.go for the decoded type.

package events

// InventoryItemsEquip is the event emitted for the inventory_items_equip action.
type InventoryItemsEquip struct {
	PacketLength int16
	Items        []EquipItemEntry
	InvType      uint8
}
