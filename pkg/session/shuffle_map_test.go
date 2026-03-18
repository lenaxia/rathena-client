package session_test

import (
	"testing"

	"github.com/lenaxia/rathena-client/pkg/session"
)

// TestShuffledCtoSID_PostShuffle verifies that for PACKETVER > 20180307,
// ShuffledCtoSID returns the stable post-shuffle wire IDs defined in the
// PACKETVER > 20180307 block of src/map/clif_shuffle.hpp.
//
// Source: clif_shuffle.hpp comment "Clients after 2018-03-07bRagexeRE do not
// have shuffled packets anymore" + parseable_packet entries in that block.
func TestShuffledCtoSID_PostShuffle(t *testing.T) {
	cases := []struct {
		name   string
		pv     uint32
		baseID uint16
		wantID uint16
	}{
		// 0x0085 (clif_parse_WalkToXY baseline) → 0x035F post-shuffle
		// Source: clif_shuffle.hpp parseable_packet(0x035F,5,clif_parse_WalkToXY,2)
		{"walk post-shuffle 20200401", 20200401, 0x0085, 0x035F},
		{"walk post-shuffle 20181001", 20181001, 0x0085, 0x035F},
		{"walk post-shuffle 20180308", 20180308, 0x0085, 0x035F},

		// 0x0089 (clif_parse_ActionRequest baseline) → 0x0437 post-shuffle
		// Source: clif_shuffle.hpp parseable_packet(0x0437,7,clif_parse_ActionRequest,2,6)
		{"action post-shuffle 20200401", 20200401, 0x0089, 0x0437},
		{"action post-shuffle 20181001", 20181001, 0x0089, 0x0437},
		{"action post-shuffle 20180308", 20180308, 0x0089, 0x0437},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := session.ShuffledCtoSID(tc.pv, tc.baseID)
			if got != tc.wantID {
				t.Errorf("ShuffledCtoSID(%d, 0x%04X) = 0x%04X, want 0x%04X",
					tc.pv, tc.baseID, got, tc.wantID)
			}
		})
	}
}

// TestShuffledCtoSID_ShuffleEra verifies that exact-match shuffle versions
// still return the correct per-version shuffled wire IDs.
//
// Source: clif_shuffle.hpp parseable_packet entries for specific versions.
func TestShuffledCtoSID_ShuffleEra(t *testing.T) {
	cases := []struct {
		name   string
		pv     uint32
		baseID uint16
		wantID uint16
	}{
		// 20180307 (last shuffle version): 0x0089 → 0x0969
		// Source: clif_shuffle.hpp #elif PACKETVER == 20180307
		{"action 20180307", 20180307, 0x0089, 0x0969},
		// 20180307: 0x0085 → 0x0877
		{"walk 20180307", 20180307, 0x0085, 0x0877},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := session.ShuffledCtoSID(tc.pv, tc.baseID)
			if got != tc.wantID {
				t.Errorf("ShuffledCtoSID(%d, 0x%04X) = 0x%04X, want 0x%04X",
					tc.pv, tc.baseID, got, tc.wantID)
			}
		})
	}
}
