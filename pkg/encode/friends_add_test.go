// Manually implemented — regression tests for worklog 0074.
// EncodeFriendsAdd must use shuffledCtoSID for pv >= 20130515.

package encode_test

import (
	"testing"

	"github.com/lenaxia/rathena-client/pkg/encode"
	"github.com/lenaxia/rathena-client/pkg/send"
)

// TestEncodeFriendsAdd_PacketID_Table verifies the wire ID at key packetvers.
// 0x0202 base ID IS in clif_shuffle.hpp — it is shuffled in the 20130515–20180307 era.
// After > 20180307 the stable block returns baseID (0x0202) for unknown entries.
// Cross-validated: clif_packetdb.hpp:259 (stable 0x0202), clif_shuffle.hpp (per-week remaps).
func TestEncodeFriendsAdd_PacketID_Table(t *testing.T) {
	req := send.FriendsAdd{Name: "TestPlayer"}

	cases := []struct {
		name   string
		pv     uint32
		wantID uint16
	}{
		// pv < 20130515: 0x0202 — single stable entry in clif_packetdb.hpp:259
		{"baseline pv=20030000", 20030000, 0x0202},
		{"pre-shuffle pv=20130514", 20130514, 0x0202},

		// pv >= 20130515: shuffledCtoSID(pv, 0x0202)
		// clif_shuffle.hpp pv=20130515: 0x0202 → 0x0962
		{"shuffle boundary pv=20130515", 20130515, 0x0962},
		// clif_shuffle.hpp pv=20130522: 0x0202 → 0x0362
		{"shuffle pv=20130522", 20130522, 0x0362},
		// clif_shuffle.hpp pv=20180307: 0x0202 → 0x08AA
		{"last shuffle pv=20180307", 20180307, 0x08AA},

		// pv > 20180307: stable post-shuffle — 0x0202 not in stable override list → returns baseID
		{"post-shuffle pv=20200401", 20200401, 0x0202},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := encode.EncodeFriendsAdd(req, tc.pv)
			gotID := uint16(p[0]) | uint16(p[1])<<8
			if gotID != tc.wantID {
				t.Errorf("pv=%d: packet ID = 0x%04X, want 0x%04X", tc.pv, gotID, tc.wantID)
			}
		})
	}
}

func TestEncodeFriendsAdd_Length(t *testing.T) {
	p := encode.EncodeFriendsAdd(send.FriendsAdd{}, 20200401)
	if len(p) != 26 {
		t.Fatalf("length: got %d, want 26", len(p))
	}
}

func BenchmarkEncodeFriendsAdd(b *testing.B) {
	req := send.FriendsAdd{Name: "TestPlayer"}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodeFriendsAdd(req, 20200401)
	}
}
