// Manually implemented — regression tests for worklog 0073.
// skill_use_location encoder must use shuffled wire ID, not hardcoded 0x0116.

package encode_test

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/encode"
	"github.com/lenaxia/rathena-client/pkg/send"
)

func sulPacketID(p [10]byte) uint16 {
	return binary.LittleEndian.Uint16(p[0:2])
}

// TestEncodeSkillUseLocation_PacketID_Table covers every explicit boundary
// from pv=20101124 onward plus shuffle era.
// Cross-validated: rAthena clif_shuffle.hpp:4738, OpenKore skill_use_location 0366.
func TestEncodeSkillUseLocation_PacketID_Table(t *testing.T) {
	req := send.SkillUseLocation{SkillLevel: 1, SkillID: 26, XPos: 100, YPos: 200}

	cases := []struct {
		name   string
		pv     uint32
		wantID uint16
	}{
		{"baseline pv=20030000", 20030000, 0x0116},
		{"pre-boundary pv=20101123", 20101123, 0x0116},

		// pv >= 20101124: 0x0366 — clif_packetdb.hpp:1388
		// OpenKore: ServerType0.pm '0366' => skill_use_location ✓
		{"boundary pv=20101124", 20101124, 0x0366},

		// pv >= 20111005: 0x0369
		{"boundary pv=20111005", 20111005, 0x0369},

		// pv >= 20120307: 0x0438 (20120418 block has no UseSkillToPos entry → inherits)
		{"boundary pv=20120307", 20120307, 0x0438},
		{"boundary pv=20120418", 20120418, 0x0438},
		{"pv=20120701", 20120701, 0x0438},

		// pv >= 20120702: 0x0863
		{"boundary pv=20120702", 20120702, 0x0863},

		// pv >= 20130320: 0x0959
		{"boundary pv=20130320", 20130320, 0x0959},
		{"pv=20130514", 20130514, 0x0959},

		// shuffle era — clif_shuffle.hpp pv=20130515: 0x0438
		{"shuffle boundary pv=20130515", 20130515, 0x0438},

		// post-shuffle stable: 0x0366 — clif_shuffle.hpp:4738
		{"post-shuffle pv=20200401", 20200401, 0x0366},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := encode.EncodeSkillUseLocation(req, tc.pv)
			got := sulPacketID(p)
			if got != tc.wantID {
				t.Errorf("pv=%d: packet ID = 0x%04X, want 0x%04X", tc.pv, got, tc.wantID)
			}
		})
	}
}

func TestEncodeSkillUseLocation_Length(t *testing.T) {
	p := encode.EncodeSkillUseLocation(send.SkillUseLocation{}, 20200401)
	if len(p) != 10 {
		t.Fatalf("length: got %d, want 10", len(p))
	}
}

// TestEncodeSkillUseLocation_Fields verifies all four fields at correct offsets.
// Source: clif_packetdb.hpp parseable_packet(..., 2, 4, 6, 8) — all uint16 LE.
func TestEncodeSkillUseLocation_Fields(t *testing.T) {
	req := send.SkillUseLocation{SkillLevel: 0x1111, SkillID: 0x2222, XPos: 0x3333, YPos: 0x4444}
	p := encode.EncodeSkillUseLocation(req, 20200401)

	if got := binary.LittleEndian.Uint16(p[2:4]); got != 0x1111 {
		t.Errorf("SkillLevel: got 0x%04X at [2:4], want 0x1111", got)
	}
	if got := binary.LittleEndian.Uint16(p[4:6]); got != 0x2222 {
		t.Errorf("SkillID: got 0x%04X at [4:6], want 0x2222", got)
	}
	if got := binary.LittleEndian.Uint16(p[6:8]); got != 0x3333 {
		t.Errorf("XPos: got 0x%04X at [6:8], want 0x3333", got)
	}
	if got := binary.LittleEndian.Uint16(p[8:10]); got != 0x4444 {
		t.Errorf("YPos: got 0x%04X at [8:10], want 0x4444", got)
	}
}

func BenchmarkEncodeSkillUseLocation(b *testing.B) {
	req := send.SkillUseLocation{SkillLevel: 1, SkillID: 26, XPos: 100, YPos: 200}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodeSkillUseLocation(req, 20200401)
	}
}
