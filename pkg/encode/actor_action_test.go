// Manually implemented — see docs/BACKLOG/EPIC-03_gokore_integration_prereqs.md US-17.

package encode_test

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/encode"
	"github.com/lenaxia/rathena-client/pkg/send"
)

// TestEncodeActorAction_PacketID verifies the wire packet ID for each
// PACKETVER range. Source: src/map/clif_shuffle.hpp and clif_packetdb.hpp.
func TestEncodeActorAction_PacketID(t *testing.T) {
	cases := []struct {
		name   string
		pv     uint32
		wantHi byte // p[1]
		wantLo byte // p[0]
		wantID string
	}{
		// PACKETVER > 20180307: stable wire ID 0x0437
		// Source: clif_shuffle.hpp parseable_packet(0x0437,7,clif_parse_ActionRequest,2,6)
		{"post-shuffle 20200401", 20200401, 0x04, 0x37, "0x0437"},
		{"post-shuffle 20180308", 20180308, 0x04, 0x37, "0x0437"},
		// PACKETVER == 20180307: shuffled to 0x0969
		// Source: clif_shuffle.hpp #elif PACKETVER == 20180307
		{"shuffle era 20180307", 20180307, 0x09, 0x69, "0x0969"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := encode.EncodeActorAction(send.ActorAction{TargetGID: 1, Action: 7}, tc.pv)
			if p[0] != tc.wantLo || p[1] != tc.wantHi {
				t.Fatalf("packetver=%d: packet ID got %02X %02X, want %02X %02X (%s)",
					tc.pv, p[0], p[1], tc.wantLo, tc.wantHi, tc.wantID)
			}
		})
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
}

func BenchmarkEncodeActorAction(b *testing.B) {
	req := send.ActorAction{TargetGID: 0xDEADBEEF, Action: 7}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodeActorAction(req, 20200401)
	}
}
