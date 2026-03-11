package session

// lengths_regression_test.go — packet length table regression assertions.
//
// These tests do NOT depend on OpenKore files at runtime; they assert specific
// known-correct values and mismatch thresholds derived from prior audits.
//
// To regenerate the pinned values, run codegen and compare against:
//   ~/personal/openkore/tables/kRO/RagexeRE_2018_06_21a/recvpackets.txt
//   ~/personal/openkore/tables/kRO/RagexeRE_2020_04_01b/recvpackets.txt

import "testing"

// helper: simulate the map length table at a given packetver.
func mapTableAt(pv uint32) [65536]int16 {
	var t [65536]int16
	populateMapLengths(pv, &t)
	return t
}

// TestLengthRegression_RankingPackets verifies that RANKLIST-based ranking
// packets compute to 282 bytes (was wrong at 42 before the 2D array fix).
// GCC verified: PACKET_ZC_BLACKSMITH_RANK at any packetver = 282 bytes.
func TestLengthRegression_RankingPackets(t *testing.T) {
	pv := uint32(20180621)
	table := mapTableAt(pv)

	cases := []struct {
		name string
		id   uint16
		want int16
	}{
		{"ZC_BLACKSMITH_RANK (0x0219)", 0x0219, 282},
		{"ZC_ALCHEMIST_RANK (0x021A)", 0x021A, 282},
		{"ZC_TAEKWON_RANK  (0x0226)", 0x0226, 282},
		{"ZC_KILLER_RANK   (0x0238)", 0x0238, 282},
	}
	for _, c := range cases {
		if got := table[c.id]; got != c.want {
			t.Errorf("pv=%d %s: got %d, want %d (RANKLIST 2D array collapse bug)",
				pv, c.name, got, c.want)
		}
	}
}

// TestLengthRegression_PacketID0071 verifies 0x0071 is always 28 bytes.
// Before the fix, the join pass wrongly emitted t[0x0071]=156 from
// PACKET_HC_NOTIFY_ZONESVR (a char server packet) at pv>=20170315.
func TestLengthRegression_PacketID0071(t *testing.T) {
	for _, pv := range []uint32{20140101, 20170315, 20180621, 20200401} {
		table := mapTableAt(pv)
		if got := table[0x0071]; got != 28 {
			t.Errorf("pv=%d: t[0x0071]=%d, want 28 (HC_NOTIFY_ZONESVR mis-mapping bug)",
				pv, got)
		}
	}
}

// TestLengthRegression_PacketID0092 verifies 0x0092 is 28 before pv=20170315
// and 0 (unknown/removed) after. Before the fix it was being assigned 156 at
// pv>=20170315 (PACKET_ZC_NPCACK_SERVERMOVE moved to 0x0AC7 at that version).
func TestLengthRegression_PacketID0092(t *testing.T) {
	cases := []struct {
		pv   uint32
		want int16 // 28 before move, 0 after move
	}{
		{20140101, 28},
		{20170314, 28},
		{20170315, 0}, // moved to 0x0AC7
		{20180621, 0},
		{20200401, 0},
	}
	for _, c := range cases {
		table := mapTableAt(c.pv)
		if got := table[0x0092]; got != c.want {
			t.Errorf("pv=%d: t[0x0092]=%d, want %d (ZC_NPCACK_SERVERMOVE version boundary bug)",
				c.pv, got, c.want)
		}
	}
}

// TestLengthRegression_PacketID0AC7 verifies 0x0AC7 gets the correct
// post-20170315 ZC_NPCACK_SERVERMOVE size (156 = 2+16+2+2+4+2+128).
func TestLengthRegression_PacketID0AC7(t *testing.T) {
	for _, pv := range []uint32{20170315, 20180621, 20200401} {
		table := mapTableAt(pv)
		if got := table[0x0AC7]; got != 156 {
			t.Errorf("pv=%d: t[0x0AC7]=%d, want 156 (ZC_NPCACK_SERVERMOVE post-20170315)",
				pv, got)
		}
	}
}

// TestLengthRegression_VariableLengthNotOverridden verifies that variable-
// length packets (-1 in clif_packetdb) are NOT overridden with fixed struct
// sizes from the S→C join pass or packets.hpp.
//
// GCC-verified: 0x0166, 0x09FD, 0x09FF are all -1 in clif_packetdb.hpp.
func TestLengthRegression_VariableLengthNotOverridden(t *testing.T) {
	pv := uint32(20180621)
	table := mapTableAt(pv)

	cases := []struct {
		name string
		id   uint16
	}{
		{"ZC_POSITION_ID_NAME_INFO (0x0166)", 0x0166},
		{"packet_unit_walking2    (0x09FD)", 0x09FD},
		{"packet_idle_unit        (0x09FF)", 0x09FF},
	}
	for _, c := range cases {
		if got := table[c.id]; got != -1 {
			t.Errorf("pv=%d %s: got %d, want -1 (variable-length override bug)",
				pv, c.name, got)
		}
	}
}

// TestLengthRegression_CharServerNotInMapTable verifies that char-server-only
// packet IDs (HC_NOTIFY_ZONESVR = 0x0081 pre-20170315, 0x0AC5 post) do NOT
// appear with the HC struct size (156 bytes) in the MAP server length table.
// They may appear with their actual map-server meaning (if any) at other lengths,
// but must NOT show 156.
func TestLengthRegression_CharServerNotInMapTable(t *testing.T) {
	// At pv=20180621: 0x0081 is ZC_ATTACK_FAILURE_FOR_NOENEMYSTATE (3 bytes in map server).
	// It must NOT be 156 (that would mean HC_NOTIFY_ZONESVR leaked into map server table).
	pv := uint32(20180621)
	table := mapTableAt(pv)

	if got := table[0x0081]; got == 156 {
		t.Errorf("pv=%d: t[0x0081]=156, should not be 156 (HC_NOTIFY_ZONESVR leaked into map server table)",
			pv)
	}
}
