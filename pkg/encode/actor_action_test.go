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

// TestEncodeActorAction_AllBreakpoints verifies the correct wire packet ID is
// emitted for every distinct packetver breakpoint.
// Sources:
//
//	clif_packetdb.hpp — parseable_packet(0xNNNN,7,clif_parse_ActionRequest,2,6)
//	clif_shuffle.hpp  — per-week exact-match shuffle assignments for 0x0089
func TestEncodeActorAction_AllBreakpoints(t *testing.T) {
	cases := []struct {
		name   string
		pv     uint32
		wantID uint16
	}{
		// clif_packetdb.hpp line 38: default baseline
		{"pre-2004 baseline", 20030000, 0x0089},
		// clif_packetdb.hpp >= 20040726: 0x0193
		{"20040726", 20040726, 0x0193},
		// clif_packetdb.hpp >= 20040906: 0x0085
		{"20040906", 20040906, 0x0085},
		// clif_packetdb.hpp >= 20041129: 0x009f
		{"20041129", 20041129, 0x009f},
		// clif_packetdb.hpp >= 20050110: 0x0190
		{"20050110", 20050110, 0x0190},
		{"20080909", 20080909, 0x0190},
		// clif_packetdb.hpp >= 20080910: 0x0437
		{"20080910", 20080910, 0x0437},
		{"20111101", 20111101, 0x0437},
		// clif_packetdb.hpp >= 20111102: 0x08aa
		{"20111102", 20111102, 0x08aa},
		// clif_packetdb.hpp >= 20120307: 0x0885
		{"20120307", 20120307, 0x0885},
		// clif_packetdb.hpp >= 20120410: 0x0369
		{"20120410", 20120410, 0x0369},
		// clif_packetdb.hpp >= 20120702: 0x085a
		{"20120702", 20120702, 0x085a},
		// clif_packetdb.hpp >= 20130320: 0x088e
		{"20130320 (bug range start)", 20130320, 0x088e},
		{"20130514 (bug range end)", 20130514, 0x088e},
		// clif_shuffle.hpp == 20130515: 0x0369
		{"20130515 (shuffle era start)", 20130515, 0x0369},
		// clif_shuffle.hpp == 20130522: 0x08A2
		{"20130522", 20130522, 0x08A2},
		// clif_shuffle.hpp == 20180307: 0x0969
		{"20180307 (last shuffle week)", 20180307, 0x0969},
		// clif_shuffle.hpp > 20180307: 0x0437
		{"20180308 (post-shuffle)", 20180308, 0x0437},
		{"20200401", 20200401, 0x0437},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := encode.EncodeActorAction(send.ActorAction{TargetGID: 1, Action: 7}, tc.pv)
			got := uint16(p[0]) | uint16(p[1])<<8
			if got != tc.wantID {
				t.Fatalf("pv=%d: wire ID = 0x%04X, want 0x%04X", tc.pv, got, tc.wantID)
			}
		})
	}
}
