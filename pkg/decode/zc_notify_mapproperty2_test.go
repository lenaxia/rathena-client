package decode

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/events"
)

// This file tests the full rAthena map-property packet family, cataloged across
// every relevant PACKETVER. The family is two distinct rAthena send paths that
// the semantic DB groups under the single action zc_notify_mapproperty2:
//
//  1. clif_map_property (src/map/clif.cpp:6871-6903) — the "property + flags"
//     packet, sent at map entry (clif.cpp:10836-10844), PvP/PK zone changes, and
//     duel start/stop. Two wire variants keyed on PACKETVER:
//     - PACKETVER <  20121010: 0x0199, 4 bytes (cmd + property). ZC_NOTIFY_MAPPROPERTY.
//       property is enum map_property (clif.hpp:365-373, values 0-6).
//     - PACKETVER >= 20121010: 0x099B, 8 bytes (cmd + property + flags). ZC_MAPPROPERTY_R2.
//       property is enum map_property; flags is a uint32 bitfield (WBUFL(buf,4),
//       clif.cpp:6888-6898).
//
//  2. clif_map_type (src/map/clif.cpp:6907-6914) — a separate packet, 0x01D6,
//     4 bytes (cmd + type), sent for battlegrounds (clif.cpp:11071). type is
//     enum e_map_type (clif.hpp:376-402, values 0-29). Struct PACKET_ZC_NOTIFY_MAPPROPERTY2
//     (packets.hpp:966-969). No PACKETVER guard — sent at all packetvers.
//
// rAthena has NO C struct for 0x0199/0x099B; both are built with raw WBUFW/WBUFL
// macros into an unsigned char buffer (clif.cpp:6875/6878). Length registration:
// packet(0x0199,4) unconditionally (clif_packetdb.hpp:185); packet(0x099b,8) under
// #if PACKETVER >= 20130320 (clif_packetdb.hpp:1600,1642) — note the guard window
// starts at 20130320 but clif_map_property sends 0x099B from 20121010; the
// [20121010,20130320) framing gap is corrected in lengths_map_overrides.go.

// build0x099BFrame builds an 8-byte 0x099B frame (ZC_MAPPROPERTY_R2) from
// explicit field values, writing each field at its rAthena-verified offset.
func build0x099BFrame(property uint16, flags uint32) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint16(b[0:], 0x099B)
	binary.LittleEndian.PutUint16(b[2:], property)
	binary.LittleEndian.PutUint32(b[4:], flags)
	return b
}

// build0x0199Frame builds a 4-byte 0x0199 frame (ZC_NOTIFY_MAPPROPERTY, legacy)
// from explicit field values.
func build0x0199Frame(property uint16) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint16(b[0:], 0x0199)
	binary.LittleEndian.PutUint16(b[2:], property)
	return b
}

func TestZcNotifyMapproperty2_0x099B_PropertyAndFlags(t *testing.T) {
	data := build0x099BFrame(
		uint16(events.MapPropertyAgitZone),
		events.MapPropertyFlagParty|events.MapPropertyFlagGuild|events.MapPropertyFlagSiege,
	)

	e := ZcNotifyMapproperty2_0x099B(data, 20200401)

	if e.Type != int16(events.MapPropertyAgitZone) {
		t.Errorf("Type: got %d want %d (MapPropertyAgitZone)", e.Type, events.MapPropertyAgitZone)
	}
	wantFlags := events.MapPropertyFlagParty | events.MapPropertyFlagGuild | events.MapPropertyFlagSiege
	if e.Flags != wantFlags {
		t.Errorf("Flags: got %#x want %#x", e.Flags, wantFlags)
	}
}

func TestZcNotifyMapproperty2_0x099B_FullBitfield(t *testing.T) {
	full := events.MapPropertyFlagParty | events.MapPropertyFlagGuild | events.MapPropertyFlagSiege |
		events.MapPropertyFlagUseSimpleEffect | events.MapPropertyFlagDisableLockon |
		events.MapPropertyFlagCountPk | events.MapPropertyFlagNoPartyFormation |
		events.MapPropertyFlagBattlefield | events.MapPropertyFlagDisableCostume |
		events.MapPropertyFlagUseCart | events.MapPropertyFlagSunmoonstarMiracle
	data := build0x099BFrame(0, full)

	e := ZcNotifyMapproperty2_0x099B(data, 20200401)

	if e.Flags != full {
		t.Errorf("Flags: got %#x want %#x (bits 0-10 set)", e.Flags, full)
	}
	if e.Type != 0 {
		t.Errorf("Type: got %d want 0", e.Type)
	}
}

func TestZcNotifyMapproperty2_0x099B_ZeroValues(t *testing.T) {
	data := build0x099BFrame(0, 0)

	e := ZcNotifyMapproperty2_0x099B(data, 20200401)

	if e.Type != 0 {
		t.Errorf("Type: got %d want 0", e.Type)
	}
	if e.Flags != 0 {
		t.Errorf("Flags: got %#x want 0", e.Flags)
	}
}

func TestZcNotifyMapproperty2_0x099B_DecodesAcrossPacketvers(t *testing.T) {
	data := build0x099BFrame(uint16(events.MapPropertyFreePvpZone), 0xCAFEBABE)

	for _, pv := range []uint32{20121010, 20130320, 20180307, 20200401, 20210101} {
		e := ZcNotifyMapproperty2_0x099B(data, pv)
		if e.Type != int16(events.MapPropertyFreePvpZone) {
			t.Errorf("pv=%d: Type got %d want %d", pv, e.Type, events.MapPropertyFreePvpZone)
		}
		if e.Flags != 0xCAFEBABE {
			t.Errorf("pv=%d: Flags got %#x want 0xCAFEBABE", pv, e.Flags)
		}
	}
}

func TestZcNotifyMapproperty2_0x0199_PropertyOnly(t *testing.T) {
	data := build0x0199Frame(uint16(events.MapPropertyFreePvpZone))

	e := ZcNotifyMapproperty2_0x0199(data, 20100700)

	if e.Type != int16(events.MapPropertyFreePvpZone) {
		t.Errorf("Type: got %d want %d (MapPropertyFreePvpZone)", e.Type, events.MapPropertyFreePvpZone)
	}
	if e.Flags != 0 {
		t.Errorf("Flags: got %#x want 0 (legacy 0x0199 has no flags field)", e.Flags)
	}
}

func TestZcNotifyMapproperty2_0x0199_DecodesAcrossLegacyPacketvers(t *testing.T) {
	data := build0x0199Frame(uint16(events.MapPropertyDenySkillZone))

	for _, pv := range []uint32{20000000, 20080101, 20100700, 20121009} {
		e := ZcNotifyMapproperty2_0x0199(data, pv)
		if e.Type != int16(events.MapPropertyDenySkillZone) {
			t.Errorf("pv=%d: Type got %d want %d", pv, e.Type, events.MapPropertyDenySkillZone)
		}
		if e.Flags != 0 {
			t.Errorf("pv=%d: Flags got %#x want 0", pv, e.Flags)
		}
	}
}

func TestZcNotifyMapproperty2_0x01D6_FlagsStaysZero(t *testing.T) {
	data := make([]byte, 4)
	binary.LittleEndian.PutUint16(data[0:], 0x01D6)
	binary.LittleEndian.PutUint16(data[2:], uint16(events.MapTypeBattlefield))

	e := ZcNotifyMapproperty2_0x01D6(data, 20200401)

	if e.Type != int16(events.MapTypeBattlefield) {
		t.Errorf("Type: got %d want %d (MapTypeBattlefield)", e.Type, events.MapTypeBattlefield)
	}
	if e.Flags != 0 {
		t.Errorf("Flags: got %#x want 0 (0x01D6 clif_map_type has no flags field)", e.Flags)
	}
}

func BenchmarkZcNotifyMapproperty2_0x099B(b *testing.B) {
	data := build0x099BFrame(3, 0xDEADBEEF)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ZcNotifyMapproperty2_0x099B(data, 20200401)
	}
}

func BenchmarkZcNotifyMapproperty2_0x0199(b *testing.B) {
	data := build0x0199Frame(1)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ZcNotifyMapproperty2_0x0199(data, 20100700)
	}
}
