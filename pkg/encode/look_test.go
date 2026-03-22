// Manually implemented — regression tests for worklog 0073.
// look encoder has a triple bug: wrong ID, wrong size ([4] vs [5]), wrong Dir offset.

package encode_test

import (
	"testing"

	"github.com/lenaxia/rathena-client/pkg/encode"
	"github.com/lenaxia/rathena-client/pkg/send"
)

// TestEncodeLook_PacketID_Table covers every explicit boundary from pv=20101124
// onward plus shuffle era and stable block.
// Cross-validated: rAthena clif_shuffle.hpp:4733, OpenKore actor_look_at 0361.
func TestEncodeLook_PacketID_Table(t *testing.T) {
	req := send.Look{HeadDir: 1, Dir: 2}

	cases := []struct {
		name   string
		pv     uint32
		wantID uint16
	}{
		// baseline — 0x009B
		{"baseline pv=20030000", 20030000, 0x009B},
		{"pre-boundary pv=20101123", 20101123, 0x009B},

		// pv >= 20101124: 0x0361 — clif_packetdb.hpp:1383
		// OpenKore: RagexeRE_2010_11_24a.pm actor_look_at 0361 ✓
		{"boundary pv=20101124", 20101124, 0x0361},
		{"pv=20111004", 20111004, 0x0361},

		// pv >= 20111005: 0x0366
		{"boundary pv=20111005", 20111005, 0x0366},

		// pv >= 20120307: 0x0890
		{"boundary pv=20120307", 20120307, 0x0890},

		// pv >= 20120410: 0x0871
		{"boundary pv=20120410", 20120410, 0x0871},

		// pv >= 20120418: 0x0202
		{"boundary pv=20120418", 20120418, 0x0202},

		// pv >= 20120702: 0x0960
		{"boundary pv=20120702", 20120702, 0x0960},

		// pv >= 20130320: 0x0897
		{"boundary pv=20130320", 20130320, 0x0897},
		{"pv=20130514", 20130514, 0x0897},

		// shuffle era — shuffledCtoSID(pv, 0x009B)
		// clif_shuffle.hpp pv=20130515: 0x0362
		{"shuffle boundary pv=20130515", 20130515, 0x0362},

		// post-shuffle stable — 0x0361 — clif_shuffle.hpp:4733, OpenKore ✓
		{"post-shuffle pv=20200401", 20200401, 0x0361},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := encode.EncodeLook(req, tc.pv)
			gotID := uint16(p[0]) | uint16(p[1])<<8
			if gotID != tc.wantID {
				t.Errorf("pv=%d: packet ID = 0x%04X, want 0x%04X", tc.pv, gotID, tc.wantID)
			}
		})
	}
}

// TestEncodeLook_Length verifies the wire packet is always 5 bytes.
// Bug: current encoder returns [4]byte, silently dropping the Dir field.
// Source: clif_shuffle.hpp:4733 parseable_packet(0x0361, 5, ...)
// OpenKore: pack 'v C' = 2(id) + 2(head) + 1(body) = 5 bytes total.
func TestEncodeLook_Length(t *testing.T) {
	p := encode.EncodeLook(send.Look{HeadDir: 0, Dir: 0}, 20200401)
	if len(p) != 5 {
		t.Fatalf("length: got %d, want 5", len(p))
	}
}

// TestEncodeLook_HeadDir verifies HeadDir at byte [2] (uint8).
// rAthena: RFIFOB(fd, pos[0]) where pos[0]=2
func TestEncodeLook_HeadDir(t *testing.T) {
	p := encode.EncodeLook(send.Look{HeadDir: 0xAB, Dir: 0}, 20200401)
	if p[2] != 0xAB {
		t.Fatalf("HeadDir: p[2] = 0x%02X, want 0xAB", p[2])
	}
}

// TestEncodeLook_Padding verifies byte [3] is always 0x00 (padding).
// rAthena: pos[1]=4 means offset 3 is never read — it is an implicit padding byte.
// OpenKore: pack 'v C' packs headDir as uint16 LE (headDir always 0-2, so p[3]=0).
func TestEncodeLook_Padding(t *testing.T) {
	p := encode.EncodeLook(send.Look{HeadDir: 0xFF, Dir: 0xFF}, 20200401)
	if p[3] != 0x00 {
		t.Fatalf("padding: p[3] = 0x%02X, want 0x00", p[3])
	}
}

// TestEncodeLook_Dir verifies Dir at byte [4] (uint8).
// Bug: current encoder places Dir at p[3], not p[4].
// rAthena: RFIFOB(fd, pos[1]) where pos[1]=4
// NOTE: This test uses a slice view so it compiles before the return type is
// fixed from [4]byte → [5]byte. After the fix it checks p[4] directly.
func TestEncodeLook_Dir(t *testing.T) {
	p := encode.EncodeLook(send.Look{HeadDir: 0, Dir: 0xCD}, 20200401)
	s := p[:]
	if len(s) < 5 {
		t.Fatalf("packet too short: len=%d, want 5 — look encoder still returns [4]byte (triple bug not fixed)", len(s))
	}
	if s[4] != 0xCD {
		t.Fatalf("Dir: p[4] = 0x%02X, want 0xCD", s[4])
	}
}

// TestEncodeLook_DirNotAtOffset3 explicitly asserts the pre-fix wrong position.
// Before the fix, Dir was written to p[3]. This test documents the regression.
func TestEncodeLook_DirNotAtOffset3(t *testing.T) {
	p := encode.EncodeLook(send.Look{HeadDir: 0, Dir: 5}, 20200401)
	s := p[:]
	// p[3] must be padding (0x00), not Dir
	if len(s) >= 4 && s[3] == 5 {
		t.Fatal("Dir must not be at offset 3 — it belongs at offset 4 (rAthena pos[1]=4)")
	}
}

func BenchmarkEncodeLook(b *testing.B) {
	req := send.Look{HeadDir: 1, Dir: 3}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodeLook(req, 20200401)
	}
}
