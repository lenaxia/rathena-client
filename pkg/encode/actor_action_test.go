// Manually implemented — see docs/BACKLOG/EPIC-03_gokore_integration_prereqs.md US-17.

package encode_test

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/encode"
	"github.com/lenaxia/rathena-client/pkg/send"
)

func TestEncodeActorAction_PacketID(t *testing.T) {
	p := encode.EncodeActorAction(send.ActorAction{TargetGID: 1, Action: 7}, 20200401)
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

func TestEncodeActorAction_TargetGID(t *testing.T) {
	p := encode.EncodeActorAction(send.ActorAction{TargetGID: 0xDEADBEEF}, 20200401)
	got := binary.LittleEndian.Uint32(p[2:6])
	if got != 0xDEADBEEF {
		t.Fatalf("TargetGID: got %08X, want DEADBEEF", got)
	}
}

func TestEncodeActorAction_ActionNormalAttack(t *testing.T) {
	p := encode.EncodeActorAction(send.ActorAction{Action: 7}, 20200401)
	if p[6] != 7 {
		t.Fatalf("Action=7: got %d at byte[6], want 7", p[6])
	}
}

func TestEncodeActorAction_ActionSitStand(t *testing.T) {
	p := encode.EncodeActorAction(send.ActorAction{Action: 0}, 20200401)
	if p[6] != 0 {
		t.Fatalf("Action=0: got %d at byte[6], want 0", p[6])
	}
}

func TestEncodeActorAction_TargetGIDZero(t *testing.T) {
	p := encode.EncodeActorAction(send.ActorAction{TargetGID: 0, Action: 7}, 20200401)
	got := binary.LittleEndian.Uint32(p[2:6])
	if got != 0 {
		t.Fatalf("TargetGID=0: got %08X, want 00000000", got)
	}
	if p[0] != 0x5A || p[1] != 0x08 {
		t.Fatalf("packet ID corrupted: got %02X %02X", p[0], p[1])
	}
}

func TestEncodeActorAction_PacketverIgnored(t *testing.T) {
	req := send.ActorAction{TargetGID: 0xCAFEBABE, Action: 2}
	p1 := encode.EncodeActorAction(req, 20200401)
	p2 := encode.EncodeActorAction(req, 20200401)
	if p1 != p2 {
		t.Fatalf("repeated calls should produce identical output: got %v vs %v", p1, p2)
	}
}

func BenchmarkEncodeActorAction(b *testing.B) {
	req := send.ActorAction{TargetGID: 0xDEADBEEF, Action: 7}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodeActorAction(req, 20200401)
	}
}
