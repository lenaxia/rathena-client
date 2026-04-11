// Package session — internal tests for fsmEncodeMapLogin field layout and
// PACKETVER-conditional length.
//
// Ground truth: src/map/clif_shuffle.hpp
//
//	PACKETVER > 20180307, default (19-byte variant):
//	  parseable_packet( 0x0436, 19, clif_parse_WantToConnection, 2, 6, 10, 14, 18 );
//	  Layout: id(2) + AID(4) + GID(4) + AuthCode(4) + clientTime(4) + sex(1) = 19 bytes
//	  sex at offset 18
//	  Source: clif_shuffle.hpp:4747
//
//	23-byte variant (two conditions, same layout):
//	  #if PACKETVER_RE_NUM >= 20211103 || PACKETVER_MAIN_NUM >= 20220330
//	    parseable_packet( 0x0436, 23, clif_parse_WantToConnection, 2, 6, 10, 14, 22 );
//	  Layout: id(2) + AID(4) + GID(4) + AuthCode(4) + clientTime(4) + tick(4) + sex(1) = 23 bytes
//	  sex at offset 22
//	  Source: clif_shuffle.hpp:4744-4745
//
//	  PACKETVER_RE_NUM >= 20211103 fires for packetver in [20211103, 20211118]
//	  (config/packets.hpp: RE window is (pv >= 20200902 && pv <= 20211118), intersected with >= 20211103)
//	  PACKETVER_MAIN_NUM >= 20220330 fires for packetver >= 20220330 (outside RE window)
//
// rAthena ingress (clif.cpp:10625): clif_parse_WantToConnection_sub checks
// packet_len == packet_db[cmd].len. Sending 19 bytes when rAthena expects 23
// triggers set_eof(fd) → immediate disconnection.
//
// GCC verification commands:
//
//	# RE=20211103 → 23 bytes
//	g++ -E -P -DPACKETVER=20211103 -DPACKETVER_RE_NUM=20211103 -DPACKETVER_MAIN_NUM=0 \
//	    -I ~/personal/rathena/src -I ~/personal/rathena/src/map -I ~/personal/rathena/src/common \
//	    -include internal/codegen/stubs/packets_hpp_stub.h \
//	    /tmp/clif_trace.cpp 2>/dev/null | grep 0x0436
//	→ last write: clif_shuffle.hpp:4745  0x0436 23 clif_parse_WantToConnection
//
//	# MAIN=20220330 → 23 bytes
//	g++ -E -P -DPACKETVER=20220330 -DPACKETVER_MAIN_NUM=20220330 -DPACKETVER_RE_NUM=0 ... | grep 0x0436
//	→ last write: clif_shuffle.hpp:4745  0x0436 23 clif_parse_WantToConnection
//
//	# MAIN=20220329 → 19 bytes
//	g++ -E -P -DPACKETVER=20220329 -DPACKETVER_MAIN_NUM=20220329 -DPACKETVER_RE_NUM=0 ... | grep 0x0436
//	→ last write: clif_shuffle.hpp:4747  0x0436 19 clif_parse_WantToConnection
package session

import (
	"encoding/binary"
	"testing"
)

// TestFsmEncodeMapLogin_Length_19bytes verifies that packetvers outside the
// 23-byte windows produce a 19-byte packet with sex at offset 18.
//
// Source: clif_shuffle.hpp:4747
// parseable_packet( 0x0436, 19, clif_parse_WantToConnection, 2, 6, 10, 14, 18 );
func TestFsmEncodeMapLogin_Length_19bytes(t *testing.T) {
	cases := []struct {
		name string
		pv   uint32
	}{
		// MAIN variants — PACKETVER_RE not set
		{"main 20180308 (post-shuffle start)", 20180308},
		{"main 20200401", 20200401},
		// RE variants before the 20211103 boundary
		// config/packets.hpp: PACKETVER_RE set when (pv >= 20200902 && pv <= 20211118);
		// intersected with < 20211103 gives [20200902, 20211102].
		{"re 20200902 (first RE post-20200901)", 20200902},
		{"re 20211102 (one day before RE boundary)", 20211102},
		// MAIN after RE window closes — one past window end
		{"main 20211119 (one past RE window)", 20211119},
		// MAIN before MAIN boundary
		{"main 20220329 (one day before MAIN boundary)", 20220329},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := fsmEncodeMapLogin(1000, 2000, 3000, 1, tc.pv)
			if len(p) != 19 {
				t.Errorf("pv=%d: len=%d, want 19", tc.pv, len(p))
			}
		})
	}
}

// TestFsmEncodeMapLogin_Length_23bytes verifies that packetvers in the two
// 23-byte windows produce a 23-byte packet with sex at offset 22.
//
// Source: clif_shuffle.hpp:4744-4745
// #if PACKETVER_RE_NUM >= 20211103 || PACKETVER_MAIN_NUM >= 20220330
//
//	parseable_packet( 0x0436, 23, clif_parse_WantToConnection, 2, 6, 10, 14, 22 );
//
// RE window [20211103, 20211118]: PACKETVER_RE_NUM=PACKETVER, condition fires.
// MAIN >= 20220330: PACKETVER_MAIN_NUM=PACKETVER (RE window closed), condition fires.
// GCC-verified: 20211103→23B, 20211118→23B, 20220330→23B, 20220401→23B.
func TestFsmEncodeMapLogin_Length_23bytes(t *testing.T) {
	cases := []struct {
		name string
		pv   uint32
	}{
		// RE window: [20211103, 20211118]
		{"re 20211103 (RE boundary)", 20211103},
		{"re 20211110", 20211110},
		{"re 20211118 (last RE in window)", 20211118},
		// MAIN >= 20220330
		{"main 20220330 (MAIN boundary)", 20220330},
		{"main 20220401", 20220401},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := fsmEncodeMapLogin(1000, 2000, 3000, 1, tc.pv)
			if len(p) != 23 {
				t.Errorf("pv=%d: len=%d, want 23 (rAthena clif_parse_WantToConnection_sub disconnects on length mismatch)", tc.pv, len(p))
			}
		})
	}
}

// TestFsmEncodeMapLogin_PacketID verifies the wire packet ID is always 0x0436
// (little-endian) for all supported packetvers, including 23-byte variants.
//
// Source: clif_shuffle.hpp stable block > 20180307; both the 19-byte and
// 23-byte variants use packet ID 0x0436.
func TestFsmEncodeMapLogin_PacketID(t *testing.T) {
	for _, pv := range []uint32{20180308, 20200401, 20211103, 20211118, 20220330, 20220401} {
		p := fsmEncodeMapLogin(0, 0, 0, 0, pv)
		id := binary.LittleEndian.Uint16(p[0:2])
		if id != 0x0436 {
			t.Errorf("pv=%d: packet ID = 0x%04X, want 0x0436", pv, id)
		}
	}
}

// TestFsmEncodeMapLogin_Fields_19byte verifies field layout for the 19-byte variant.
//
// Layout (clif_shuffle.hpp:4747, positions 2,6,10,14,18):
//
//	offset 0-1:  packet ID (0x0436)
//	offset 2-5:  AID       (pos[0]=2)
//	offset 6-9:  GID       (pos[1]=6)
//	offset 10-13: AuthCode (pos[2]=10)
//	offset 14-17: clientTime (pos[3]=14)
//	offset 18:   sex       (pos[4]=18)
func TestFsmEncodeMapLogin_Fields_19byte(t *testing.T) {
	const aid = uint32(2000001)
	const gid = uint32(150001)
	const auth = uint32(0xDEADBEEF)
	const sex = uint8(1)

	p := fsmEncodeMapLogin(aid, gid, auth, sex, 20200401)

	if got := binary.LittleEndian.Uint32(p[2:6]); got != aid {
		t.Errorf("AID at [2:6]: got %d, want %d", got, aid)
	}
	if got := binary.LittleEndian.Uint32(p[6:10]); got != gid {
		t.Errorf("GID at [6:10]: got %d, want %d", got, gid)
	}
	if got := binary.LittleEndian.Uint32(p[10:14]); got != auth {
		t.Errorf("AuthCode at [10:14]: got 0x%08X, want 0x%08X", got, auth)
	}
	// clientTime is always 0 (rAthena does not care about this field)
	if got := binary.LittleEndian.Uint32(p[14:18]); got != 0 {
		t.Errorf("clientTime at [14:18]: got %d, want 0", got)
	}
	if p[18] != sex {
		t.Errorf("sex at [18]: got %d, want %d", p[18], sex)
	}
}

// TestFsmEncodeMapLogin_Fields_23byte verifies field layout for the 23-byte variant
// across both windows that produce it: RE pv=20211103 and MAIN pv=20220330.
//
// Layout (clif_shuffle.hpp:4745, positions 2,6,10,14,22):
//
//	offset 0-1:  packet ID   (0x0436)
//	offset 2-5:  AID         (pos[0]=2)
//	offset 6-9:  GID         (pos[1]=6)
//	offset 10-13: AuthCode   (pos[2]=10)
//	offset 14-17: clientTime (pos[3]=14)  ← rAthena reads clientTick from here
//	offset 18-21: tick       (extra field not present in 19-byte variant)
//	offset 22:   sex         (pos[4]=22)
//
// OpenKore RagexeRE_2021_11_03.pm confirms:
//
//	'0436' => ['map_login', 'a4 a4 a4 V2 C', [qw(accountID charID sessionID unknown tick sex)]]
func TestFsmEncodeMapLogin_Fields_23byte(t *testing.T) {
	const aid = uint32(2000001)
	const gid = uint32(150001)
	const auth = uint32(0xDEADBEEF)
	const sex = uint8(1)

	// Both the RE window (20211103) and the MAIN window (20220330) take the same
	// code path and produce identical layout. Test both to catch any future
	// refactor that splits the two conditions into separate branches.
	for _, pv := range []uint32{20211103, 20220330} {
		p := fsmEncodeMapLogin(aid, gid, auth, sex, pv)

		if got := binary.LittleEndian.Uint32(p[2:6]); got != aid {
			t.Errorf("pv=%d AID at [2:6]: got %d, want %d", pv, got, aid)
		}
		if got := binary.LittleEndian.Uint32(p[6:10]); got != gid {
			t.Errorf("pv=%d GID at [6:10]: got %d, want %d", pv, got, gid)
		}
		if got := binary.LittleEndian.Uint32(p[10:14]); got != auth {
			t.Errorf("pv=%d AuthCode at [10:14]: got 0x%08X, want 0x%08X", pv, got, auth)
		}
		// rAthena reads clientTick from pos[3]=14; send 0.
		if got := binary.LittleEndian.Uint32(p[14:18]); got != 0 {
			t.Errorf("pv=%d clientTime at [14:18]: got %d, want 0", pv, got)
		}
		// tick at [18:22]: extra field; send 0.
		if got := binary.LittleEndian.Uint32(p[18:22]); got != 0 {
			t.Errorf("pv=%d tick at [18:22]: got %d, want 0", pv, got)
		}
		if p[22] != sex {
			t.Errorf("pv=%d sex at [22]: got %d, want %d", pv, p[22], sex)
		}
	}
}

// TestFsmEncodeMapLogin_Boundary verifies both exact PACKETVER boundaries:
//
//   - RE boundary: pv=20211102 → 19 bytes; pv=20211103 → 23 bytes; pv=20211118 → 23 bytes
//   - RE window end: pv=20211119 → 19 bytes (outside RE window, MAIN, below 20220330)
//   - MAIN boundary: pv=20220329 → 19 bytes; pv=20220330 → 23 bytes
//
// GCC-verified boundaries: 20211103→23B, 20211118→23B, 20211119→19B, 20220329→19B, 20220330→23B.
func TestFsmEncodeMapLogin_Boundary(t *testing.T) {
	cases := []struct {
		pv   uint32
		want int
	}{
		{20211102, 19}, // one before RE boundary
		{20211103, 23}, // RE boundary (exact)
		{20211118, 23}, // last in RE window
		{20211119, 19}, // one past RE window end (MAIN, below 20220330)
		{20220329, 19}, // one before MAIN boundary
		{20220330, 23}, // MAIN boundary (exact)
	}
	for _, tc := range cases {
		p := fsmEncodeMapLogin(1, 2, 3, 0, tc.pv)
		if len(p) != tc.want {
			t.Errorf("pv=%d: len=%d, want %d", tc.pv, len(p), tc.want)
		}
	}
}

// TestFsmEncodeMapLogin_SexPreserved_23byte verifies sex is correctly placed
// at offset 22 (not 18) in the 23-byte variant, for both male and female,
// across both 23-byte windows (RE and MAIN).
func TestFsmEncodeMapLogin_SexPreserved_23byte(t *testing.T) {
	for _, pv := range []uint32{20211103, 20220330} {
		male := fsmEncodeMapLogin(1, 2, 3, 1, pv)
		if male[22] != 1 {
			t.Errorf("pv=%d male sex: byte[22]=%d, want 1", pv, male[22])
		}
		female := fsmEncodeMapLogin(1, 2, 3, 0, pv)
		if female[22] != 0 {
			t.Errorf("pv=%d female sex: byte[22]=%d, want 0", pv, female[22])
		}
	}
}
