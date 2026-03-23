// Manually implemented — regression tests for missing ActionBattleChat,
// ActionPartyChat, ActionSetWhisperState (same gap class as whisper/worklog 0076).

package encode_test

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/encode"
	"github.com/lenaxia/rathena-client/pkg/send"
	"github.com/lenaxia/rathena-client/pkg/session"
)

// ── BattleChat ────────────────────────────────────────────────────────────────

// TestEncodeBattleChat_WireFormat verifies CZ_BATTLEFIELD_CHAT (0x02DB).
// Source: clif_packetdb.hpp:921 parseable_packet(0x02db,-1,clif_parse_BattleChat,2,4)
// Wire: [packetType:2][packetLength:2]["Name : Message\0"]
func TestEncodeBattleChat_WireFormat(t *testing.T) {
	req := send.BattleChat{Name: "Warrior", Message: "Charge!"}
	p := encode.EncodeBattleChat(req, 20200401)

	if p[0] != 0xDB || p[1] != 0x02 {
		t.Errorf("packet ID: got [%02X %02X], want [DB 02]", p[0], p[1])
	}
	text := "Warrior : Charge!\x00"
	wantLen := 4 + len(text)
	if gotLen := int(binary.LittleEndian.Uint16(p[2:4])); gotLen != wantLen {
		t.Errorf("length field: got %d, want %d", gotLen, wantLen)
	}
	if string(p[4:]) != text {
		t.Errorf("body: got %q, want %q", string(p[4:]), text)
	}
}

func TestActionBattleChat_Registered(t *testing.T) {
	_ = session.ActionBattleChat
	if session.ActionBattleChat == 0 {
		t.Fatal("ActionBattleChat == 0 (ActionUnknown)")
	}
	if session.ActionBattleChat.String() != "ActionBattleChat" {
		t.Errorf("String() = %q, want ActionBattleChat", session.ActionBattleChat.String())
	}
}

func BenchmarkEncodeBattleChat(b *testing.B) {
	req := send.BattleChat{Name: "Warrior", Message: "Charge!"}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodeBattleChat(req, 20200401)
	}
}

// ── PartyChat ─────────────────────────────────────────────────────────────────

// TestEncodePartyChat_WireFormat verifies CZ_PARTY_MESSAGE (0x0108).
// Source: clif_packetdb.hpp:108 parseable_packet(0x0108,-1,clif_parse_PartyMessage,2,4)
// Wire: [packetType:2][packetLength:2]["Name : Message\0"]
func TestEncodePartyChat_WireFormat(t *testing.T) {
	req := send.PartyChat{Name: "Tank", Message: "Pull now"}
	p := encode.EncodePartyChat(req, 20200401)

	if p[0] != 0x08 || p[1] != 0x01 {
		t.Errorf("packet ID: got [%02X %02X], want [08 01]", p[0], p[1])
	}
	text := "Tank : Pull now\x00"
	wantLen := 4 + len(text)
	if gotLen := int(binary.LittleEndian.Uint16(p[2:4])); gotLen != wantLen {
		t.Errorf("length field: got %d, want %d", gotLen, wantLen)
	}
	if string(p[4:]) != text {
		t.Errorf("body: got %q, want %q", string(p[4:]), text)
	}
}

func TestActionPartyChat_Registered(t *testing.T) {
	_ = session.ActionPartyChat
	if session.ActionPartyChat == 0 {
		t.Fatal("ActionPartyChat == 0 (ActionUnknown)")
	}
	if session.ActionPartyChat.String() != "ActionPartyChat" {
		t.Errorf("String() = %q, want ActionPartyChat", session.ActionPartyChat.String())
	}
}

func BenchmarkEncodePartyChat(b *testing.B) {
	req := send.PartyChat{Name: "Tank", Message: "Pull now"}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodePartyChat(req, 20200401)
	}
}

// ── SetWhisperState ───────────────────────────────────────────────────────────

// TestEncodeSetWhisperState_WireFormat verifies CZ_SETTING_WHISPER_PC (0x00CF).
// Source: clif_packetdb.hpp:78 parseable_packet(0x00cf,27,clif_parse_PMIgnore,2,26)
// Wire: [packetType:2][nick:24][type:1] = 27 bytes fixed.
func TestEncodeSetWhisperState_WireFormat(t *testing.T) {
	req := send.SetWhisperState{Nick: "Scammer", Type: 1}
	p := encode.EncodeSetWhisperState(req, 20200401)

	if len(p) != 27 {
		t.Fatalf("length: got %d, want 27", len(p))
	}
	if p[0] != 0xCF || p[1] != 0x00 {
		t.Errorf("packet ID: got [%02X %02X], want [CF 00]", p[0], p[1])
	}
	// Nick at [2:26], NUL-padded to 24 bytes
	if string(p[2:9]) != "Scammer" {
		t.Errorf("nick: got %q, want %q", string(p[2:9]), "Scammer")
	}
	for i := 9; i < 26; i++ {
		if p[i] != 0x00 {
			t.Errorf("nick padding byte[%d]: got 0x%02X, want 0x00", i, p[i])
		}
	}
	// Type at offset 26
	if p[26] != 1 {
		t.Errorf("type: got %d, want 1", p[26])
	}
}

func TestActionSetWhisperState_Registered(t *testing.T) {
	_ = session.ActionSetWhisperState
	if session.ActionSetWhisperState == 0 {
		t.Fatal("ActionSetWhisperState == 0 (ActionUnknown)")
	}
	if session.ActionSetWhisperState.String() != "ActionSetWhisperState" {
		t.Errorf("String() = %q, want ActionSetWhisperState", session.ActionSetWhisperState.String())
	}
}

func BenchmarkEncodeSetWhisperState(b *testing.B) {
	req := send.SetWhisperState{Nick: "Scammer", Type: 1}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encode.EncodeSetWhisperState(req, 20200401)
	}
}
