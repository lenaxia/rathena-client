// Package decode — tests for the 0x0B1A variant of ZC_USESKILL_ACK (skill_cast).
//
// PACKET_ZC_USESKILL_ACK at packetver >= 20181212 (struct: packets_struct.hpp:3952-3963):
//
//	offset 0  : packetType int16   (0x0B1A)
//	offset 2  : srcId     uint32
//	offset 6  : dstId     uint32
//	offset 10 : x         uint16
//	offset 12 : y         uint16
//	offset 14 : skillId   uint16
//	offset 16 : element   uint32
//	offset 20 : delayTime uint32
//	offset 24 : disposable uint8
//	offset 25 : attackMT  uint32   // NEW vs 0x07FB; total 29 bytes
//
// This is the same semantic action as 0x07FB but adds the attackMT field at the
// tail. The events.SkillCast struct already carries an AttackMT field.
package decode

import (
	"encoding/binary"
	"testing"
)

// build0x0B1AFrame builds a 29-byte 0x0B1A frame from explicit field values,
// writing each field at its GCC-verified offset.
func build0x0B1AFrame(srcID, dstID uint32, x, y, skillID uint16, element, delayTime uint32, disposable uint8, attackMT uint32) []byte {
	b := make([]byte, 29)
	binary.LittleEndian.PutUint16(b[0:], 0x0B1A)
	binary.LittleEndian.PutUint32(b[2:], srcID)
	binary.LittleEndian.PutUint32(b[6:], dstID)
	binary.LittleEndian.PutUint16(b[10:], x)
	binary.LittleEndian.PutUint16(b[12:], y)
	binary.LittleEndian.PutUint16(b[14:], skillID)
	binary.LittleEndian.PutUint32(b[16:], element)
	binary.LittleEndian.PutUint32(b[20:], delayTime)
	b[24] = disposable
	binary.LittleEndian.PutUint32(b[25:], attackMT)
	return b
}

// TestSkillCast_0x0B1A_AllFields verifies every field (including AttackMT at
// offset 25) decodes from the correct offsets.
func TestSkillCast_0x0B1A_AllFields(t *testing.T) {
	data := build0x0B1AFrame(
		0x12345678, // srcId   [2:6]
		0x9ABCDEF0, // dstId   [6:10]
		0x1111,     // x       [10:12]
		0x2222,     // y       [12:14]
		0x03E8,     // skillId [14:16] (1000)
		0x0A0A0A0A, // element [16:20]
		0x00FF00FF, // delayTime [20:24]
		0x01,       // disposable [24]
		0x42424242, // attackMT [25:29]
	)

	e := SkillCast_0x0B1A(data, 20200401)

	if e.SrcId != 0x12345678 {
		t.Errorf("SrcId: got %#x want 0x12345678", e.SrcId)
	}
	if e.DstId != 0x9ABCDEF0 {
		t.Errorf("DstId: got %#x want 0x9ABCDEF0", e.DstId)
	}
	if e.X != 0x1111 {
		t.Errorf("X: got %#x want 0x1111", e.X)
	}
	if e.Y != 0x2222 {
		t.Errorf("Y: got %#x want 0x2222", e.Y)
	}
	if e.SkillId != 0x03E8 {
		t.Errorf("SkillId: got %#x want 0x03E8", e.SkillId)
	}
	if e.Element != 0x0A0A0A0A {
		t.Errorf("Element: got %#x want 0x0A0A0A0A", e.Element)
	}
	if e.DelayTime != 0x00FF00FF {
		t.Errorf("DelayTime: got %#x want 0x00FF00FF", e.DelayTime)
	}
	if e.Disposable != 0x01 {
		t.Errorf("Disposable: got %d want 1", e.Disposable)
	}
	if e.AttackMT != 0x42424242 {
		t.Errorf("AttackMT: got %#x want 0x42424242 (must be decoded from offset 25)", e.AttackMT)
	}
}

// TestSkillCast_0x0B1A_AttackMT_FromOffset25 specifically pins AttackMT to the
// 0x0B1A-only trailing field. Writing a distinct sentinel at [25:29] must reach
// AttackMT — this is the regression guard for the gap that 0x0B1A is added to fix.
func TestSkillCast_0x0B1A_AttackMT_FromOffset25(t *testing.T) {
	data := make([]byte, 29)
	binary.LittleEndian.PutUint16(data[0:], 0x0B1A)
	binary.LittleEndian.PutUint32(data[25:], 0xDEADBEEF) // attackMT sentinel

	e := SkillCast_0x0B1A(data, 20200401)
	if e.AttackMT != 0xDEADBEEF {
		t.Fatalf("AttackMT: got %#x want 0xDEADBEEF (offset 25, size 4)", e.AttackMT)
	}
}

// TestSkillCast_0x0B1A_ZeroValues verifies the all-zero frame decodes to zero
// values for every field (no off-by-one into the packet header).
func TestSkillCast_0x0B1A_ZeroValues(t *testing.T) {
	data := make([]byte, 29)
	binary.LittleEndian.PutUint16(data[0:], 0x0B1A) // only header set

	e := SkillCast_0x0B1A(data, 20200401)

	if e.SrcId != 0 || e.DstId != 0 || e.X != 0 || e.Y != 0 || e.SkillId != 0 ||
		e.Element != 0 || e.DelayTime != 0 || e.Disposable != 0 || e.AttackMT != 0 {
		t.Errorf("zero frame decoded non-zero: %+v", e)
	}
}

// TestSkillCast_0x0B1A_DecodesRegardlessOfPacketver verifies the 0x0B1A decoder
// is packetver-agnostic — it always reads the full 29-byte layout (the 0x0B1A
// struct only exists at packetver >= 20181212, so there are no legacy branches).
func TestSkillCast_0x0B1A_DecodesRegardlessOfPacketver(t *testing.T) {
	data := build0x0B1AFrame(7, 8, 9, 10, 11, 12, 13, 1, 14)

	for _, pv := range []uint32{20181212, 20191120, 20200401, 20210101} {
		e := SkillCast_0x0B1A(data, pv)
		if e.AttackMT != 14 {
			t.Errorf("pv=%d: AttackMT got %d want 14", pv, e.AttackMT)
		}
		if e.SkillId != 11 {
			t.Errorf("pv=%d: SkillId got %d want 11", pv, e.SkillId)
		}
	}
}

// BenchmarkSkillCast_0x0B1A verifies 0 allocs/op on the decode hot path.
// SkillCast has no string fields — expect 0 allocs/op.
func BenchmarkSkillCast_0x0B1A(b *testing.B) {
	data := build0x0B1AFrame(1, 2, 3, 4, 5, 6, 7, 1, 8)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = SkillCast_0x0B1A(data, 20200401)
	}
}
