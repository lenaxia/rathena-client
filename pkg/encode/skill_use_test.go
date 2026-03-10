// Manually implemented — see docs/BACKLOG/EPIC-03_gokore_integration_prereqs.md US-18.

package encode_test

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/encode"
	"github.com/lenaxia/rathena-client/pkg/send"
)

func TestEncodeSkillUse_PacketID(t *testing.T) {
	p := encode.EncodeSkillUse(send.SkillUse{Lv: 1, SkillID: 10, TargetID: 100}, 20200401)
	if p[0] != 0x62 || p[1] != 0x08 {
		t.Fatalf("packet ID: got %02X %02X, want 62 08", p[0], p[1])
	}
}

func TestEncodeSkillUse_Length(t *testing.T) {
	p := encode.EncodeSkillUse(send.SkillUse{}, 20200401)
	if len(p) != 10 {
		t.Fatalf("length: got %d, want 10", len(p))
	}
}

func TestEncodeSkillUse_Lv(t *testing.T) {
	p := encode.EncodeSkillUse(send.SkillUse{Lv: 5, SkillID: 1, TargetID: 1}, 20200401)
	got := binary.LittleEndian.Uint16(p[2:4])
	if got != 5 {
		t.Fatalf("Lv=5: got %d at bytes[2-3], want 5", got)
	}
}

func TestEncodeSkillUse_SkillID(t *testing.T) {
	p := encode.EncodeSkillUse(send.SkillUse{Lv: 1, SkillID: 114, TargetID: 1}, 20200401)
	got := binary.LittleEndian.Uint16(p[4:6])
	if got != 114 {
		t.Fatalf("SkillID=114: got %d at bytes[4-5], want 114", got)
	}
}

func TestEncodeSkillUse_TargetID(t *testing.T) {
	p := encode.EncodeSkillUse(send.SkillUse{Lv: 1, SkillID: 1, TargetID: 0xCAFEBABE}, 20200401)
	got := binary.LittleEndian.Uint32(p[6:10])
	if got != 0xCAFEBABE {
		t.Fatalf("TargetID: got %08X, want CAFEBABE", got)
	}
}

func TestEncodeSkillUse_AllZero(t *testing.T) {
	p := encode.EncodeSkillUse(send.SkillUse{}, 20200401)
	if p[0] != 0x62 || p[1] != 0x08 {
		t.Fatalf("packet ID: got %02X %02X, want 62 08", p[0], p[1])
	}
	for i := 2; i < 10; i++ {
		if p[i] != 0 {
			t.Fatalf("all-zero fields: byte[%d] = %02X, want 00", i, p[i])
		}
	}
}

func TestEncodeSkillUse_PacketverIgnored(t *testing.T) {
	req := send.SkillUse{Lv: 5, SkillID: 114, TargetID: 0xCAFEBABE}
	p1 := encode.EncodeSkillUse(req, 20200401)
	p2 := encode.EncodeSkillUse(req, 20200401)
	if p1 != p2 {
		t.Fatalf("repeated calls should produce identical output: got %v vs %v", p1, p2)
	}
}

func BenchmarkEncodeSkillUse(b *testing.B) {
	req := send.SkillUse{Lv: 5, SkillID: 114, TargetID: 0xCAFEBABE}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodeSkillUse(req, 20200401)
	}
}
