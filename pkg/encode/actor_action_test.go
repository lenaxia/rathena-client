// Manually implemented — see docs/BACKLOG/EPIC-03_gokore_integration_prereqs.md US-17.

package encode_test

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/encode"
	"github.com/lenaxia/rathena-client/pkg/send"
)

func TestEncodeActorAction_PacketID(t *testing.T) {
	p := encode.EncodeActorAction(send.ActorAction{TargetID: 1, Type: 7}, 20200401)
	if p[0] != 0x5A || p[1] != 0x08 {
		t.Fatalf("packet ID: got %02X %02X, want 5A 08", p[0], p[1])
	}
}

func TestEncodeActorAction_Length(t *testing.T) {
	p := encode.EncodeActorAction(send.ActorAction{}, 20200401)
	if len(p) != 7 {
		t.Fatalf("length: got %d, want 7", len(p))
	}
}

func TestEncodeActorAction_TargetID(t *testing.T) {
	p := encode.EncodeActorAction(send.ActorAction{TargetID: 0xDEADBEEF}, 20200401)
	got := binary.LittleEndian.Uint32(p[2:6])
	if got != 0xDEADBEEF {
		t.Fatalf("TargetID: got %08X, want DEADBEEF", got)
	}
}

func TestEncodeActorAction_TypeNormalAttack(t *testing.T) {
	p := encode.EncodeActorAction(send.ActorAction{Type: 7}, 20200401)
	if p[6] != 7 {
		t.Fatalf("Type=7: got %d at byte[6], want 7", p[6])
	}
}

func TestEncodeActorAction_TypeSitStand(t *testing.T) {
	p := encode.EncodeActorAction(send.ActorAction{Type: 0}, 20200401)
	if p[6] != 0 {
		t.Fatalf("Type=0: got %d at byte[6], want 0", p[6])
	}
}

func TestEncodeActorAction_TargetIDZero(t *testing.T) {
	p := encode.EncodeActorAction(send.ActorAction{TargetID: 0, Type: 7}, 20200401)
	got := binary.LittleEndian.Uint32(p[2:6])
	if got != 0 {
		t.Fatalf("TargetID=0: got %08X, want 00000000", got)
	}
	if p[0] != 0x5A || p[1] != 0x08 {
		t.Fatalf("packet ID corrupted: got %02X %02X", p[0], p[1])
	}
}

func TestEncodeActorAction_PacketverIgnored(t *testing.T) {
	req := send.ActorAction{TargetID: 0xCAFEBABE, Type: 2}
	p1 := encode.EncodeActorAction(req, 20200401)
	p2 := encode.EncodeActorAction(req, 20200401)
	if p1 != p2 {
		t.Fatalf("repeated calls should produce identical output: got %v vs %v", p1, p2)
	}
}

func BenchmarkEncodeActorAction(b *testing.B) {
	req := send.ActorAction{TargetID: 0xDEADBEEF, Type: 7}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodeActorAction(req, 20200401)
	}
}
