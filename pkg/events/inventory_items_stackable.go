// Hand-written: List []byte replaced with Items []NormalItemEntry.
// See pkg/events/normal_item_entry.go for the decoded type.

package events

// InventoryItemsStackable is the event emitted for the inventory_items_stackable action.
type InventoryItemsStackable struct {
	PacketLength int16
	Items        []NormalItemEntry
	InvType      uint8
}
