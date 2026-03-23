// Hand-written: replaces codegen output which left CHARACTER_INFO arrays as []byte.
//
// CHARACTER_INFO wire sizes by PACKETVER breakpoint (MAIN client only):
//
//	pv < 20100720          : 112 bytes
//	pv >= 20100720, ≤20100727 : 128 bytes  (+mapName[16])
//	pv >= 20100803         : 132 bytes  (+mapName[16]+DelRevDate[4])
//	pv >= 20110111         : 136 bytes  (+robePalette[4])
//	pv >= 20110928         : 140 bytes  (+chr_slot_changeCnt[4])
//	pv >= 20111025         : 144 bytes  (+chr_name_changeCnt[4])
//	pv >= 20141016         : 145 bytes  (+sex[1])
//	pv >= 20141022         : 147 bytes  (+body[2] inserted after head, shifts subsequent offsets)
//	pv >= 20170830         : 155 bytes  (exp/jobexp int32→int64)
//	pv >= 20220330 MAIN    : 175 bytes  (hp/maxhp int32→int64, sp/maxsp int16→int64)
//
// Source: rAthena src/common/packets.hpp:31–105, verified via GCC preprocessor.
//
// Allocation note: decodeCharacterInfoList calls make([]CharacterInfoEntry, n) — one
// alloc per packet, unavoidable for variable-count arrays. Excluded from zero-alloc bench.
package decode

import "github.com/lenaxia/rathena-client/pkg/events"

// charInfoSize returns the wire size of one CHARACTER_INFO entry for the given packetver.
// Source: common/packets.hpp:31–105.
func charInfoSize(pv uint32) int {
	switch {
	case pv >= 20220330:
		return 175
	case pv >= 20170830:
		return 155
	case pv >= 20141022:
		return 147
	case pv >= 20141016:
		return 145
	case pv >= 20111025:
		return 144
	case pv >= 20110928:
		return 140
	case pv >= 20110111:
		return 136
	case pv >= 20100803:
		return 132
	case pv >= 20100720:
		return 128
	default:
		return 112
	}
}

// decodeCharacterInfoEntry reads one CHARACTER_INFO from b into dst.
// Returns the number of bytes consumed (== charInfoSize(pv)), or 0 if b is too short.
// b must begin at the first byte of the CHARACTER_INFO struct (i.e. GID field).
func decodeCharacterInfoEntry(dst *events.CharacterInfoEntry, b []byte, pv uint32) int {
	sz := charInfoSize(pv)
	if len(b) < sz {
		return 0
	}

	dst.GID = leU32(b, 0) // rAthena: GID (offset 0, size 4)

	if pv >= 20170830 {
		dst.Exp = leI64(b, 4)       // rAthena: exp int64 (offset 4, size 8)
		dst.JobExp = leI64(b, 16)   // rAthena: jobexp int64 (offset 16, size 8)
		dst.JobLevel = leI32(b, 24) // rAthena: joblevel int32 (offset 24; OpenKore: lv_job)
	} else {
		dst.Exp = int64(leI32(b, 4))     // rAthena: exp int32 (offset 4, size 4)
		dst.JobExp = int64(leI32(b, 12)) // rAthena: jobexp int32 (offset 12, size 4)
		dst.JobLevel = leI32(b, 16)      // rAthena: joblevel int32 (offset 16; OpenKore: lv_job)
	}

	// hp/maxhp/sp/maxsp offsets depend on exp/jobexp width.
	// Pre-20170830: jobpoint at 40, hp at 42.
	// Post-20170830: jobpoint at 48, hp at 50 (exp+8, jobexp+8 = +8 total shift).
	// speed(int16) immediately follows maxsp in all versions.
	if pv >= 20220330 {
		// hp/maxhp: int64 at +50; sp/maxsp: int64 at +66/+74; speed at +82
		dst.HP = leI64(b, 50)    // rAthena: hp int64
		dst.MaxHP = leI64(b, 58) // rAthena: maxhp int64
		dst.SP = leI64(b, 66)    // rAthena: sp int64
		dst.MaxSP = leI64(b, 74) // rAthena: maxsp int64
		dst.Speed = leI16(b, 82) // rAthena: speed int16 (OpenKore: walkspeed)
	} else if pv >= 20170830 {
		// hp/maxhp: int32 at +50; sp/maxsp: int16 at +58/+60; speed at +62
		dst.HP = int64(leI32(b, 50))    // rAthena: hp int32
		dst.MaxHP = int64(leI32(b, 54)) // rAthena: maxhp int32
		dst.SP = int64(leI16(b, 58))    // rAthena: sp int16
		dst.MaxSP = int64(leI16(b, 60)) // rAthena: maxsp int16
		dst.Speed = leI16(b, 62)        // rAthena: speed int16 (OpenKore: walkspeed)
	} else {
		// hp/maxhp: int32 at +42; sp/maxsp: int16 at +50/+52; speed at +54
		dst.HP = int64(leI32(b, 42))    // rAthena: hp int32
		dst.MaxHP = int64(leI32(b, 46)) // rAthena: maxhp int32
		dst.SP = int64(leI16(b, 50))    // rAthena: sp int16
		dst.MaxSP = int64(leI16(b, 52)) // rAthena: maxsp int16
		dst.Speed = leI16(b, 54)        // rAthena: speed int16 (OpenKore: walkspeed)
	}

	// job and level offsets depend on whether body(int16) is present (pv >= 20141022).
	// body is inserted after head, shifting weapon/level/sppoint/accessory/etc +2.
	// name[24] and CharNum follow bodypalette.
	if pv >= 20220330 {
		// speed=82 job=84 head=86 body=88 weapon=90 level=92
		// name=108 CharNum=138
		dst.Job = leI16(b, 84)                   // rAthena: job
		dst.Level = leI16(b, 92)                 // rAthena: level
		dst.Name = nullTermString(b[108:132])    // rAthena: name[24]
		dst.CharNum = b[138]                     // rAthena: CharNum
		dst.MapName = nullTermString(b[142:158]) // rAthena: mapName[16]
		dst.Sex = b[174]                         // rAthena: sex
	} else if pv >= 20170830 {
		// speed=62 job=64 head=66 body=68 weapon=70 level=72
		// name=88 CharNum=118 mapName=122 sex=154
		dst.Job = leI16(b, 64)                   // rAthena: job
		dst.Level = leI16(b, 72)                 // rAthena: level
		dst.Name = nullTermString(b[88:112])     // rAthena: name[24]
		dst.CharNum = b[118]                     // rAthena: CharNum
		dst.MapName = nullTermString(b[122:138]) // rAthena: mapName[16]
		dst.Sex = b[154]                         // rAthena: sex
	} else if pv >= 20141022 {
		// body present; speed=54 job=56 head=58 body=60 weapon=62 level=64
		// name=80 CharNum=110 mapName=114 sex=146
		dst.Job = leI16(b, 56)                   // rAthena: job
		dst.Level = leI16(b, 64)                 // rAthena: level
		dst.Name = nullTermString(b[80:104])     // rAthena: name[24]
		dst.CharNum = b[110]                     // rAthena: CharNum
		dst.MapName = nullTermString(b[114:130]) // rAthena: mapName[16]
		dst.Sex = b[146]                         // rAthena: sex
	} else if pv >= 20141016 {
		// sex present but no body; speed=54 job=56 head=58 weapon=60 level=62
		// name=78 CharNum=108 mapName=112 sex=144
		dst.Job = leI16(b, 56)                   // rAthena: job
		dst.Level = leI16(b, 62)                 // rAthena: level
		dst.Name = nullTermString(b[78:102])     // rAthena: name[24]
		dst.CharNum = b[108]                     // rAthena: CharNum
		dst.MapName = nullTermString(b[112:128]) // rAthena: mapName[16]
		dst.Sex = b[144]                         // rAthena: sex
	} else if pv >= 20100803 {
		// no body, no sex; speed=54 job=56 weapon=60 level=62
		// name=78 CharNum=108 mapName=112
		dst.Job = leI16(b, 56)                   // rAthena: job
		dst.Level = leI16(b, 62)                 // rAthena: level
		dst.Name = nullTermString(b[78:102])     // rAthena: name[24]
		dst.CharNum = b[108]                     // rAthena: CharNum
		dst.MapName = nullTermString(b[112:128]) // rAthena: mapName[16]
	} else if pv >= 20100720 {
		// +mapName[16] only; no DelRevDate yet
		// name=78 CharNum=108 mapName=112 (128-byte struct ends after mapName)
		dst.Job = leI16(b, 56)                   // rAthena: job
		dst.Level = leI16(b, 62)                 // rAthena: level
		dst.Name = nullTermString(b[78:102])     // rAthena: name[24]
		dst.CharNum = b[108]                     // rAthena: CharNum
		dst.MapName = nullTermString(b[112:128]) // rAthena: mapName[16]
	} else {
		// baseline: no mapName, no body, no sex
		// speed=54 job=56 weapon=60 level=62 name=78 CharNum=108
		dst.Job = leI16(b, 56)               // rAthena: job
		dst.Level = leI16(b, 62)             // rAthena: level
		dst.Name = nullTermString(b[78:102]) // rAthena: name[24]
		dst.CharNum = b[108]                 // rAthena: CharNum
	}

	return sz
}

// DecodeCharacterInfoList decodes the CHARACTER_INFO flex array in body into a slice.
// body is the raw bytes starting immediately after the enclosing packet header.
// Trailing bytes shorter than one entry are silently ignored.
func DecodeCharacterInfoList(body []byte, pv uint32) []events.CharacterInfoEntry {
	return decodeCharacterInfoList(body, pv)
}

// decodeCharacterInfoList decodes the CHARACTER_INFO flex array in body into a slice.
// body is the raw bytes starting immediately after the enclosing packet header.
// Trailing bytes shorter than one entry are silently ignored.
func decodeCharacterInfoList(body []byte, pv uint32) []events.CharacterInfoEntry {
	sz := charInfoSize(pv)
	n := len(body) / sz
	if n == 0 {
		return nil
	}
	entries := make([]events.CharacterInfoEntry, n)
	for i := range entries {
		decodeCharacterInfoEntry(&entries[i], body[i*sz:], pv)
	}
	return entries
}
