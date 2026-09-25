// Manually implemented — regression test for the goKore bug report in
// docs/07_WORK_LOG/1248_2026-09-25_bot_party_formation.md §R4 (issue #385,
// PR #421): ZC party invite-ack was not decoded. ActionZcPartyJoinReqAck,
// ZcPartyJoinReqAck_0x02C5 (modern), and ZcPartyJoinReqAck_0x00FD (legacy)
// must exist and decode correctly.

package decode_test

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/decode"
	"github.com/lenaxia/rathena-client/pkg/session"
)

// buildZcPartyJoinReqAck0x02C5 builds a PACKET_ZC_PARTY_JOIN_REQ_ACK modern
// packet. GCC layout at pv=20200401 (packets_struct.hpp:5093-5101):
//
//	int16  PacketType     offset 0  (2 bytes)
//	char   characterName[24] offset 2  (24 bytes) — NUL-padded
//	int    result         offset 26 (4 bytes)
//
// Total: 30 bytes. Header 0x02C5 for PACKETVER >= 20070821
// (packets_struct.hpp:5101, clif.cpp:7947).
func buildZcPartyJoinReqAck0x02C5(characterName string, result int32) []byte {
	b := make([]byte, 30)
	binary.LittleEndian.PutUint16(b[0:], 0x02C5)
	copy(b[2:26], characterName)
	binary.LittleEndian.PutUint32(b[26:], uint32(result))
	return b
}

// buildZcPartyJoinReqAck0x00FD builds the legacy-header form. The struct is
// identical at all packetvers (packets_struct.hpp:5093-5101) — only the header
// constant is gated (< 20070821 → 0x00FD); clif_party_invite_reply sends the
// 30-byte struct unconditionally (clif.cpp:7968-7974). The framer length is
// corrected to 30 in lengths_map_overrides.go (stale packet(0x00fd,27) literal).
func buildZcPartyJoinReqAck0x00FD(characterName string, result int32) []byte {
	b := make([]byte, 30)
	binary.LittleEndian.PutUint16(b[0:], 0x00FD)
	copy(b[2:26], characterName)
	binary.LittleEndian.PutUint32(b[26:], uint32(result))
	return b
}

// TestZcPartyJoinReqAck_0x02C5_Decode verifies the modern form across the
// documented result enum (rAthena src/map/clif.hpp:160-172 e_party_invite_reply).
func TestZcPartyJoinReqAck_0x02C5_Decode(t *testing.T) {
	cases := []struct {
		name string
		nick string
		res  int32
	}{
		{"accepted", "Bob", 2},
		{"rejected", "Mallory", 1},
		{"party full", "Charlie", 3},
		{"already in party", "Dave", 0},
		{"offline", "Eve", 7},
		{"level restriction", "Frank", 11},
	}
	for _, tc := range cases {
		e := decode.ZcPartyJoinReqAck_0x02C5(buildZcPartyJoinReqAck0x02C5(tc.nick, tc.res), 20200401)
		if e.CharacterName != tc.nick {
			t.Errorf("%s: CharacterName: got %q, want %q", tc.name, e.CharacterName, tc.nick)
		}
		if e.Result != tc.res {
			t.Errorf("%s: Result: got %d, want %d", tc.name, e.Result, tc.res)
		}
	}
}

// TestZcPartyJoinReqAck_0x00FD_Decode verifies the legacy-header form: the
// struct is the same 30-byte layout with an int32 result.
func TestZcPartyJoinReqAck_0x00FD_Decode(t *testing.T) {
	cases := []struct {
		name string
		nick string
		res  int32
	}{
		{"accepted", "Alice", 2},
		{"rejected", "Bob", 1},
		{"party full", "Carol", 3},
		{"already in party", "Dan", 0},
		{"int32 result above uint8 range", "Eve", 258}, // 0x0102 — pins int32, not uint8 truncation
	}
	for _, tc := range cases {
		e := decode.ZcPartyJoinReqAck_0x00FD(buildZcPartyJoinReqAck0x00FD(tc.nick, tc.res), 20070820)
		if e.CharacterName != tc.nick {
			t.Errorf("%s: CharacterName: got %q, want %q", tc.name, e.CharacterName, tc.nick)
		}
		if e.Result != tc.res {
			t.Errorf("%s: Result: got %d, want %d", tc.name, e.Result, tc.res)
		}
	}
}

// TestZcPartyJoinReqAck_NulPaddedName verifies NUL-padding in
// characterName is stripped and an all-NUL name decodes as "".
func TestZcPartyJoinReqAck_NulPaddedName(t *testing.T) {
	e := decode.ZcPartyJoinReqAck_0x02C5(buildZcPartyJoinReqAck0x02C5("Hi", 2), 20200401)
	if e.CharacterName != "Hi" {
		t.Errorf("CharacterName: got %q, want %q", e.CharacterName, "Hi")
	}

	e = decode.ZcPartyJoinReqAck_0x02C5(buildZcPartyJoinReqAck0x02C5("", 1), 20200401)
	if e.CharacterName != "" {
		t.Errorf("empty CharacterName: got %q, want \"\"", e.CharacterName)
	}
}

// TestZcPartyJoinReqAck_FullWidthName verifies a name filling all 24 bytes
// with no NUL terminator decodes as the full 24 bytes.
func TestZcPartyJoinReqAck_FullWidthName(t *testing.T) {
	nick := "TwentyFourCharacterName!" // exactly 24 bytes
	if len(nick) != 24 {
		t.Fatalf("test bug: nick is %d bytes, want 24", len(nick))
	}
	e := decode.ZcPartyJoinReqAck_0x02C5(buildZcPartyJoinReqAck0x02C5(nick, 2), 20200401)
	if e.CharacterName != nick {
		t.Errorf("CharacterName: got %q, want %q", e.CharacterName, nick)
	}
}

// TestActionZcPartyJoinReqAck_Exists verifies the semantic action constant
// exists. If ActionZcPartyJoinReqAck is missing, this file will not compile
// and the test suite fails at build time.
func TestActionZcPartyJoinReqAck_Exists(t *testing.T) {
	_ = session.ActionZcPartyJoinReqAck
	if session.ActionZcPartyJoinReqAck == 0 {
		t.Fatal("ActionZcPartyJoinReqAck == 0 (ActionUnknown) — not assigned a real value")
	}
	if session.ActionZcPartyJoinReqAck.String() != "ActionZcPartyJoinReqAck" {
		t.Errorf("String() = %q, want ActionZcPartyJoinReqAck",
			session.ActionZcPartyJoinReqAck.String())
	}
}

func BenchmarkZcPartyJoinReqAck_0x02C5(b *testing.B) {
	data := buildZcPartyJoinReqAck0x02C5("Bob", 2)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = decode.ZcPartyJoinReqAck_0x02C5(data, 20200401)
	}
}

func BenchmarkZcPartyJoinReqAck_0x00FD(b *testing.B) {
	data := buildZcPartyJoinReqAck0x00FD("Alice", 2)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = decode.ZcPartyJoinReqAck_0x00FD(data, 20070820)
	}
}
