// Hand-written: codegen leaves CHARACTER_INFO arrays as []byte; this type decodes them.
// Source: rAthena src/common/packets.hpp:31–105 (CHARACTER_INFO struct)
//
// PACKETVER breakpoints (MAIN client, verified via GCC preprocessor):
//
//	pv < 20100720          : 112 bytes — baseline
//	pv >= 20100720 (≤0727) : 128 bytes — +mapName[16]  (narrow window, same as 20100803 path for us)
//	pv >= 20100803         : 132 bytes — +mapName[16]+DelRevDate(4)
//	pv >= 20110111         : 136 bytes — +robePalette(4)
//	pv >= 20110928         : 140 bytes — +chr_slot_changeCnt(4)
//	pv >= 20111025         : 144 bytes — +chr_name_changeCnt(4)
//	pv >= 20141016         : 145 bytes — +sex(1)
//	pv >= 20141022         : 147 bytes — +body(2) inserted after head (shifts all subsequent offsets +2)
//	pv >= 20170830         : 155 bytes — exp/jobexp int32→int64 (+8 total)
//	pv >= 20220330 (MAIN)  : 175 bytes — hp/maxhp int32→int64 (+8), sp/maxsp int16→int64 (+12)
//
// Note: PACKETVER_RE_NUM >= 20211103 also widens hp/sp but goKore targets MAIN only.
// See docs/BACKLOG/TECH-DEBT-01_packetver-re-zero-support.md.
//
// All PACKETVER-conditional field widths are normalised to their widest form so
// callers never branch on packetver.
package events

// CharacterInfoEntry is the decoded form of one CHARACTER_INFO element from a
// char-server packet (HC_ACCEPT_ENTER 0x006B, HC_ACK_CHARINFO_PER_PAGE 0x099D/0x0B72).
//
// rAthena source: common/packets.hpp:31–105
type CharacterInfoEntry struct {
	GID     uint32 // rAthena: GID
	Exp     int64  // rAthena: exp     (int32 pv<20170830, int64 pv>=20170830)
	JobExp  int64  // rAthena: jobexp  (same breakpoint as exp)
	HP      int64  // rAthena: hp      (int32 pv<20220330 MAIN, int64 pv>=20220330)
	MaxHP   int64  // rAthena: maxhp   (same breakpoint as hp)
	SP      int64  // rAthena: sp      (int16 pv<20220330 MAIN, int64 pv>=20220330)
	MaxSP   int64  // rAthena: maxsp   (same breakpoint as sp)
	Job     int16  // rAthena: job
	Level   int16  // rAthena: level
	Name    string // rAthena: name[24]
	MapName string // rAthena: mapName[16] (present pv>=20100720, empty otherwise)
	CharNum uint8  // rAthena: CharNum — slot index (0–8)
	Sex     uint8  // rAthena: sex (present pv>=20141016, 0 otherwise)
}
