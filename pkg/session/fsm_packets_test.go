package session_test

// Phase 1 C→S packet encoder golden tests.
//
// These tests verify byte-level correctness of each generated encode function
// against GCC-verified struct layouts from rAthena source.
//
// Sources:
//
//	CA_LOGIN (0x0064):            common/packets.hpp PACKET_CA_LOGIN = 55 bytes
//	CH_ENTER (0x0065):            synthetic_structs.hpp SYNTH_CH_ENTER_0x0065 = 17 bytes
//	CH_SELECT_CHAR (0x0066):      common/packets.hpp PACKET_CH_SELECT_CHAR = 3 bytes
//	CH_CHARLIST_REQ (0x09A1):     common/packets.hpp PACKET_CH_CHARLIST_REQ = 2 bytes
//	CZ_ENTER2 (0x0436):           synthetic_structs.hpp SYNTH_CZ_ENTER = 19 bytes
//	CZ_NOTIFY_ACTORINIT (0x007D): synthetic_structs.hpp SYNTH_CZ_NOTIFY_ACTORINIT = 2 bytes
//	CZ_REQUEST_TIME (0x007E):     synthetic_structs.hpp SYNTH_CZ_REQUEST_TIME = 6 bytes
//	CZ_REQUEST_TIME2 (0x0360):    synthetic_structs.hpp SYNTH_CZ_REQUEST_TIME2 = 6 bytes

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/encode"
	"github.com/lenaxia/rathena-client/pkg/send"
)

// ── EncodeMasterLogin (0x0064 CA_LOGIN) ──────────────────────────────────────
//
// struct PACKET_CA_LOGIN at PACKETVER=20180307 (GCC-verified):
//
//	int16  packetType   (0x0064)        offset 0, size 2
//	uint32 version      (packetver)     offset 2, size 4
//	char   username[24]                 offset 6, size 24
//	char   password[24]                 offset 30, size 24
//	uint8  clienttype   (always 0)      offset 54, size 1
//	total = 55 bytes

func TestEncodeMasterLogin_FieldLayout(t *testing.T) {
	pkt := encode.EncodeMasterLogin(send.MasterLogin{
		Version:    20180307,
		Username:   "botijo1",
		Password:   "Melon.77",
		Clienttype: 0,
	}, 20180307)
	if len(pkt) != 55 {
		t.Fatalf("len=%d want 55", len(pkt))
	}
	if id := binary.LittleEndian.Uint16(pkt[0:2]); id != 0x0064 {
		t.Errorf("packetType=%#04x want 0x0064", id)
	}
	if ver := binary.LittleEndian.Uint32(pkt[2:6]); ver != 20180307 {
		t.Errorf("version=%d want 20180307", ver)
	}
	user := strings.TrimRight(string(pkt[6:30]), "\x00")
	if user != "botijo1" {
		t.Errorf("username=%q want botijo1", user)
	}
	pass := strings.TrimRight(string(pkt[30:54]), "\x00")
	if pass != "Melon.77" {
		t.Errorf("password=%q want Melon.77", pass)
	}
	if pkt[54] != 0 {
		t.Errorf("clienttype=%d want 0", pkt[54])
	}
}

func TestEncodeMasterLogin_VersionField(t *testing.T) {
	cases := []uint32{20030000, 20120000, 20170315, 20200401}
	for _, pv := range cases {
		pkt := encode.EncodeMasterLogin(send.MasterLogin{Version: pv, Username: "u", Password: "p"}, pv)
		got := binary.LittleEndian.Uint32(pkt[2:6])
		if got != pv {
			t.Errorf("pv=%d: version field=%d want %d", pv, got, pv)
		}
	}
}

func TestEncodeMasterLogin_UsernameNullPad(t *testing.T) {
	pkt := encode.EncodeMasterLogin(send.MasterLogin{Version: 20180307, Username: "a", Password: "b"}, 20180307)
	for i := 7; i < 30; i++ {
		if pkt[i] != 0 {
			t.Errorf("username[%d]=%#02x want 0x00 (null padding)", i-6, pkt[i])
		}
	}
	for i := 31; i < 54; i++ {
		if pkt[i] != 0 {
			t.Errorf("password[%d]=%#02x want 0x00 (null padding)", i-30, pkt[i])
		}
	}
}

func TestEncodeMasterLogin_UsernameExact24(t *testing.T) {
	user23 := "12345678901234567890123" // 23 chars
	pkt := encode.EncodeMasterLogin(send.MasterLogin{Version: 20180307, Username: user23, Password: "p"}, 20180307)
	got := strings.TrimRight(string(pkt[6:30]), "\x00")
	if got != user23 {
		t.Errorf("username=%q want %q", got, user23)
	}
}

// ── EncodeGameLogin (0x0065 CH_ENTER) ────────────────────────────────────────
//
// SYNTH_CH_ENTER_0x0065 layout from char_clif.cpp:820:
//
//	int16  packetType   (0x0065)   offset 0, size 2
//	uint32 AID                     offset 2, size 4
//	uint32 AuthCode                offset 6, size 4
//	uint32 login_id2               offset 10, size 4
//	uint16 clienttype  (always 0)  offset 14, size 2
//	uint8  sex                     offset 16, size 1
//	total = 17 bytes

func TestEncodeGameLogin_FieldLayout(t *testing.T) {
	const aid = uint32(2000003)
	const sid1 = uint32(0xDEADBEEF)
	const sid2 = uint32(0xCAFEBABE)
	const sex = uint8(1)

	pkt := encode.EncodeGameLogin(send.GameLogin{
		AID: aid, AuthCode: sid1, Login_id2: sid2, Clienttype: 0, Sex: sex,
	}, 20180307)
	if len(pkt) != 17 {
		t.Fatalf("len=%d want 17", len(pkt))
	}
	if id := binary.LittleEndian.Uint16(pkt[0:2]); id != 0x0065 {
		t.Errorf("packetType=%#04x want 0x0065", id)
	}
	if got := binary.LittleEndian.Uint32(pkt[2:6]); got != aid {
		t.Errorf("AID=%d want %d", got, aid)
	}
	if got := binary.LittleEndian.Uint32(pkt[6:10]); got != sid1 {
		t.Errorf("AuthCode=%#x want %#x", got, sid1)
	}
	if got := binary.LittleEndian.Uint32(pkt[10:14]); got != sid2 {
		t.Errorf("login_id2=%#x want %#x", got, sid2)
	}
	if got := binary.LittleEndian.Uint16(pkt[14:16]); got != 0 {
		t.Errorf("clienttype=%d want 0", got)
	}
	if pkt[16] != sex {
		t.Errorf("sex=%d want %d", pkt[16], sex)
	}
}

func TestEncodeGameLogin_FemaleChar(t *testing.T) {
	pkt := encode.EncodeGameLogin(send.GameLogin{AID: 1, AuthCode: 2, Login_id2: 3, Sex: 0}, 20180307)
	if pkt[16] != 0 {
		t.Errorf("sex=%d want 0 (female)", pkt[16])
	}
}

// ── EncodeCharLogin (0x0066 CH_SELECT_CHAR) ───────────────────────────────────
//
// struct PACKET_CH_SELECT_CHAR (GCC-verified):
//
//	int16 packetType  (0x0066)   offset 0, size 2
//	uint8 slot                   offset 2, size 1
//	total = 3 bytes

func TestEncodeCharLogin_Slot0(t *testing.T) {
	pkt := encode.EncodeCharLogin(send.CharLogin{Slot: 0}, 20180307)
	if len(pkt) != 3 {
		t.Fatalf("len=%d want 3", len(pkt))
	}
	if id := binary.LittleEndian.Uint16(pkt[0:2]); id != 0x0066 {
		t.Errorf("packetType=%#04x want 0x0066", id)
	}
	if pkt[2] != 0 {
		t.Errorf("slot=%d want 0", pkt[2])
	}
}

func TestEncodeCharLogin_AllSlots(t *testing.T) {
	for slot := uint8(0); slot <= 8; slot++ {
		pkt := encode.EncodeCharLogin(send.CharLogin{Slot: slot}, 20180307)
		if pkt[2] != slot {
			t.Errorf("slot=%d: pkt[2]=%d want %d", slot, pkt[2], slot)
		}
	}
}

// ── EncodeRequestCharacterPage (0x09A1 CH_CHARLIST_REQ) ───────────────────────
//
// struct PACKET_CH_CHARLIST_REQ (GCC-verified):
//
//	int16 packetType  (0x09A1)   offset 0, size 2
//	total = 2 bytes

func TestEncodeRequestCharacterPage_FieldLayout(t *testing.T) {
	pkt := encode.EncodeRequestCharacterPage(send.RequestCharacterPage{}, 20180307)
	if len(pkt) != 2 {
		t.Fatalf("len=%d want 2", len(pkt))
	}
	if id := binary.LittleEndian.Uint16(pkt[0:2]); id != 0x09A1 {
		t.Errorf("packetType=%#04x want 0x09A1", id)
	}
}

// ── EncodeMapLogin (0x0436 CZ_ENTER2) ────────────────────────────────────────
//
// SYNTH_CZ_ENTER layout from clif.cpp:10641:
//
//	int16  packetType   (0x0436)   offset 0,  size 2
//	uint32 AID                     offset 2,  size 4
//	uint32 GID                     offset 6,  size 4
//	int32  AuthCode                offset 10, size 4
//	uint32 clientTime  (0)         offset 14, size 4
//	uint8  Sex                     offset 18, size 1
//	total = 19 bytes

func TestEncodeMapLogin_FieldLayout(t *testing.T) {
	const aid = uint32(2000003)
	const cid = uint32(150001)
	// 0xDEADBEEF as int32 is -559038737; use a variable to avoid typed-constant overflow.
	sid1 := int32(-559038737) // == int32(0xDEADBEEF)
	const sex = uint8(1)

	pkt := encode.EncodeMapLogin(send.MapLogin{
		AID: aid, GID: cid, AuthCode: sid1, ClientTime: 0, Sex: sex,
	}, 20180307)
	if len(pkt) != 19 {
		t.Fatalf("len=%d want 19", len(pkt))
	}
	if id := binary.LittleEndian.Uint16(pkt[0:2]); id != 0x0436 {
		t.Errorf("packetType=%#04x want 0x0436", id)
	}
	if got := binary.LittleEndian.Uint32(pkt[2:6]); got != aid {
		t.Errorf("AID=%d want %d", got, aid)
	}
	if got := binary.LittleEndian.Uint32(pkt[6:10]); got != cid {
		t.Errorf("GID=%d want %d", got, cid)
	}
	if got := int32(binary.LittleEndian.Uint32(pkt[10:14])); got != sid1 {
		t.Errorf("AuthCode=%#x want %#x", got, sid1)
	}
	if got := binary.LittleEndian.Uint32(pkt[14:18]); got != 0 {
		t.Errorf("clientTime=%d want 0", got)
	}
	if pkt[18] != sex {
		t.Errorf("Sex=%d want %d", pkt[18], sex)
	}
}

func TestEncodeMapLogin_ZeroCharID(t *testing.T) {
	pkt := encode.EncodeMapLogin(send.MapLogin{AID: 1001, GID: 0, AuthCode: int32(0xABCD), Sex: 0}, 20180307)
	if got := binary.LittleEndian.Uint32(pkt[6:10]); got != 0 {
		t.Errorf("GID=%d want 0", got)
	}
	if got := binary.LittleEndian.Uint32(pkt[10:14]); got != 0xABCD {
		t.Errorf("AuthCode=%#x want 0xABCD", got)
	}
}

// ── EncodeMapLoaded (0x007D CZ_NOTIFY_ACTORINIT) ──────────────────────────────
//
// SYNTH_CZ_NOTIFY_ACTORINIT: int16 only = 2 bytes
// Source: clif.cpp:10742, parseable_packet(0x007d, 2, ...)

func TestEncodeMapLoaded_FieldLayout(t *testing.T) {
	pkt := encode.EncodeMapLoaded(send.MapLoaded{}, 20180307)
	if len(pkt) != 2 {
		t.Fatalf("len=%d want 2", len(pkt))
	}
	if id := binary.LittleEndian.Uint16(pkt[0:2]); id != 0x007D {
		t.Errorf("packetType=%#04x want 0x007D", id)
	}
}

// ── EncodeTimeSyncResponse (0x007E / 0x0360) ──────────────────────────────────
//
// CZ_REQUEST_TIME  (0x007E): int16 + uint32 = 6 bytes (packetver < 20101124)
// CZ_REQUEST_TIME2 (0x0360): int16 + uint32 = 6 bytes (packetver >= 20101124)
// Source: clif.cpp:11196-11197, parseable_packet(0x007e/0x0360, 6, ...)
//
// NOTE: The packetver boundary changed from the old FSM (20080102) to the correct
// semantics DB boundary (20101124). The old FSM used 0x0360 starting at 20080102;
// rAthena's shuffle tables confirm 0x0360 as CZ_REQUEST_TIME2 starting at 20101124.

func TestEncodeTimeSyncResponse_Pre20101124(t *testing.T) {
	pkt := encode.EncodeTimeSyncResponse(send.TimeSyncResponse{ClientTime: 0}, 20030000)
	if len(pkt) != 6 {
		t.Fatalf("len=%d want 6", len(pkt))
	}
	if id := binary.LittleEndian.Uint16(pkt[0:2]); id != 0x007E {
		t.Errorf("packetType=%#04x want 0x007E (pre-20101124)", id)
	}
	if tick := binary.LittleEndian.Uint32(pkt[2:6]); tick != 0 {
		t.Errorf("clientTime=%d want 0", tick)
	}
}

func TestEncodeTimeSyncResponse_Post20101124(t *testing.T) {
	pkt := encode.EncodeTimeSyncResponse(send.TimeSyncResponse{ClientTime: 0}, 20101124)
	if len(pkt) != 6 {
		t.Fatalf("len=%d want 6", len(pkt))
	}
	if id := binary.LittleEndian.Uint16(pkt[0:2]); id != 0x0360 {
		t.Errorf("packetType=%#04x want 0x0360 (post-20101124)", id)
	}
}

func TestEncodeTimeSyncResponse_BoundaryExact(t *testing.T) {
	// Boundary: 20101123 → 0x007E; 20101124 → 0x0360
	pre := encode.EncodeTimeSyncResponse(send.TimeSyncResponse{}, 20101123)
	if id := binary.LittleEndian.Uint16(pre[0:2]); id != 0x007E {
		t.Errorf("pv=20101123: id=%#04x want 0x007E", id)
	}
	post := encode.EncodeTimeSyncResponse(send.TimeSyncResponse{}, 20101124)
	if id := binary.LittleEndian.Uint16(post[0:2]); id != 0x0360 {
		t.Errorf("pv=20101124: id=%#04x want 0x0360", id)
	}
}

func TestEncodeTimeSyncResponse_Modern(t *testing.T) {
	pkt := encode.EncodeTimeSyncResponse(send.TimeSyncResponse{}, 20200401)
	if id := binary.LittleEndian.Uint16(pkt[0:2]); id != 0x0360 {
		t.Errorf("pv=20200401: id=%#04x want 0x0360", id)
	}
	if len(pkt) != 6 {
		t.Fatalf("len=%d want 6", len(pkt))
	}
}
