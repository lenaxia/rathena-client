// Tests for NORMALITEM_INFO and EQUIPITEM_INFO decoders.
//
// Golden bytes are synthesised directly from the rAthena struct definitions
// (packets_struct.hpp) by laying out each field at its correct offset for the
// given packetver. This validates both normalItemSize/equipItemSize and the
// field-extraction logic.
//
// Allocation benchmarks: one alloc per call is expected and documented (see HLD
// §known-exceptions). These benchmarks capture the alloc count for reference, not
// as a zero-alloc assertion.

package decode

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/events"
)

// ── NORMALITEM_INFO helpers ───────────────────────────────────────────────────

// buildNormalItem builds one NORMALITEM_INFO wire entry at the given packetver.
// Field values are chosen to be distinct so misreads are visible.
func buildNormalItem(pv uint32, index int16, itid uint32, typ uint8, count int16,
	wearState uint32, cards [4]uint32, hireExpire int32, isIdentified uint8, placeETC uint8) []byte {

	sz := normalItemSize(pv)
	b := make([]byte, sz)
	off := 0

	binary.LittleEndian.PutUint16(b[off:], uint16(index))
	off += 2

	if pv >= 20181121 {
		binary.LittleEndian.PutUint32(b[off:], itid)
		off += 4
	} else {
		binary.LittleEndian.PutUint16(b[off:], uint16(itid))
		off += 2
	}

	b[off] = typ
	off++

	if pv < 20120925 {
		b[off] = isIdentified
		off++
	}

	binary.LittleEndian.PutUint16(b[off:], uint16(count))
	off += 2

	if pv >= 20120925 {
		binary.LittleEndian.PutUint32(b[off:], wearState)
		off += 4
	} else {
		binary.LittleEndian.PutUint16(b[off:], uint16(wearState))
		off += 2
	}

	if pv >= 20181121 {
		for i := 0; i < 4; i++ {
			binary.LittleEndian.PutUint32(b[off:], cards[i])
			off += 4
		}
	} else {
		for i := 0; i < 4; i++ {
			binary.LittleEndian.PutUint16(b[off:], uint16(cards[i]))
			off += 2
		}
	}

	if pv >= 20080102 {
		binary.LittleEndian.PutUint32(b[off:], uint32(hireExpire))
		off += 4
	}

	if pv >= 20120925 {
		flag := isIdentified&0x01 | (placeETC&0x01)<<1
		b[off] = flag
	}

	return b
}

// buildNormalPacket builds a complete packet_itemlist_normal wire packet.
// hasInvType is true for 0x0B09 (pv >= 20181002 MAIN).
func buildNormalPacket(packetID uint16, pv uint32, hasInvType bool, invType uint8,
	items [][]byte) []byte {

	headerSize := 4
	if hasInvType {
		headerSize = 5
	}
	total := headerSize
	for _, item := range items {
		total += len(item)
	}

	pkt := make([]byte, total)
	binary.LittleEndian.PutUint16(pkt[0:], packetID)
	binary.LittleEndian.PutUint16(pkt[2:], uint16(total))
	off := 4
	if hasInvType {
		pkt[4] = invType
		off = 5
	}
	for _, item := range items {
		copy(pkt[off:], item)
		off += len(item)
	}
	return pkt
}

// ── NORMALITEM_INFO size tests ────────────────────────────────────────────────

func TestNormalItemSize(t *testing.T) {
	cases := []struct {
		pv   uint32
		want int
	}{
		{19000101, 18}, // pv < 20080102
		{20070101, 18},
		{20080102, 22}, // pv >= 20080102, < 20120925
		{20100101, 22},
		{20120925, 24}, // pv >= 20120925, < 20181121
		{20170101, 24},
		{20181121, 34}, // pv >= 20181121
		{20200401, 34},
	}
	for _, c := range cases {
		got := normalItemSize(c.pv)
		if got != c.want {
			t.Errorf("normalItemSize(%d) = %d, want %d", c.pv, got, c.want)
		}
	}
}

// ── NORMALITEM_INFO golden decode tests ───────────────────────────────────────

// pv=20060101: pv < 20080102, 18-byte entries, 0x00A3 decoder.
// Fields: IsIdentified is standalone; WearState is uint16; cards are uint16; no HireExpireDate.
func TestInventoryItemsStackable_0x00A3_Golden(t *testing.T) {
	pv := uint32(20060101)
	item := buildNormalItem(pv, 10, 501, 2, 3, 0x0000, [4]uint32{0x0101, 0x0202, 0, 0}, 0, 1, 0)
	pkt := buildNormalPacket(0x00A3, pv, false, 0, [][]byte{item})

	got := InventoryItemsStackable_0x00A3(pkt, pv)

	if got.PacketLength != int16(len(pkt)) {
		t.Fatalf("PacketLength: got %d, want %d", got.PacketLength, len(pkt))
	}
	if len(got.Items) != 1 {
		t.Fatalf("Items len: got %d, want 1", len(got.Items))
	}
	e := got.Items[0]
	if e.Index != 10 {
		t.Errorf("Index: got %d, want 10", e.Index)
	}
	if e.ITID != 501 {
		t.Errorf("ITID: got %d, want 501", e.ITID)
	}
	if e.Type != 2 {
		t.Errorf("Type: got %d, want 2", e.Type)
	}
	if e.Count != 3 {
		t.Errorf("Count: got %d, want 3", e.Count)
	}
	if e.Cards[0] != 0x0101 {
		t.Errorf("Cards[0]: got %#x, want 0x0101", e.Cards[0])
	}
	if e.Cards[1] != 0x0202 {
		t.Errorf("Cards[1]: got %#x, want 0x0202", e.Cards[1])
	}
	if e.IsIdentified != 1 {
		t.Errorf("IsIdentified: got %d, want 1", e.IsIdentified)
	}
	if e.HireExpireDate != 0 {
		t.Errorf("HireExpireDate: got %d, want 0 (absent at this pv)", e.HireExpireDate)
	}
}

// pv=20100101: pv >= 20080102, < 20120925, 22-byte entries, 0x02E8 decoder.
func TestInventoryItemsStackable_0x02E8_Golden(t *testing.T) {
	pv := uint32(20100101)
	item := buildNormalItem(pv, 5, 999, 4, 10, 0x0000, [4]uint32{0xAAAA, 0, 0, 0}, -1, 0, 0)
	pkt := buildNormalPacket(0x02E8, pv, false, 0, [][]byte{item})

	got := InventoryItemsStackable_0x02E8(pkt, pv)

	if len(got.Items) != 1 {
		t.Fatalf("Items len: got %d, want 1", len(got.Items))
	}
	e := got.Items[0]
	if e.ITID != 999 {
		t.Errorf("ITID: got %d, want 999", e.ITID)
	}
	if e.HireExpireDate != -1 {
		t.Errorf("HireExpireDate: got %d, want -1", e.HireExpireDate)
	}
	if e.Cards[0] != 0xAAAA {
		t.Errorf("Cards[0]: got %#x, want 0xAAAA", e.Cards[0])
	}
}

// pv=20170101: pv >= 20120925, < 20181121, 24-byte entries, 0x0991 decoder.
// IsIdentified and PlaceETCTab come from Flag bitfield.
func TestInventoryItemsStackable_0x0991_Golden(t *testing.T) {
	pv := uint32(20170101)
	item := buildNormalItem(pv, 7, 12345, 3, 1, 0xDEAD, [4]uint32{0x1111, 0x2222, 0x3333, 0x4444},
		99, 1 /*IsIdentified*/, 1 /*PlaceETCTab*/)
	pkt := buildNormalPacket(0x0991, pv, false, 0, [][]byte{item})

	got := InventoryItemsStackable_0x0991(pkt, pv)

	if len(got.Items) != 1 {
		t.Fatalf("Items len: got %d, want 1", len(got.Items))
	}
	e := got.Items[0]
	if e.ITID != 12345 {
		t.Errorf("ITID: got %d, want 12345", e.ITID)
	}
	if e.WearState != 0xDEAD {
		t.Errorf("WearState: got %#x, want 0xDEAD", e.WearState)
	}
	if e.Cards[3] != 0x4444 {
		t.Errorf("Cards[3]: got %#x, want 0x4444", e.Cards[3])
	}
	if e.IsIdentified != 1 {
		t.Errorf("IsIdentified: got %d, want 1", e.IsIdentified)
	}
	if e.PlaceETCTab != 1 {
		t.Errorf("PlaceETCTab: got %d, want 1", e.PlaceETCTab)
	}
}

// pv=20200401: pv >= 20181121, 34-byte entries, 0x0B09 decoder with invType.
func TestInventoryItemsStackable_0x0B09_Golden(t *testing.T) {
	pv := uint32(20200401)
	item := buildNormalItem(pv, 3, 0x0001869F, 6, 50, 0x0000FFFF,
		[4]uint32{0x11111111, 0x22222222, 0x33333333, 0x44444444},
		12345678, 1, 0)
	pkt := buildNormalPacket(0x0B09, pv, true, 0x01, [][]byte{item})

	got := InventoryItemsStackable_0x0B09(pkt, pv)

	if got.InvType != 0x01 {
		t.Errorf("InvType: got %d, want 1", got.InvType)
	}
	if len(got.Items) != 1 {
		t.Fatalf("Items len: got %d, want 1", len(got.Items))
	}
	e := got.Items[0]
	if e.ITID != 0x0001869F {
		t.Errorf("ITID: got %#x, want 0x0001869F", e.ITID)
	}
	if e.Cards[0] != 0x11111111 {
		t.Errorf("Cards[0]: got %#x, want 0x11111111", e.Cards[0])
	}
	if e.Cards[3] != 0x44444444 {
		t.Errorf("Cards[3]: got %#x, want 0x44444444", e.Cards[3])
	}
	if e.HireExpireDate != 12345678 {
		t.Errorf("HireExpireDate: got %d, want 12345678", e.HireExpireDate)
	}
}

// Multiple items in one packet.
func TestInventoryItemsStackable_MultipleItems(t *testing.T) {
	pv := uint32(20170101)
	item1 := buildNormalItem(pv, 1, 100, 1, 5, 0, [4]uint32{}, 0, 0, 0)
	item2 := buildNormalItem(pv, 2, 200, 2, 10, 0, [4]uint32{}, 0, 1, 0)
	item3 := buildNormalItem(pv, 3, 300, 3, 1, 0, [4]uint32{}, 0, 0, 1)
	pkt := buildNormalPacket(0x0991, pv, false, 0, [][]byte{item1, item2, item3})

	got := InventoryItemsStackable_0x0991(pkt, pv)

	if len(got.Items) != 3 {
		t.Fatalf("Items len: got %d, want 3", len(got.Items))
	}
	if got.Items[0].ITID != 100 || got.Items[1].ITID != 200 || got.Items[2].ITID != 300 {
		t.Errorf("item ITIDs: got %d, %d, %d; want 100, 200, 300",
			got.Items[0].ITID, got.Items[1].ITID, got.Items[2].ITID)
	}
}

// Empty body → zero items, no panic.
func TestInventoryItemsStackable_EmptyBody(t *testing.T) {
	pv := uint32(20170101)
	pkt := buildNormalPacket(0x0991, pv, false, 0, nil)
	got := InventoryItemsStackable_0x0991(pkt, pv)
	if len(got.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(got.Items))
	}
}

// Remainder bytes that do not fill a complete entry are silently truncated.
func TestInventoryItemsStackable_PartialEntry(t *testing.T) {
	pv := uint32(20170101)
	item := buildNormalItem(pv, 1, 999, 1, 1, 0, [4]uint32{}, 0, 1, 0)
	// Append 3 partial bytes that don't complete a second entry.
	partial := append(item, 0xAA, 0xBB, 0xCC)
	pkt := buildNormalPacket(0x0991, pv, false, 0, [][]byte{partial})
	// Manually fix PacketLength to reflect the extra bytes.
	binary.LittleEndian.PutUint16(pkt[2:], uint16(len(pkt)))

	got := InventoryItemsStackable_0x0991(pkt, pv)
	if len(got.Items) != 1 {
		t.Errorf("expected 1 item (partial entry dropped), got %d", len(got.Items))
	}
}

// ── EQUIPITEM_INFO helpers ────────────────────────────────────────────────────

// buildEquipItem builds one EQUIPITEM_INFO wire entry at the given packetver.
func buildEquipItem(pv uint32, index int16, itid uint32, typ uint8, loc uint32, wearState uint32,
	refLevel uint8, cards [4]uint32, hireExpire int32, bindType uint16, sprite uint16,
	optCount uint8, opts [5]events.ItemOption, grade uint8,
	isIdent uint8, isDamaged uint8, placeETC uint8) []byte {

	sz := equipItemSize(pv)
	b := make([]byte, sz)
	off := 0

	binary.LittleEndian.PutUint16(b[off:], uint16(index))
	off += 2

	if pv >= 20181121 {
		binary.LittleEndian.PutUint32(b[off:], itid)
		off += 4
	} else {
		binary.LittleEndian.PutUint16(b[off:], uint16(itid))
		off += 2
	}

	b[off] = typ
	off++

	if pv < 20120925 {
		b[off] = isIdent
		off++
	}

	if pv >= 20120925 {
		binary.LittleEndian.PutUint32(b[off:], loc)
		off += 4
		binary.LittleEndian.PutUint32(b[off:], wearState)
		off += 4
	} else {
		binary.LittleEndian.PutUint16(b[off:], uint16(loc))
		off += 2
		binary.LittleEndian.PutUint16(b[off:], uint16(wearState))
		off += 2
	}

	if pv < 20120925 {
		b[off] = isDamaged
		off++
	}

	if pv < 20200916 {
		b[off] = refLevel
		off++
	}

	if pv >= 20181121 {
		for i := 0; i < 4; i++ {
			binary.LittleEndian.PutUint32(b[off:], cards[i])
			off += 4
		}
	} else {
		for i := 0; i < 4; i++ {
			binary.LittleEndian.PutUint16(b[off:], uint16(cards[i]))
			off += 2
		}
	}

	if pv >= 20071002 {
		binary.LittleEndian.PutUint32(b[off:], uint32(hireExpire))
		off += 4
	}

	if pv >= 20080102 {
		binary.LittleEndian.PutUint16(b[off:], bindType)
		off += 2
	}

	if pv >= 20100629 {
		binary.LittleEndian.PutUint16(b[off:], sprite)
		off += 2
	}

	if pv >= 20150226 {
		b[off] = optCount
		off++
		for i := 0; i < 5; i++ {
			binary.LittleEndian.PutUint16(b[off:], uint16(opts[i].Index))
			binary.LittleEndian.PutUint16(b[off+2:], uint16(opts[i].Value))
			b[off+4] = opts[i].Param
			off += 5
		}
	}

	if pv >= 20200916 {
		b[off] = refLevel
		off++
		b[off] = grade
		off++
	}

	if pv >= 20120925 {
		flag := isIdent&0x01 | (isDamaged&0x01)<<1 | (placeETC&0x01)<<2
		b[off] = flag
	}

	return b
}

// buildEquipPacket builds a complete packet_itemlist_equip wire packet.
func buildEquipPacket(packetID uint16, pv uint32, hasInvType bool, invType uint8,
	items [][]byte) []byte {
	return buildNormalPacket(packetID, pv, hasInvType, invType, items)
}

// ── EQUIPITEM_INFO size tests ─────────────────────────────────────────────────

func TestEquipItemSize(t *testing.T) {
	cases := []struct {
		pv   uint32
		want int
	}{
		{19000101, 20}, // pv < 20071002
		{20071002, 24}, // pv >= 20071002, < 20080102
		{20080102, 26}, // pv >= 20080102, < 20100629
		{20100629, 28}, // pv >= 20100629, < 20120925
		{20120925, 31}, // pv >= 20120925, < 20150226
		{20150226, 57}, // pv >= 20150226, < 20181121
		{20181121, 67}, // pv >= 20181121, < 20200916
		{20200916, 68}, // pv >= 20200916
	}
	for _, c := range cases {
		got := equipItemSize(c.pv)
		if got != c.want {
			t.Errorf("equipItemSize(%d) = %d, want %d", c.pv, got, c.want)
		}
	}
}

// ── EQUIPITEM_INFO golden decode tests ────────────────────────────────────────

// pv < 20071002: 20 bytes, standalone IsIdentified/IsDamaged, uint16 loc/WearState.
func TestInventoryItemsEquip_0x00A4_Golden(t *testing.T) {
	pv := uint32(20060101)
	item := buildEquipItem(pv, 1, 2101, 5, 0x0004, 0x0004, 7,
		[4]uint32{0x0A0A, 0, 0, 0}, 0, 0, 0, 0, [5]events.ItemOption{}, 0, 1, 0, 0)
	pkt := buildEquipPacket(0x00A4, pv, false, 0, [][]byte{item})

	got := InventoryItemsEquip_0x00A4(pkt, pv)

	if len(got.Items) != 1 {
		t.Fatalf("Items len: got %d, want 1", len(got.Items))
	}
	e := got.Items[0]
	if e.ITID != 2101 {
		t.Errorf("ITID: got %d, want 2101", e.ITID)
	}
	if e.Location != 0x0004 {
		t.Errorf("Location: got %#x, want 0x0004", e.Location)
	}
	if e.RefiningLevel != 7 {
		t.Errorf("RefiningLevel: got %d, want 7", e.RefiningLevel)
	}
	if e.Cards[0] != 0x0A0A {
		t.Errorf("Cards[0]: got %#x, want 0x0A0A", e.Cards[0])
	}
	if e.IsIdentified != 1 {
		t.Errorf("IsIdentified: got %d, want 1", e.IsIdentified)
	}
}

// pv=20071002: 24 bytes, adds HireExpireDate.
func TestInventoryItemsEquip_0x0295_Golden(t *testing.T) {
	pv := uint32(20071002)
	item := buildEquipItem(pv, 2, 1301, 4, 0x0010, 0x0010, 5,
		[4]uint32{}, 777, 0, 0, 0, [5]events.ItemOption{}, 0, 1, 1, 0)
	pkt := buildEquipPacket(0x0295, pv, false, 0, [][]byte{item})

	got := InventoryItemsEquip_0x0295(pkt, pv)

	if len(got.Items) != 1 {
		t.Fatalf("Items len: got %d, want 1", len(got.Items))
	}
	e := got.Items[0]
	if e.HireExpireDate != 777 {
		t.Errorf("HireExpireDate: got %d, want 777", e.HireExpireDate)
	}
}

// pv=20120925: 31 bytes, Flag bitfield, uint32 loc/WearState, bindOnEquipType, sprite.
func TestInventoryItemsEquip_0x0992_Golden(t *testing.T) {
	pv := uint32(20130101)
	item := buildEquipItem(pv, 3, 5101, 4, 0x00010000, 0x00010000, 9,
		[4]uint32{0xBEEF, 0, 0, 0}, 888, 0x0001, 0x0002, 0, [5]events.ItemOption{}, 0,
		1 /*isIdent*/, 1 /*isDamaged*/, 1 /*placeETC*/)
	pkt := buildEquipPacket(0x0992, pv, false, 0, [][]byte{item})

	got := InventoryItemsEquip_0x0992(pkt, pv)

	if len(got.Items) != 1 {
		t.Fatalf("Items len: got %d, want 1", len(got.Items))
	}
	e := got.Items[0]
	if e.Location != 0x00010000 {
		t.Errorf("Location: got %#x, want 0x00010000", e.Location)
	}
	if e.IsIdentified != 1 {
		t.Errorf("IsIdentified: got %d, want 1", e.IsIdentified)
	}
	if e.IsDamaged != 1 {
		t.Errorf("IsDamaged: got %d, want 1", e.IsDamaged)
	}
	if e.PlaceETCTab != 1 {
		t.Errorf("PlaceETCTab: got %d, want 1", e.PlaceETCTab)
	}
	if e.BindOnEquipType != 0x0001 {
		t.Errorf("BindOnEquipType: got %#x, want 0x0001", e.BindOnEquipType)
	}
	if e.SpriteNumber != 0x0002 {
		t.Errorf("SpriteNumber: got %#x, want 0x0002", e.SpriteNumber)
	}
}

// pv=20170101: 57 bytes, adds option_count + option_data[5].
func TestInventoryItemsEquip_0x0A0D_Golden(t *testing.T) {
	pv := uint32(20170101)
	opts := [5]events.ItemOption{
		{Index: 1, Value: 10, Param: 2},
		{Index: 2, Value: 20, Param: 3},
	}
	item := buildEquipItem(pv, 4, 7777, 4, 0x0200, 0x0200, 10,
		[4]uint32{0x1234, 0, 0, 0}, 0, 0, 0, 2, opts, 0, 1, 0, 0)
	pkt := buildEquipPacket(0x0A0D, pv, false, 0, [][]byte{item})

	got := InventoryItemsEquip_0x0A0D(pkt, pv)

	if len(got.Items) != 1 {
		t.Fatalf("Items len: got %d, want 1", len(got.Items))
	}
	e := got.Items[0]
	if e.OptionCount != 2 {
		t.Errorf("OptionCount: got %d, want 2", e.OptionCount)
	}
	if e.Options[0].Index != 1 || e.Options[0].Value != 10 || e.Options[0].Param != 2 {
		t.Errorf("Options[0]: got %+v, want {1 10 2}", e.Options[0])
	}
	if e.Options[1].Value != 20 {
		t.Errorf("Options[1].Value: got %d, want 20", e.Options[1].Value)
	}
}

// pv=20200401: 67 bytes (>= 20181121, < 20200916), invType present, 0x0B0A decoder.
func TestInventoryItemsEquip_0x0B0A_Golden(t *testing.T) {
	pv := uint32(20200401)
	item := buildEquipItem(pv, 5, 0x00ABCDEF, 4, 0x0040, 0x0040, 12,
		[4]uint32{0x11223344, 0, 0, 0}, 0, 0, 0, 0, [5]events.ItemOption{}, 0, 1, 0, 0)
	pkt := buildEquipPacket(0x0B0A, pv, true, 0x02, [][]byte{item})

	got := InventoryItemsEquip_0x0B0A(pkt, pv)

	if got.InvType != 0x02 {
		t.Errorf("InvType: got %d, want 2", got.InvType)
	}
	if len(got.Items) != 1 {
		t.Fatalf("Items len: got %d, want 1", len(got.Items))
	}
	e := got.Items[0]
	if e.ITID != 0x00ABCDEF {
		t.Errorf("ITID: got %#x, want 0x00ABCDEF", e.ITID)
	}
	if e.Cards[0] != 0x11223344 {
		t.Errorf("Cards[0]: got %#x, want 0x11223344", e.Cards[0])
	}
	if e.RefiningLevel != 12 {
		t.Errorf("RefiningLevel: got %d, want 12", e.RefiningLevel)
	}
}

// pv=20200916: 68 bytes, RefiningLevel repositioned, grade added, 0x0B39 decoder.
func TestInventoryItemsEquip_0x0B39_Golden(t *testing.T) {
	pv := uint32(20210101)
	item := buildEquipItem(pv, 6, 0x00001234, 4, 0x0080, 0x0080, 15,
		[4]uint32{0xDEADBEEF, 0, 0, 0}, 0, 0, 0, 0, [5]events.ItemOption{}, 3,
		1 /*isIdent*/, 0, 0)
	pkt := buildEquipPacket(0x0B39, pv, true, 0x03, [][]byte{item})

	got := InventoryItemsEquip_0x0B39(pkt, pv)

	if got.InvType != 0x03 {
		t.Errorf("InvType: got %d, want 3", got.InvType)
	}
	if len(got.Items) != 1 {
		t.Fatalf("Items len: got %d, want 1", len(got.Items))
	}
	e := got.Items[0]
	if e.RefiningLevel != 15 {
		t.Errorf("RefiningLevel: got %d, want 15", e.RefiningLevel)
	}
	if e.Grade != 3 {
		t.Errorf("Grade: got %d, want 3", e.Grade)
	}
	if e.Cards[0] != 0xDEADBEEF {
		t.Errorf("Cards[0]: got %#x, want 0xDEADBEEF", e.Cards[0])
	}
}

// Empty body for equip → zero items, no panic.
func TestInventoryItemsEquip_EmptyBody(t *testing.T) {
	pv := uint32(20170101)
	pkt := buildEquipPacket(0x0A0D, pv, false, 0, nil)
	got := InventoryItemsEquip_0x0A0D(pkt, pv)
	if len(got.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(got.Items))
	}
}

// ── Benchmarks ────────────────────────────────────────────────────────────────

// BenchmarkDecodeNormalItems_1Entry: 1 alloc per call expected (make slice).
func BenchmarkDecodeNormalItems_1Entry(b *testing.B) {
	pv := uint32(20170101)
	item := buildNormalItem(pv, 1, 1001, 2, 5, 0, [4]uint32{}, 0, 1, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = decodeNormalItems(item, pv)
	}
}

// BenchmarkDecodeNormalItems_10Entries: 1 alloc per call expected.
func BenchmarkDecodeNormalItems_10Entries(b *testing.B) {
	pv := uint32(20200401)
	body := make([]byte, normalItemSize(pv)*10)
	for i := 0; i < 10; i++ {
		entry := buildNormalItem(pv, int16(i), uint32(1000+i), 2, 1, 0,
			[4]uint32{}, 0, 1, 0)
		copy(body[i*normalItemSize(pv):], entry)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = decodeNormalItems(body, pv)
	}
}

// BenchmarkDecodeEquipItems_1Entry: 1 alloc per call expected.
func BenchmarkDecodeEquipItems_1Entry(b *testing.B) {
	pv := uint32(20200401)
	item := buildEquipItem(pv, 1, 5000, 4, 0x0040, 0x0040, 10,
		[4]uint32{}, 0, 0, 0, 0, [5]events.ItemOption{}, 0, 1, 0, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = decodeEquipItems(item, pv)
	}
}
