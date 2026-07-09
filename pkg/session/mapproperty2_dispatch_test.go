package session

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/events"
)

// This file is the regression test for the missing 0x099B dispatch at
// PACKETVER >= 20121010 (issue #9), plus coverage of the full map-property
// packet family across all relevant packetvers.
//
// rAthena clif_map_property (clif.cpp:6871-6903) sends the map-property packet as:
//   - 0x0199 (4 bytes) for PACKETVER <  20121010
//   - 0x099B (8 bytes) for PACKETVER >= 20121010
// Before this fix, receiveDispatch[ActionZcNotifyMapproperty2] listed only 0x01D6,
// so 0x099B was framed (the length table already knew 0x099B=8) but never decoded
// and never fired the semantic handler on any modern packetver.

func build099BMappropertyFrame(property uint16, flags uint32) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint16(b[0:], 0x099B)
	binary.LittleEndian.PutUint16(b[2:], property)
	binary.LittleEndian.PutUint32(b[4:], flags)
	return b
}

func build0199MappropertyFrame(property uint16) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint16(b[0:], 0x0199)
	binary.LittleEndian.PutUint16(b[2:], property)
	return b
}

func build01D6MaptypeFrame(mapType uint16) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint16(b[0:], 0x01D6)
	binary.LittleEndian.PutUint16(b[2:], mapType)
	return b
}

func TestMapproperty2_Dispatch_HasAllVariants(t *testing.T) {
	entries, ok := receiveDispatch[ActionZcNotifyMapproperty2]
	if !ok || len(entries) == 0 {
		t.Fatalf("ActionZcNotifyMapproperty2 has no entries in receiveDispatch")
	}

	want := map[uint16]bool{0x01D6: true, 0x0199: true, 0x099B: true}
	seen := map[uint16]bool{}
	for _, e := range entries {
		seen[e.id] = true
	}
	for pid := range want {
		if !seen[pid] {
			t.Errorf("0x%04X missing from ActionZcNotifyMapproperty2 dispatch (have %v)", pid, seen)
		}
	}
	if len(entries) != 3 {
		t.Errorf("ActionZcNotifyMapproperty2 dispatch entry count: got %d want 3", len(entries))
	}
}

func TestMapproperty2_0x099B_FiresAt_20200401(t *testing.T) {
	pv := uint32(20200401)
	s := NewMapSession(pv)

	if got := s.core.lengths[0x099B]; got != 8 {
		t.Fatalf("prerequisite: lengths[0x099B] = %d at pv=%d, want 8", got, pv)
	}

	var fired int
	var gotEvent events.ZcNotifyMapproperty2
	RegisterSemanticHandler(s, ActionZcNotifyMapproperty2, func(e events.ZcNotifyMapproperty2) {
		fired++
		gotEvent = e
	})

	frame := build099BMappropertyFrame(uint16(events.MapPropertyAgitZone),
		events.MapPropertyFlagParty|events.MapPropertyFlagGuild|events.MapPropertyFlagSiege)
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed(0x099B) error: %v", err)
	}

	if fired != 1 {
		t.Fatalf("ActionZcNotifyMapproperty2 fired %d times, want 1", fired)
	}
	if unhandled := s.UnhandledPackets(); unhandled != 0 {
		t.Errorf("UnhandledPackets = %d, want 0", unhandled)
	}
	if gotEvent.Type != int16(events.MapPropertyAgitZone) {
		t.Errorf("Type: got %d want %d", gotEvent.Type, events.MapPropertyAgitZone)
	}
	wantFlags := events.MapPropertyFlagParty | events.MapPropertyFlagGuild | events.MapPropertyFlagSiege
	if gotEvent.Flags != wantFlags {
		t.Errorf("Flags: got %#x want %#x", gotEvent.Flags, wantFlags)
	}
}

func TestMapproperty2_0x099B_FiresAt_Boundary_20121010(t *testing.T) {
	pv := uint32(20121010)
	s := NewMapSession(pv)

	if got := s.core.lengths[0x099B]; got != 8 {
		t.Fatalf("prerequisite: lengths[0x099B] = %d at pv=%d, want 8 (rAthena sends 0x099B from 20121010)", got, pv)
	}

	fired := 0
	RegisterSemanticHandler(s, ActionZcNotifyMapproperty2, func(e events.ZcNotifyMapproperty2) {
		fired++
	})

	frame := build099BMappropertyFrame(uint16(events.MapPropertyFreePvpZone), 0)
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed(0x099B) error: %v", err)
	}
	if fired != 1 {
		t.Fatalf("ActionZcNotifyMapproperty2 fired %d times at pv=20121010, want 1", fired)
	}
}

func TestMapproperty2_0x099B_FiresInGapWindow_20130319(t *testing.T) {
	pv := uint32(20130319)
	s := NewMapSession(pv)

	if got := s.core.lengths[0x099B]; got != 8 {
		t.Fatalf("prerequisite: lengths[0x099B] = %d at pv=%d, want 8 (lengths_map_overrides window)", got, pv)
	}

	fired := 0
	RegisterSemanticHandler(s, ActionZcNotifyMapproperty2, func(e events.ZcNotifyMapproperty2) {
		fired++
	})

	frame := build099BMappropertyFrame(uint16(events.MapPropertyNothing), events.MapPropertyFlagUseCart)
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed(0x099B) error: %v", err)
	}
	if fired != 1 {
		t.Fatalf("ActionZcNotifyMapproperty2 fired %d times at pv=20130319, want 1", fired)
	}
}

func TestMapproperty2_0x0199_FiresAt_LegacyPacketver(t *testing.T) {
	pv := uint32(20100700)
	s := NewMapSession(pv)

	if got := s.core.lengths[0x0199]; got != 4 {
		t.Fatalf("prerequisite: lengths[0x0199] = %d at pv=%d, want 4", got, pv)
	}

	var fired int
	var gotEvent events.ZcNotifyMapproperty2
	RegisterSemanticHandler(s, ActionZcNotifyMapproperty2, func(e events.ZcNotifyMapproperty2) {
		fired++
		gotEvent = e
	})

	frame := build0199MappropertyFrame(uint16(events.MapPropertyFreePvpZone))
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed(0x0199) error: %v", err)
	}
	if fired != 1 {
		t.Fatalf("ActionZcNotifyMapproperty2 fired %d times, want 1", fired)
	}
	if gotEvent.Type != int16(events.MapPropertyFreePvpZone) {
		t.Errorf("Type: got %d want %d", gotEvent.Type, events.MapPropertyFreePvpZone)
	}
	if gotEvent.Flags != 0 {
		t.Errorf("Flags: got %#x want 0 (legacy 0x0199 has no flags field)", gotEvent.Flags)
	}
}

func TestMapproperty2_0x01D6_StillFires(t *testing.T) {
	pv := uint32(20200401)
	s := NewMapSession(pv)

	if got := s.core.lengths[0x01D6]; got != 4 {
		t.Fatalf("prerequisite: lengths[0x01D6] = %d at pv=%d, want 4", got, pv)
	}

	var fired int
	var gotEvent events.ZcNotifyMapproperty2
	RegisterSemanticHandler(s, ActionZcNotifyMapproperty2, func(e events.ZcNotifyMapproperty2) {
		fired++
		gotEvent = e
	})

	frame := build01D6MaptypeFrame(uint16(events.MapTypeBattlefield))
	if err := s.Feed(frame); err != nil {
		t.Fatalf("Feed(0x01D6) error: %v", err)
	}
	if fired != 1 {
		t.Fatalf("ActionZcNotifyMapproperty2 fired %d times for 0x01D6, want 1", fired)
	}
	if gotEvent.Type != int16(events.MapTypeBattlefield) {
		t.Errorf("Type: got %d want %d (MapTypeBattlefield)", gotEvent.Type, events.MapTypeBattlefield)
	}
	if gotEvent.Flags != 0 {
		t.Errorf("Flags: got %#x want 0 (0x01D6 clif_map_type has no flags)", gotEvent.Flags)
	}
}
