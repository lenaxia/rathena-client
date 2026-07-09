// Package decode — benchmarks and zero-value edge case for the 0x099B variant
// of ZC_MAPPROPERTY_R2 and the 0x01D6 variant of ZC_NOTIFY_MAPPROPERTY2.
//
// Rule 1 mandates "Benchmark tests verifying 0 allocs/op on all decode/encode
// functions." Both decoders below read scalar fields only (no strings, no
// slices), so each must be 0 allocs/op.
package decode

import (
	"encoding/binary"
	"testing"
)

// build0x099BFrame builds an 8-byte 0x099B frame matching rAthena's
// clif_map_property() wire layout (PACKETVER >= 20121010 in source,
// wire-effective at PACKETVER >= 20130320 per clif_packetdb.hpp:1600-1645).
func build0x099BFrame(typ uint16, flags uint32) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint16(b[0:], 0x099B)
	binary.LittleEndian.PutUint16(b[2:], typ)
	binary.LittleEndian.PutUint32(b[4:], flags)
	return b
}

// build0x01D6Frame builds a 4-byte 0x01D6 frame matching rAthena's
// clif_map_type() wire layout (all packetvers; 4-byte struct).
func build0x01D6Frame(typ uint16) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint16(b[0:], 0x01D6)
	binary.LittleEndian.PutUint16(b[2:], typ)
	return b
}

// TestZcNotifyMapproperty2_0x099B_ZeroValues verifies the decoder reads every
// field from the correct offset (no off-by-one into the header bytes) by
// feeding a frame with all payload bytes zero except the header.
func TestZcNotifyMapproperty2_0x099B_ZeroValues(t *testing.T) {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint16(data[0:], 0x099B) // only header set
	e := ZcNotifyMapproperty2_0x099B(data, 20200401)
	if e.Type != 0 {
		t.Errorf("Type: got %d want 0", e.Type)
	}
	if e.Flags != 0 {
		t.Errorf("Flags: got %#x want 0", e.Flags)
	}
}

// TestZcNotifyMapproperty2_0x099B_DecodesRegardlessOfPacketver verifies the
// 0x099B decoder is packetver-agnostic (the SYNTH_ZC_MAPPROPERTY_R2 struct is
// fixed-size and has no versioned branches).
func TestZcNotifyMapproperty2_0x099B_DecodesRegardlessOfPacketver(t *testing.T) {
	data := build0x099BFrame(1, 0x467)
	for _, pv := range []uint32{20130320, 20191120, 20200401, 20210101} {
		e := ZcNotifyMapproperty2_0x099B(data, pv)
		if e.Type != 1 {
			t.Errorf("pv=%d: Type got %d want 1", pv, e.Type)
		}
		if e.Flags != 0x467 {
			t.Errorf("pv=%d: Flags got %#x want 0x467", pv, e.Flags)
		}
	}
}

// TestZcNotifyMapproperty2_0x01D6_FlagsAlwaysZero documents the backward-compat
// invariant: the 4-byte 0x01D6 layout has no flags bitfield, so the decoder
// must always leave Flags at the zero value regardless of input bytes.
func TestZcNotifyMapproperty2_0x01D6_FlagsAlwaysZero(t *testing.T) {
	// Feed a 0x01D6 frame with type=1. The decoder must NOT try to read a
	// Flags field — there is no offset 4 in a 4-byte buffer.
	data := build0x01D6Frame(1)
	e := ZcNotifyMapproperty2_0x01D6(data, 20200401)
	if e.Type != 1 {
		t.Errorf("Type: got %d want 1", e.Type)
	}
	if e.Flags != 0 {
		t.Errorf("Flags: got %#x want 0 (0x01D6 layout has no flags bitfield)", e.Flags)
	}
}

// BenchmarkZcNotifyMapproperty2_0x099B verifies 0 allocs/op on the decode hot
// path. Both fields are scalar (int16, uint32) — no strings or slices — so
// the decoder must not allocate.
func BenchmarkZcNotifyMapproperty2_0x099B(b *testing.B) {
	data := build0x099BFrame(1, 0x467)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ZcNotifyMapproperty2_0x099B(data, 20200401)
	}
}

// BenchmarkZcNotifyMapproperty2_0x01D6 verifies 0 allocs/op for the legacy
// 4-byte variant. Scalar reads only — expect 0 allocs/op.
func BenchmarkZcNotifyMapproperty2_0x01D6(b *testing.B) {
	data := build0x01D6Frame(1)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ZcNotifyMapproperty2_0x01D6(data, 20200401)
	}
}
