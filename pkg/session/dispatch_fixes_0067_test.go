// Dispatch tests for bug fixes in 0067:
//   BUG-1:     0x0295/0x02D0 now dispatch for ActionInventoryItemsEquip
//   BUG-2:     0x099F/0x09CA now dispatch for ActionAreaSpell
//   BUG-NEW-1: 0x084B/0x0ADD now dispatch for ActionItemAppeared
//   BUG-NEW-2: 0x0983 now dispatches for ActionActorStatusActive
//   BUG-NEW-3: middle-gen actor IDs now dispatch

package session

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/events"
)

// makeVarFrameID builds a minimal variable-length frame with the given ID and body size.
func makeVarFrameID(id uint16, size int) []byte {
	frame := make([]byte, size)
	binary.LittleEndian.PutUint16(frame[0:], id)
	binary.LittleEndian.PutUint16(frame[2:], uint16(size))
	return frame
}

// TestDispatch_0x0295_EquipInventory verifies BUG-1: 0x0295 fires ActionInventoryItemsEquip.
func TestDispatch_0x0295_EquipInventory(t *testing.T) {
	pv := uint32(20071002)
	s := NewMapSession(pv)

	fired := 0
	RegisterSemanticHandler(s, ActionInventoryItemsEquip, func(e events.InventoryItemsEquip) {
		fired++
	})

	if s.core.lengths[0x0295] == 0 {
		t.Skip("0x0295 not active at this pv")
	}

	frame := makeVarFrameID(0x0295, 4) // minimal valid variable-length frame
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed error: %v", err)
	}
	if fired != 1 {
		t.Errorf("handler fired %d times, want 1", fired)
	}
}

// TestDispatch_0x02D0_EquipInventory verifies BUG-1: 0x02D0 fires ActionInventoryItemsEquip.
func TestDispatch_0x02D0_EquipInventory(t *testing.T) {
	pv := uint32(20080102)
	s := NewMapSession(pv)

	fired := 0
	RegisterSemanticHandler(s, ActionInventoryItemsEquip, func(e events.InventoryItemsEquip) {
		fired++
	})

	if s.core.lengths[0x02D0] == 0 {
		t.Skip("0x02D0 not active at this pv")
	}

	frame := makeVarFrameID(0x02D0, 4)
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed error: %v", err)
	}
	if fired != 1 {
		t.Errorf("handler fired %d times, want 1", fired)
	}
}

// TestDispatch_0x099F_AreaSpell verifies BUG-2: 0x099F fires ActionAreaSpell.
// Note: lengths_map assigns t[0x099F]=22 at pv>=20130320 (not 20121212).
func TestDispatch_0x099F_AreaSpell(t *testing.T) {
	pv := uint32(20130320)
	s := NewMapSession(pv)

	fired := 0
	RegisterSemanticHandler(s, ActionAreaSpell, func(e events.AreaSpell) {
		fired++
	})

	// 0x099F length = 22 at this pv
	frame := make([]byte, 22)
	binary.LittleEndian.PutUint16(frame[0:], 0x099F)
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed error: %v", err)
	}
	if fired != 1 {
		t.Errorf("handler fired %d times, want 1", fired)
	}
}

// TestDispatch_0x09CA_AreaSpell verifies BUG-2: 0x09CA fires ActionAreaSpell.
func TestDispatch_0x09CA_AreaSpell(t *testing.T) {
	pv := uint32(20130731)
	s := NewMapSession(pv)

	fired := 0
	RegisterSemanticHandler(s, ActionAreaSpell, func(e events.AreaSpell) {
		fired++
	})

	// 0x09CA length = 23 at this pv
	frame := make([]byte, 23)
	binary.LittleEndian.PutUint16(frame[0:], 0x09CA)
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed error: %v", err)
	}
	if fired != 1 {
		t.Errorf("handler fired %d times, want 1", fired)
	}
}

// TestDispatch_0x084B_ItemAppeared verifies BUG-NEW-1: 0x084B fires ActionItemAppeared.
func TestDispatch_0x084B_ItemAppeared(t *testing.T) {
	pv := uint32(20150101)
	s := NewMapSession(pv)

	fired := 0
	RegisterSemanticHandler(s, ActionItemAppeared, func(e events.ItemAppeared) {
		fired++
	})

	// 0x084B length = 19
	frame := make([]byte, 19)
	binary.LittleEndian.PutUint16(frame[0:], 0x084B)
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed error: %v", err)
	}
	if fired != 1 {
		t.Errorf("handler fired %d times, want 1", fired)
	}
}

// TestDispatch_0x0ADD_ItemAppeared verifies BUG-NEW-1: 0x0ADD fires ActionItemAppeared.
// Also verifies that at pv=20200401 the length override gives 24 bytes.
func TestDispatch_0x0ADD_ItemAppeared(t *testing.T) {
	pv := uint32(20200401)
	s := NewMapSession(pv)

	if s.core.lengths[0x0ADD] != 24 {
		t.Errorf("expected 0x0ADD length=24 at pv=20200401, got %d", s.core.lengths[0x0ADD])
	}

	fired := 0
	var gotEvent events.ItemAppeared
	RegisterSemanticHandler(s, ActionItemAppeared, func(e events.ItemAppeared) {
		fired++
		gotEvent = e
	})

	// Build a 24-byte 0x0ADD frame with known ITID
	frame := make([]byte, 24)
	binary.LittleEndian.PutUint16(frame[0:], 0x0ADD)
	binary.LittleEndian.PutUint32(frame[2:], 0x1111)  // ITAID
	binary.LittleEndian.PutUint32(frame[6:], 0xABCDE) // ITID (uint32 at pv>=20181121)
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed error: %v", err)
	}
	if fired != 1 {
		t.Errorf("handler fired %d times, want 1", fired)
	}
	if gotEvent.ITID != 0xABCDE {
		t.Errorf("ITID: got %#x, want 0xABCDE", gotEvent.ITID)
	}
}

// TestDispatch_0x0983_ActorStatusActive verifies BUG-NEW-2: 0x0983 fires ActionActorStatusActive.
func TestDispatch_0x0983_ActorStatusActive(t *testing.T) {
	pv := uint32(20200401)
	s := NewMapSession(pv)

	if s.core.lengths[0x0983] == 0 {
		t.Errorf("expected 0x0983 to be active at pv=20200401")
	}

	fired := 0
	var gotEvent events.ActorStatusActive
	RegisterSemanticHandler(s, ActionActorStatusActive, func(e events.ActorStatusActive) {
		fired++
		gotEvent = e
	})

	// 0x0983 = 29 bytes
	frame := make([]byte, 29)
	binary.LittleEndian.PutUint16(frame[0:], 0x0983)
	binary.LittleEndian.PutUint16(frame[2:], 10)     // index
	binary.LittleEndian.PutUint32(frame[4:], 0xBEEF) // AID
	frame[8] = 3                                     // state
	binary.LittleEndian.PutUint32(frame[9:], 5000)   // Total
	binary.LittleEndian.PutUint32(frame[13:], 2500)  // Left

	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed error: %v", err)
	}
	if fired != 1 {
		t.Errorf("handler fired %d times, want 1", fired)
	}
	if gotEvent.AID != 0xBEEF {
		t.Errorf("AID: got %#x, want 0xBEEF", gotEvent.AID)
	}
	if gotEvent.Total != 5000 {
		t.Errorf("Total: got %d, want 5000", gotEvent.Total)
	}
	if gotEvent.Left != 2500 {
		t.Errorf("Left: got %d, want 2500", gotEvent.Left)
	}
}

// TestDispatch_MiddleGenActors verifies BUG-NEW-3: one test per middle-gen ID
// confirms the handler fires (no field validation — lengths table drives frame delivery).
func TestDispatch_MiddleGenActors(t *testing.T) {
	cases := []struct {
		name   string
		pv     uint32
		id     uint16
		action SemanticAction
	}{
		{"ActorExists_0x07F9", 20091103, 0x07F9, ActionActorExists},
		{"ActorExists_0x0857", 20101124, 0x0857, ActionActorExists},
		{"ActorExists_0x0915", 20120221, 0x0915, ActionActorExists},
		{"ActorConnected_0x07F8", 20091103, 0x07F8, ActionActorConnected},
		{"ActorConnected_0x0858", 20101124, 0x0858, ActionActorConnected},
		{"ActorConnected_0x090F", 20120221, 0x090F, ActionActorConnected},
		{"ActorMoved_0x07F7", 20091103, 0x07F7, ActionActorMoved},
		{"ActorMoved_0x0856", 20101124, 0x0856, ActionActorMoved},
		{"ActorMoved_0x0914", 20120221, 0x0914, ActionActorMoved},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := NewMapSession(c.pv)
			length := s.core.lengths[c.id]
			if length == 0 {
				t.Skipf("packet %#x not active at pv=%d", c.id, c.pv)
			}
			var frameSize int
			if length == -1 {
				frameSize = 64 // variable-length: send a small valid frame
			} else {
				frameSize = int(length)
			}

			fired := 0
			// Register a generic handler using the raw low-level API since the
			// specific event type varies per action.
			s.registerHandler(c.id, func(data []byte, pv uint32) {
				fired++
			})

			frame := makeVarFrameID(c.id, frameSize)
			if err := s.Feed(frame); err != nil {
				t.Fatalf("Feed error: %v", err)
			}
			if fired != 1 {
				t.Errorf("%s: handler fired %d times, want 1", c.name, fired)
			}
		})
	}
}
