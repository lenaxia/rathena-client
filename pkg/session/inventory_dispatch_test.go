// Tests for ActionInventoryItemsStackable dispatch, specifically verifying that
// 0x0B09 fires at pv=20200401 where 0x0991 is disabled.

package session

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/events"
)

// buildNormalItemFrame builds a minimal 0x0B09 frame with one NORMALITEM_INFO
// entry at pv >= 20181121 (34-byte entry size).
//
// Packet layout (packets_struct.hpp:1187–1194):
//
//	[0:2]  PacketType (0x0B09)
//	[2:4]  PacketLength
//	[4]    invType
//	[5:]   NORMALITEM_INFO entries (34 bytes each at pv >= 20181121)
func build0x0B09Frame(invType uint8, itid uint32, pv uint32) []byte {
	// Entry layout at pv >= 20181121 (34 bytes):
	// index(2)+ITID(4)+type(1)+count(2)+WearState(4)+slot.card[4*4](16)+HireExpireDate(4)+Flag(1)
	entrySize := 34
	totalLen := 5 + entrySize
	frame := make([]byte, totalLen)

	binary.LittleEndian.PutUint16(frame[0:], 0x0B09)
	binary.LittleEndian.PutUint16(frame[2:], uint16(totalLen))
	frame[4] = invType

	off := 5
	binary.LittleEndian.PutUint16(frame[off:], 1) // index = 1
	off += 2
	binary.LittleEndian.PutUint32(frame[off:], itid) // ITID
	off += 4
	frame[off] = 3 // type
	off++
	binary.LittleEndian.PutUint16(frame[off:], 5) // count
	off += 2
	binary.LittleEndian.PutUint32(frame[off:], 0xDEAD) // WearState
	off += 4
	// slot.card[4] uint32 each
	binary.LittleEndian.PutUint32(frame[off:], 0x1111)
	binary.LittleEndian.PutUint32(frame[off+4:], 0x2222)
	binary.LittleEndian.PutUint32(frame[off+8:], 0x3333)
	binary.LittleEndian.PutUint32(frame[off+12:], 0x4444)
	off += 16
	binary.LittleEndian.PutUint32(frame[off:], uint32(99999)) // HireExpireDate
	off += 4
	frame[off] = 0x01 // Flag: IsIdentified=1

	return frame
}

// TestInventoryItemsStackable_0x0B09_Dispatch verifies that at pv=20200401:
//   - 0x0991 is disabled (length=0, so the session ignores it)
//   - 0x0B09 fires ActionInventoryItemsStackable with correct field values
func TestInventoryItemsStackable_0x0B09_Dispatch(t *testing.T) {
	pv := uint32(20200401)
	s := NewMapSession(pv)

	var fired int
	var gotEvent events.InventoryItemsStackable
	RegisterSemanticHandler(s, ActionInventoryItemsStackable, func(e events.InventoryItemsStackable) {
		fired++
		gotEvent = e
	})

	// Verify 0x0991 is not active at this pv: its length should be 0.
	// (lengths_map.go:2011 sets t[0x0991] = 0 at pv >= 20181002)
	if s.core.lengths[0x0991] != 0 {
		t.Errorf("expected 0x0991 length=0 at pv=%d, got %d", pv, s.core.lengths[0x0991])
	}
	// Verify 0x0B09 is active: its length should be -1 (variable).
	if s.core.lengths[0x0B09] != -1 {
		t.Errorf("expected 0x0B09 length=-1 at pv=%d, got %d", pv, s.core.lengths[0x0B09])
	}

	frame := build0x0B09Frame(0x02, 0xABCDE, pv)
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed returned error: %v", err)
	}

	if fired != 1 {
		t.Fatalf("handler fired %d times, want 1", fired)
	}
	if gotEvent.InvType != 0x02 {
		t.Errorf("InvType: got %d, want 2", gotEvent.InvType)
	}
	if len(gotEvent.Items) != 1 {
		t.Fatalf("Items len: got %d, want 1", len(gotEvent.Items))
	}
	item := gotEvent.Items[0]
	if item.ITID != 0xABCDE {
		t.Errorf("ITID: got %#x, want 0xABCDE", item.ITID)
	}
	if item.Index != 1 {
		t.Errorf("Index: got %d, want 1", item.Index)
	}
	if item.Count != 5 {
		t.Errorf("Count: got %d, want 5", item.Count)
	}
	if item.WearState != 0xDEAD {
		t.Errorf("WearState: got %#x, want 0xDEAD", item.WearState)
	}
	if item.Cards[0] != 0x1111 || item.Cards[1] != 0x2222 {
		t.Errorf("Cards: got %v, want [0x1111 0x2222 ...]", item.Cards)
	}
	if item.HireExpireDate != 99999 {
		t.Errorf("HireExpireDate: got %d, want 99999", item.HireExpireDate)
	}
	if item.IsIdentified != 1 {
		t.Errorf("IsIdentified: got %d, want 1", item.IsIdentified)
	}
}

// TestInventoryItemsStackable_0x0991_Disabled verifies that at pv=20200401
// a 0x0991 frame does NOT fire ActionInventoryItemsStackable (packet is ignored).
func TestInventoryItemsStackable_0x0991_Disabled(t *testing.T) {
	pv := uint32(20200401)
	s := NewMapSession(pv)

	fired := 0
	RegisterSemanticHandler(s, ActionInventoryItemsStackable, func(e events.InventoryItemsStackable) {
		fired++
	})

	// Build a minimal 0x0991 variable-length frame (lengths_map sets it to 0 at this pv).
	frame := makeVarFrame(0x0991, 28) // 4-byte header + one 24-byte entry
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed returned error: %v", err)
	}

	if fired != 0 {
		t.Errorf("handler fired %d times for disabled 0x0991, want 0", fired)
	}
}
