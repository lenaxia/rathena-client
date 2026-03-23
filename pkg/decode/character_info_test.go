// Hand-written tests for CHARACTER_INFO decoder.
// Golden bytes synthesised from GCC-preprocessed struct layouts at each PACKETVER
// breakpoint (verified via validation/preprocess_check.sh).
//
// Representative breakpoints tested:
//
//	B0 pv=20030000 : 112 bytes — baseline, no optional fields
//	B2 pv=20100803 : 132 bytes — +mapName[16]+DelRevDate
//	B5 pv=20111025 : 144 bytes — +robePalette+chr_slot_changeCnt+chr_name_changeCnt
//	B7 pv=20141022 : 147 bytes — +body(after head)+sex
//	B8 pv=20170830 : 155 bytes — exp/jobexp int32→int64
//	B8 real        : pv=20200401 — 4 real captured characters from DUMP17_login_4chars
//	B9 pv=20220330 : 175 bytes — hp/maxhp/sp/maxsp int32/int16→int64
//
// Source: rAthena src/common/packets.hpp:31–105
package decode

import (
	"encoding/binary"
	"testing"

	"github.com/lenaxia/rathena-client/pkg/events"
)

// buildCharInfoB0 builds a 112-byte CHARACTER_INFO entry for pv < 20100720.
// All PACKETVER-conditional fields absent; name fixed at 24 bytes.
func buildCharInfoB0(gid uint32, exp, money, jobexp int32, job, level int16, name string, charNum uint8) []byte {
	b := make([]byte, 112)
	binary.LittleEndian.PutUint32(b[0:], gid)
	binary.LittleEndian.PutUint32(b[4:], uint32(exp))
	binary.LittleEndian.PutUint32(b[8:], uint32(money))
	binary.LittleEndian.PutUint32(b[12:], uint32(jobexp))
	// joblevel=1 bodystate=0 healthstate=0 effectstate=0 virtue=0 honor=0 → zeros
	// jobpoint offset=40
	// hp offset=42
	binary.LittleEndian.PutUint32(b[42:], 500) // hp
	binary.LittleEndian.PutUint32(b[46:], 500) // maxhp
	binary.LittleEndian.PutUint16(b[50:], 100) // sp
	binary.LittleEndian.PutUint16(b[52:], 100) // maxsp
	binary.LittleEndian.PutUint16(b[54:], 150) // speed
	binary.LittleEndian.PutUint16(b[56:], uint16(job))
	// head offset=58 weapon offset=60
	binary.LittleEndian.PutUint16(b[62:], uint16(level))
	// name offset=78
	copy(b[78:102], []byte(name))
	b[108] = charNum // CharNum
	return b
}

// buildCharInfoB2 builds a 132-byte CHARACTER_INFO entry for pv >= 20100803.
// Adds mapName[16] and DelRevDate after bIsChangedCharName.
func buildCharInfoB2(gid uint32, job, level int16, name, mapName string, charNum uint8) []byte {
	b := make([]byte, 132)
	binary.LittleEndian.PutUint32(b[0:], gid)
	binary.LittleEndian.PutUint32(b[42:], 800)  // hp
	binary.LittleEndian.PutUint32(b[46:], 1000) // maxhp
	binary.LittleEndian.PutUint16(b[50:], 200)  // sp
	binary.LittleEndian.PutUint16(b[52:], 300)  // maxsp
	binary.LittleEndian.PutUint16(b[54:], 150)  // speed
	binary.LittleEndian.PutUint16(b[56:], uint16(job))
	binary.LittleEndian.PutUint16(b[62:], uint16(level))
	copy(b[78:102], []byte(name))
	b[108] = charNum
	copy(b[112:128], []byte(mapName))         // mapName offset=112
	binary.LittleEndian.PutUint32(b[128:], 0) // DelRevDate offset=128
	return b
}

// buildCharInfoB5 builds a 144-byte CHARACTER_INFO entry for pv >= 20111025.
func buildCharInfoB5(gid uint32, job, level int16, name, mapName string, charNum uint8) []byte {
	b := make([]byte, 144)
	binary.LittleEndian.PutUint32(b[0:], gid)
	binary.LittleEndian.PutUint32(b[42:], 1200)
	binary.LittleEndian.PutUint32(b[46:], 1500)
	binary.LittleEndian.PutUint16(b[50:], 300)
	binary.LittleEndian.PutUint16(b[52:], 400)
	binary.LittleEndian.PutUint16(b[54:], 150)
	binary.LittleEndian.PutUint16(b[56:], uint16(job))
	binary.LittleEndian.PutUint16(b[62:], uint16(level))
	copy(b[78:102], []byte(name))
	b[108] = charNum
	copy(b[112:128], []byte(mapName))
	// robePalette offset=132, chr_slot_changeCnt=136, chr_name_changeCnt=140
	binary.LittleEndian.PutUint32(b[132:], 7)
	binary.LittleEndian.PutUint32(b[136:], 2)
	binary.LittleEndian.PutUint32(b[140:], 1)
	return b
}

// buildCharInfoB7 builds a 147-byte CHARACTER_INFO entry for pv >= 20141022.
// body(int16) inserted after head; sex appended.
// Field offsets shift by 2 after head compared to B5.
func buildCharInfoB7(gid uint32, job, level int16, name, mapName string, charNum, sex uint8) []byte {
	b := make([]byte, 147)
	binary.LittleEndian.PutUint32(b[0:], gid)
	// hp offset=42 maxhp=46 sp=50 maxsp=52 speed=54 job=56 head=58
	// body=60 weapon=62 level=64
	binary.LittleEndian.PutUint32(b[42:], 2000)
	binary.LittleEndian.PutUint32(b[46:], 2500)
	binary.LittleEndian.PutUint16(b[50:], 400)
	binary.LittleEndian.PutUint16(b[52:], 500)
	binary.LittleEndian.PutUint16(b[54:], 150)
	binary.LittleEndian.PutUint16(b[56:], uint16(job))
	binary.LittleEndian.PutUint16(b[58:], 12) // head
	binary.LittleEndian.PutUint16(b[60:], 5)  // body
	binary.LittleEndian.PutUint16(b[64:], uint16(level))
	// name offset=80
	copy(b[80:104], []byte(name))
	b[110] = charNum                  // CharNum offset=110
	copy(b[114:130], []byte(mapName)) // mapName offset=114
	// robePalette=134 chr_slot=138 chr_name=142
	binary.LittleEndian.PutUint32(b[134:], 3)
	b[146] = sex // sex offset=146
	return b
}

// buildCharInfoB8 builds a 155-byte CHARACTER_INFO entry for pv >= 20170830.
// exp/jobexp widened to int64; body present; sex appended.
func buildCharInfoB8(gid uint32, exp, jobexp int64, job, level int16, name, mapName string, charNum, sex uint8) []byte {
	b := make([]byte, 155)
	binary.LittleEndian.PutUint32(b[0:], gid)
	binary.LittleEndian.PutUint64(b[4:], uint64(exp))
	// money offset=12
	binary.LittleEndian.PutUint64(b[16:], uint64(jobexp))
	// joblevel offset=24 ... jobpoint=48
	// hp=50 maxhp=54 sp=58 maxsp=60 speed=62 job=64 head=66 body=68 weapon=70 level=72
	binary.LittleEndian.PutUint32(b[50:], 86115)
	binary.LittleEndian.PutUint32(b[54:], 86115)
	binary.LittleEndian.PutUint16(b[58:], 15264)
	binary.LittleEndian.PutUint16(b[60:], 15264)
	binary.LittleEndian.PutUint16(b[62:], 150)
	binary.LittleEndian.PutUint16(b[64:], uint16(job))
	binary.LittleEndian.PutUint16(b[72:], uint16(level))
	copy(b[88:112], []byte(name))
	b[118] = charNum
	copy(b[122:138], []byte(mapName))
	b[154] = sex
	return b
}

// buildCharInfoB9 builds a 175-byte CHARACTER_INFO entry for pv >= 20220330 (MAIN).
// hp/maxhp/sp/maxsp widened to int64 on top of B8.
func buildCharInfoB9(gid uint32, exp, jobexp int64, hp, maxhp, sp, maxsp int64, job, level int16, name, mapName string, charNum, sex uint8) []byte {
	b := make([]byte, 175)
	binary.LittleEndian.PutUint32(b[0:], gid)
	binary.LittleEndian.PutUint64(b[4:], uint64(exp))
	binary.LittleEndian.PutUint64(b[16:], uint64(jobexp))
	// hp=50 maxhp=58 sp=66 maxsp=74 speed=82 job=84 head=86 body=88 weapon=90 level=92
	binary.LittleEndian.PutUint64(b[50:], uint64(hp))
	binary.LittleEndian.PutUint64(b[58:], uint64(maxhp))
	binary.LittleEndian.PutUint64(b[66:], uint64(sp))
	binary.LittleEndian.PutUint64(b[74:], uint64(maxsp))
	binary.LittleEndian.PutUint16(b[82:], 150)
	binary.LittleEndian.PutUint16(b[84:], uint16(job))
	binary.LittleEndian.PutUint16(b[92:], uint16(level))
	copy(b[108:132], []byte(name))
	b[138] = charNum
	copy(b[142:158], []byte(mapName))
	b[174] = sex
	return b
}

// TestDecodeCharacterInfoEntry_B0_Baseline verifies pv < 20100720 (112 bytes):
// no mapName, no body, exp/jobexp are int32.
func TestDecodeCharacterInfoEntry_B0_Baseline(t *testing.T) {
	pv := uint32(20030000)
	raw := buildCharInfoB0(12345, 9999, 50000, 1234, 0, 50, "Novice", 0)

	var got events.CharacterInfoEntry
	n := decodeCharacterInfoEntry(&got, raw, pv)

	if n != 112 {
		t.Fatalf("expected 112 bytes consumed, got %d", n)
	}
	if got.GID != 12345 {
		t.Errorf("GID: want 12345, got %d", got.GID)
	}
	if got.Exp != 9999 {
		t.Errorf("Exp: want 9999, got %d", got.Exp)
	}
	if got.JobExp != 1234 {
		t.Errorf("JobExp: want 1234, got %d", got.JobExp)
	}
	if got.Job != 0 {
		t.Errorf("Job: want 0, got %d", got.Job)
	}
	if got.Level != 50 {
		t.Errorf("Level: want 50, got %d", got.Level)
	}
	if got.Name != "Novice" {
		t.Errorf("Name: want %q, got %q", "Novice", got.Name)
	}
	if got.CharNum != 0 {
		t.Errorf("CharNum: want 0, got %d", got.CharNum)
	}
	if got.HP != 500 {
		t.Errorf("HP: want 500, got %d", got.HP)
	}
	if got.MaxHP != 500 {
		t.Errorf("MaxHP: want 500, got %d", got.MaxHP)
	}
	if got.SP != 100 {
		t.Errorf("SP: want 100, got %d", got.SP)
	}
	if got.MaxSP != 100 {
		t.Errorf("MaxSP: want 100, got %d", got.MaxSP)
	}
	// No mapName in B0
	if got.MapName != "" {
		t.Errorf("MapName: want empty, got %q", got.MapName)
	}
	// No sex in B0
	if got.Sex != 0 {
		t.Errorf("Sex: want 0, got %d", got.Sex)
	}
}

// TestDecodeCharacterInfoEntry_B2_MapName verifies pv=20100803 (132 bytes):
// mapName[16] and DelRevDate added after bIsChangedCharName.
func TestDecodeCharacterInfoEntry_B2_MapName(t *testing.T) {
	pv := uint32(20100803)
	raw := buildCharInfoB2(99, 1, 10, "Swordsman", "prontera.gat", 1)

	var got events.CharacterInfoEntry
	n := decodeCharacterInfoEntry(&got, raw, pv)

	if n != 132 {
		t.Fatalf("expected 132 bytes consumed, got %d", n)
	}
	if got.GID != 99 {
		t.Errorf("GID: want 99, got %d", got.GID)
	}
	if got.Name != "Swordsman" {
		t.Errorf("Name: want %q, got %q", "Swordsman", got.Name)
	}
	if got.MapName != "prontera.gat" {
		t.Errorf("MapName: want %q, got %q", "prontera.gat", got.MapName)
	}
	if got.CharNum != 1 {
		t.Errorf("CharNum: want 1, got %d", got.CharNum)
	}
	if got.Job != 1 {
		t.Errorf("Job: want 1, got %d", got.Job)
	}
	if got.Level != 10 {
		t.Errorf("Level: want 10, got %d", got.Level)
	}
	// sex not present in B2
	if got.Sex != 0 {
		t.Errorf("Sex: want 0, got %d", got.Sex)
	}
}

// TestDecodeCharacterInfoEntry_B5_RobePalette verifies pv=20111025 (144 bytes):
// robePalette, chr_slot_changeCnt, chr_name_changeCnt present. Still no body/sex.
func TestDecodeCharacterInfoEntry_B5_RobePalette(t *testing.T) {
	pv := uint32(20111025)
	raw := buildCharInfoB5(777, 4, 99, "Archer", "pay_fild04.gat", 2)

	var got events.CharacterInfoEntry
	n := decodeCharacterInfoEntry(&got, raw, pv)

	if n != 144 {
		t.Fatalf("expected 144 bytes consumed, got %d", n)
	}
	if got.GID != 777 {
		t.Errorf("GID: want 777, got %d", got.GID)
	}
	if got.Job != 4 {
		t.Errorf("Job: want 4, got %d", got.Job)
	}
	if got.Level != 99 {
		t.Errorf("Level: want 99, got %d", got.Level)
	}
	if got.Name != "Archer" {
		t.Errorf("Name: want %q, got %q", "Archer", got.Name)
	}
	if got.MapName != "pay_fild04.gat" {
		t.Errorf("MapName: want %q, got %q", "pay_fild04.gat", got.MapName)
	}
	if got.CharNum != 2 {
		t.Errorf("CharNum: want 2, got %d", got.CharNum)
	}
	if got.Sex != 0 {
		t.Errorf("Sex: want 0 (not present at B5), got %d", got.Sex)
	}
}

// TestDecodeCharacterInfoEntry_B7_BodyAndSex verifies pv=20141022 (147 bytes):
// body(int16) inserted after head — shifts all subsequent offsets by 2.
// sex appended at end.
func TestDecodeCharacterInfoEntry_B7_BodyAndSex(t *testing.T) {
	pv := uint32(20141022)
	raw := buildCharInfoB7(55555, 7, 150, "Wizard", "gef_fild01.gat", 0, 1)

	var got events.CharacterInfoEntry
	n := decodeCharacterInfoEntry(&got, raw, pv)

	if n != 147 {
		t.Fatalf("expected 147 bytes consumed, got %d", n)
	}
	if got.GID != 55555 {
		t.Errorf("GID: want 55555, got %d", got.GID)
	}
	if got.Job != 7 {
		t.Errorf("Job: want 7, got %d", got.Job)
	}
	if got.Level != 150 {
		t.Errorf("Level: want 150, got %d", got.Level)
	}
	if got.Name != "Wizard" {
		t.Errorf("Name: want %q, got %q", "Wizard", got.Name)
	}
	if got.MapName != "gef_fild01.gat" {
		t.Errorf("MapName: want %q, got %q", "gef_fild01.gat", got.MapName)
	}
	if got.CharNum != 0 {
		t.Errorf("CharNum: want 0, got %d", got.CharNum)
	}
	if got.Sex != 1 {
		t.Errorf("Sex: want 1, got %d", got.Sex)
	}
}

// TestDecodeCharacterInfoEntry_B8_ExpWidened verifies pv=20170830 (155 bytes):
// exp and jobexp widened from int32 to int64.
// Values exceed int32 max to confirm widening is correct.
func TestDecodeCharacterInfoEntry_B8_ExpWidened(t *testing.T) {
	pv := uint32(20170830)
	bigExp := int64(3_000_000_000)    // > int32 max (2,147,483,647)
	bigJobExp := int64(1_500_000_000) // just under int32 max for contrast
	raw := buildCharInfoB8(200001, bigExp, bigJobExp, 4054, 175, "HighPriest", "abbey02.gat", 1, 0)

	var got events.CharacterInfoEntry
	n := decodeCharacterInfoEntry(&got, raw, pv)

	if n != 155 {
		t.Fatalf("expected 155 bytes consumed, got %d", n)
	}
	if got.GID != 200001 {
		t.Errorf("GID: want 200001, got %d", got.GID)
	}
	if got.Exp != bigExp {
		t.Errorf("Exp: want %d, got %d", bigExp, got.Exp)
	}
	if got.JobExp != bigJobExp {
		t.Errorf("JobExp: want %d, got %d", bigJobExp, got.JobExp)
	}
	if got.Job != 4054 {
		t.Errorf("Job: want 4054, got %d", got.Job)
	}
	if got.Level != 175 {
		t.Errorf("Level: want 175, got %d", got.Level)
	}
	if got.Name != "HighPriest" {
		t.Errorf("Name: want %q, got %q", "HighPriest", got.Name)
	}
	if got.MapName != "abbey02.gat" {
		t.Errorf("MapName: want %q, got %q", "abbey02.gat", got.MapName)
	}
	if got.CharNum != 1 {
		t.Errorf("CharNum: want 1, got %d", got.CharNum)
	}
	if got.Sex != 0 {
		t.Errorf("Sex: want 0, got %d", got.Sex)
	}
	if got.HP != 86115 {
		t.Errorf("HP: want 86115, got %d", got.HP)
	}
	if got.MaxHP != 86115 {
		t.Errorf("MaxHP: want 86115, got %d", got.MaxHP)
	}
}

// TestDecodeCharacterInfoEntry_B8_RealCapture validates against real captured
// bytes from DUMP17_login_4chars (pv=20200401, B8 layout, 155 bytes/entry).
// Four characters: Almarc(slot0), Chrno Crusade(slot1), Beyond Faith(slot2), Eclair(slot3).
func TestDecodeCharacterInfoEntry_B8_RealCapture(t *testing.T) {
	pv := uint32(20200401)

	cases := []struct {
		raw     string // hex from dump
		gid     uint32
		job     int16
		level   int16
		name    string
		charNum uint8
		sex     uint8
		mapName string
		exp     int64
		hp      int64
		maxHP   int64
	}{
		{
			raw:     "f14902008f4b02000000000040420f000e3d0000000000003a00000000000000000000000000000000000000000000009a026350010063500100a03ba03b9600df0f0c00000008007c009800000000003a01000003000200416c6d617263000000000000000000000000000000000000ffffffffffff000301007072745f66696c6430382e6761740000000000000000000000000000000000011e",
			gid:     150001,
			job:     0x0fdf, // 4063
			level:   124,
			name:    "Almarc",
			charNum: 0,
			sex:     0x1e, // 30 — server-side value from rAthena (not 0/1 client sex)
			mapName: "prt_fild08.gat",
			exp:     150415,
			hp:      86115,
			maxHP:   86115,
		},
		{
			raw:     "4b0200000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000300028000000280000000b000b0096000000020000000100010000000000000000000000000000004368726e6f204372757361646500000000000000000000000101010101010100010070726f6e746572612e676174000000000000000000000000000000000000000001",
			gid:     587,
			job:     0,
			level:   1,
			name:    "Chrno Crusade",
			charNum: 1,
			sex:     1,
			mapName: "prontera.gat",
			exp:     0,
			hp:      40,
			maxHP:   40,
		},
		{
			raw:     "1f4b02001200000000000000000000000e00000000000000460000000000000000000000000000000000000000000000f10f8a4300008a430000730873089600d90f020000000000c8001d010000000000000000000000004265796f6e642046616974680000000000000000000000000a0101010101020001007072745f73657762322e6761740000000000000000000000000000000000000000",
			gid:     150303,
			job:     0x0fd9, // 4057
			level:   200,
			name:    "Beyond Faith",
			charNum: 2,
			sex:     0,
			mapName: "prt_sewb2.gat",
			exp:     18,
			hp:      17290,
			maxHP:   17290,
		},
		{
			raw:     "204b0200c00d0500000000000000000020cc0400000000004600000000000000000000000000000000000000000000003710564d0300564d0300426842689600dd0f030000000100c800650000000000000000000000000045636c616972000000000000000000000000000000000000ffffffffffff030001007072745f696e2e6761740000000000000000000000000000000000000000000001",
			gid:     150304,
			job:     0x0fdd, // 4061
			level:   200,
			name:    "Eclair",
			charNum: 3,
			sex:     1,
			mapName: "prt_in.gat",
			exp:     331200,
			hp:      216406,
			maxHP:   216406,
		},
	}

	for _, tc := range cases {
		raw := hexToBytes(t, tc.raw)
		if len(raw) != 155 {
			t.Fatalf("case %q: raw length %d, expected 155", tc.name, len(raw))
		}

		var got events.CharacterInfoEntry
		n := decodeCharacterInfoEntry(&got, raw, pv)

		if n != 155 {
			t.Errorf("%q: consumed %d bytes, want 155", tc.name, n)
		}
		if got.GID != tc.gid {
			t.Errorf("%q: GID want %d got %d", tc.name, tc.gid, got.GID)
		}
		if got.Job != tc.job {
			t.Errorf("%q: Job want %d got %d", tc.name, tc.job, got.Job)
		}
		if got.Level != tc.level {
			t.Errorf("%q: Level want %d got %d", tc.name, tc.level, got.Level)
		}
		if got.Name != tc.name {
			t.Errorf("%q: Name want %q got %q", tc.name, tc.name, got.Name)
		}
		if got.CharNum != tc.charNum {
			t.Errorf("%q: CharNum want %d got %d", tc.name, tc.charNum, got.CharNum)
		}
		if got.Sex != tc.sex {
			t.Errorf("%q: Sex want %d got %d", tc.name, tc.sex, got.Sex)
		}
		if got.MapName != tc.mapName {
			t.Errorf("%q: MapName want %q got %q", tc.name, tc.mapName, got.MapName)
		}
		if got.Exp != tc.exp {
			t.Errorf("%q: Exp want %d got %d", tc.name, tc.exp, got.Exp)
		}
		if got.HP != tc.hp {
			t.Errorf("%q: HP want %d got %d", tc.name, tc.hp, got.HP)
		}
		if got.MaxHP != tc.maxHP {
			t.Errorf("%q: MaxHP want %d got %d", tc.name, tc.maxHP, got.MaxHP)
		}
	}
}

// TestDecodeCharacterInfoEntry_B9_HPWidened verifies pv=20220330 (175 bytes):
// hp/maxhp widened int32→int64, sp/maxsp widened int16→int64.
// Values exceed their old widths to confirm widening.
func TestDecodeCharacterInfoEntry_B9_HPWidened(t *testing.T) {
	pv := uint32(20220330)
	bigHP := int64(5_000_000_000) // > int32 max
	bigSP := int64(2_000_000_000) // > int16 max (32767)
	raw := buildCharInfoB9(300001, 9_000_000_000, 4_000_000_000, bigHP, bigHP, bigSP, bigSP, 4096, 175, "RuneKnight", "moc_fild16.gat", 0, 0)

	var got events.CharacterInfoEntry
	n := decodeCharacterInfoEntry(&got, raw, pv)

	if n != 175 {
		t.Fatalf("expected 175 bytes consumed, got %d", n)
	}
	if got.GID != 300001 {
		t.Errorf("GID: want 300001, got %d", got.GID)
	}
	if got.HP != bigHP {
		t.Errorf("HP: want %d, got %d", bigHP, got.HP)
	}
	if got.MaxHP != bigHP {
		t.Errorf("MaxHP: want %d, got %d", bigHP, got.MaxHP)
	}
	if got.SP != bigSP {
		t.Errorf("SP: want %d, got %d", bigSP, got.SP)
	}
	if got.MaxSP != bigSP {
		t.Errorf("MaxSP: want %d, got %d", bigSP, got.MaxSP)
	}
	if got.Job != 4096 {
		t.Errorf("Job: want 4096, got %d", got.Job)
	}
	if got.Level != 175 {
		t.Errorf("Level: want 175, got %d", got.Level)
	}
	if got.Name != "RuneKnight" {
		t.Errorf("Name: want %q, got %q", "RuneKnight", got.Name)
	}
	if got.MapName != "moc_fild16.gat" {
		t.Errorf("MapName: want %q, got %q", "moc_fild16.gat", got.MapName)
	}
}

// TestDecodeCharacterInfoList_MultipleEntries verifies that decodeCharacterInfoList
// correctly splits a buffer containing multiple entries.
func TestDecodeCharacterInfoList_MultipleEntries(t *testing.T) {
	pv := uint32(20200401) // B8, 155 bytes/entry

	e0 := buildCharInfoB8(1, 100, 50, 0, 1, "Alpha", "prontera.gat", 0, 0)
	e1 := buildCharInfoB8(2, 200, 75, 1, 20, "Beta", "geffen.gat", 1, 1)
	e2 := buildCharInfoB8(3, 300, 25, 7, 50, "Gamma", "moc_fild01.gat", 2, 0)

	buf := append(append(e0, e1...), e2...)
	entries := decodeCharacterInfoList(buf, pv)

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].GID != 1 || entries[0].Name != "Alpha" || entries[0].CharNum != 0 {
		t.Errorf("entry[0]: %+v", entries[0])
	}
	if entries[1].GID != 2 || entries[1].Name != "Beta" || entries[1].CharNum != 1 {
		t.Errorf("entry[1]: %+v", entries[1])
	}
	if entries[2].GID != 3 || entries[2].Name != "Gamma" || entries[2].CharNum != 2 {
		t.Errorf("entry[2]: %+v", entries[2])
	}
}

// TestDecodeCharacterInfoList_Empty verifies empty input returns nil slice.
func TestDecodeCharacterInfoList_Empty(t *testing.T) {
	entries := decodeCharacterInfoList(nil, 20200401)
	if len(entries) != 0 {
		t.Errorf("expected empty, got %d entries", len(entries))
	}
}

// TestDecodeCharacterInfoList_PartialTrailingBytes verifies that trailing bytes
// shorter than one entry are silently ignored.
func TestDecodeCharacterInfoList_PartialTrailingBytes(t *testing.T) {
	pv := uint32(20200401) // 155 bytes/entry
	e0 := buildCharInfoB8(1, 0, 0, 0, 1, "Solo", "prontera.gat", 0, 0)
	// Append 10 garbage bytes — less than one entry
	buf := append(e0, make([]byte, 10)...)

	entries := decodeCharacterInfoList(buf, pv)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Name != "Solo" {
		t.Errorf("Name: want %q, got %q", "Solo", entries[0].Name)
	}
}

// TestDecodeCharacterInfoEntry_NameNullTerminated verifies that a name shorter
// than 24 bytes with null padding decodes correctly without trailing nulls.
func TestDecodeCharacterInfoEntry_NameNullTerminated(t *testing.T) {
	pv := uint32(20200401)
	raw := buildCharInfoB8(1, 0, 0, 0, 1, "Hi", "prontera.gat", 0, 0)

	var got events.CharacterInfoEntry
	decodeCharacterInfoEntry(&got, raw, pv)

	if got.Name != "Hi" {
		t.Errorf("Name: want %q, got %q", "Hi", got.Name)
	}
}

// TestDecodeCharacterInfoEntry_MapNameNullTerminated verifies map name with
// ".gat" suffix and null padding decodes to the full null-terminated string.
func TestDecodeCharacterInfoEntry_MapNameNullTerminated(t *testing.T) {
	pv := uint32(20200401)
	raw := buildCharInfoB8(1, 0, 0, 0, 1, "X", "gef_fild07.gat", 0, 0)

	var got events.CharacterInfoEntry
	decodeCharacterInfoEntry(&got, raw, pv)

	if got.MapName != "gef_fild07.gat" {
		t.Errorf("MapName: want %q, got %q", "gef_fild07.gat", got.MapName)
	}
}

// TestDecodeCharacterInfoEntry_TooShort verifies that a buffer shorter than the
// minimum entry size returns 0 and leaves the struct zeroed.
func TestDecodeCharacterInfoEntry_TooShort(t *testing.T) {
	pv := uint32(20030000) // B0, needs 112 bytes
	raw := make([]byte, 50)

	var got events.CharacterInfoEntry
	n := decodeCharacterInfoEntry(&got, raw, pv)

	if n != 0 {
		t.Errorf("expected 0 bytes consumed for short buffer, got %d", n)
	}
	if got.GID != 0 {
		t.Errorf("expected zeroed struct, GID=%d", got.GID)
	}
}

// hexToBytes converts a hex string (no spaces/separators) to []byte.
func hexToBytes(t *testing.T, h string) []byte {
	t.Helper()
	if len(h)%2 != 0 {
		t.Fatalf("odd hex string length %d", len(h))
	}
	b := make([]byte, len(h)/2)
	for i := range b {
		hi := hexNibble(h[i*2])
		lo := hexNibble(h[i*2+1])
		b[i] = hi<<4 | lo
	}
	return b
}

func hexNibble(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}
