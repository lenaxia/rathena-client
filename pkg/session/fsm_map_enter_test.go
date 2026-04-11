// Package session — internal tests for zcAcceptEnterID (ZC_ACCEPT_ENTER
// packet ID selection).
//
// Ground truth: src/map/packets.hpp:545-575
//
//	#if PACKETVER < 20080102
//	  DEFINE_PACKET_HEADER(ZC_ACCEPT_ENTER, 0x73)   // 11 bytes
//	#elif PACKETVER < 20141022 || PACKETVER >= 20160330
//	  DEFINE_PACKET_HEADER(ZC_ACCEPT_ENTER, 0x2eb)  // 13 bytes
//	#else  (>= 20141022 && < 20160330)
//	  DEFINE_PACKET_HEADER(ZC_ACCEPT_ENTER, 0xa18)  // 14 bytes
//	#endif
//
// Previous bug: the condition used >= 20141016 instead of >= 20141022.
// For pv ∈ [20141016, 20141021] the FSM registered 0x0A18 but rAthena sends
// 0x02EB → onMapEnter never fired → map entry timed out.
package session

import "testing"

// TestZcAcceptEnterID_Boundaries verifies the exact boundaries from packets.hpp.
func TestZcAcceptEnterID_Boundaries(t *testing.T) {
	cases := []struct {
		pv   uint32
		want uint16
		note string
	}{
		// 0x0073 era (< 20080102)
		{20080101, 0x0073, "one before 0x02EB era"},
		// 0x02EB era (>= 20080102, but < 20141022 or >= 20160330)
		{20080102, 0x02EB, "first 0x02EB packetver"},
		{20141021, 0x02EB, "one before 0x0A18 boundary (previously registered 0x0A18)"},
		// 0x0A18 era (>= 20141022 && < 20160330)
		{20141022, 0x0A18, "0x0A18 lower boundary (exact)"},
		{20150101, 0x0A18, "mid 0x0A18 range"},
		{20160329, 0x0A18, "one before 0x0A18 upper boundary"},
		// back to 0x02EB (>= 20160330)
		{20160330, 0x02EB, "0x02EB resumes at upper boundary"},
		{20200401, 0x02EB, "modern MAIN"},
	}
	for _, tc := range cases {
		got := zcAcceptEnterID(tc.pv)
		if got != tc.want {
			t.Errorf("pv=%d (%s): got 0x%04X, want 0x%04X",
				tc.pv, tc.note, got, tc.want)
		}
	}
}

// TestZcAcceptEnterID_PreBugRange covers pv ∈ [20141016, 20141021] — the six
// packetvers where the off-by-six-days bug caused incorrect registration.
// All must return 0x02EB (rAthena sends 0x02EB for PACKETVER < 20141022).
func TestZcAcceptEnterID_PreBugRange(t *testing.T) {
	for _, pv := range []uint32{20141016, 20141017, 20141018, 20141019, 20141020, 20141021} {
		got := zcAcceptEnterID(pv)
		if got != 0x02EB {
			t.Errorf("pv=%d: got 0x%04X, want 0x02EB", pv, got)
		}
	}
}
