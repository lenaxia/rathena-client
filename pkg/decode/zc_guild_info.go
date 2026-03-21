// Hand-written: replaces generated ZcGuildInfo_0x0A84 whose else branch applied the
// 0x01B6 layout (masterName before manageLand). PACKET_ZC_GUILD_INFO for 0x0A84 has
// no masterName field — manageLand is at offset 70 for all pv >= 20161019.
// Source: packets_struct.hpp:4830-4848.

package decode

import "github.com/lenaxia/rathena-client/pkg/events"

// ZcGuildInfo_0x0A84 decodes a 0x0A84 packet (struct PACKET_ZC_GUILD_INFO).
// Active: PACKETVER_MAIN_NUM >= 20161019, < 20200902.
func ZcGuildInfo_0x0A84(data []byte, packetver uint32) events.ZcGuildInfo {
	var e events.ZcGuildInfo
	_ = packetver
	e.GDID = data[2:]                          // rAthena: GDID (offset 2, size 4)
	e.Level = data[6:]                         // rAthena: level (offset 6, size 4)
	e.UserNum = data[10:]                      // rAthena: userNum (offset 10, size 4)
	e.MaxUserNum = data[14:]                   // rAthena: maxUserNum (offset 14, size 4)
	e.UserAverageLevel = data[18:]             // rAthena: userAverageLevel (offset 18, size 4)
	e.Exp = data[22:]                          // rAthena: exp (offset 22, size 4)
	e.MaxExp = data[26:]                       // rAthena: maxExp (offset 26, size 4)
	e.Point = data[30:]                        // rAthena: point (offset 30, size 4)
	e.Honor = data[34:]                        // rAthena: honor (offset 34, size 4)
	e.Virtue = data[38:]                       // rAthena: virtue (offset 38, size 4)
	e.EmblemVersion = data[42:]                // rAthena: emblemVersion (offset 42, size 4)
	e.Guildname = nullTermString(data[46:70])  // rAthena: guildname (offset 46, size 24)
	e.ManageLand = nullTermString(data[70:86]) // rAthena: manageLand (offset 70, size 16)
	e.Zeny = data[86:]                         // rAthena: zeny (offset 86, size 4)
	e.MasterGID = data[90:]                    // rAthena: masterGID (offset 90, size 4)
	return e
}

// ZcGuildInfo_0x01B6 decodes a 0x01B6 packet (struct PACKET_ZC_GUILD_INFO).
func ZcGuildInfo_0x01B6(data []byte, packetver uint32) events.ZcGuildInfo {
	var e events.ZcGuildInfo
	_ = packetver
	e.GDID = data[2:]                           // rAthena: GDID (offset 2, size 4)
	e.Level = data[6:]                          // rAthena: level (offset 6, size 4)
	e.UserNum = data[10:]                       // rAthena: userNum (offset 10, size 4)
	e.MaxUserNum = data[14:]                    // rAthena: maxUserNum (offset 14, size 4)
	e.UserAverageLevel = data[18:]              // rAthena: userAverageLevel (offset 18, size 4)
	e.Exp = data[22:]                           // rAthena: exp (offset 22, size 4)
	e.MaxExp = data[26:]                        // rAthena: maxExp (offset 26, size 4)
	e.Point = data[30:]                         // rAthena: point (offset 30, size 4)
	e.Honor = data[34:]                         // rAthena: honor (offset 34, size 4)
	e.Virtue = data[38:]                        // rAthena: virtue (offset 38, size 4)
	e.EmblemVersion = data[42:]                 // rAthena: emblemVersion (offset 42, size 4)
	e.Guildname = nullTermString(data[46:70])   // rAthena: guildname (offset 46, size 24)
	e.MasterName = nullTermString(data[70:94])  // rAthena: masterName (offset 70, size 24)
	e.ManageLand = nullTermString(data[94:110]) // rAthena: manageLand (offset 94, size 16)
	e.Zeny = data[110:]                         // rAthena: zeny (offset 110, size 4)
	return e
}

// ZcGuildInfo_0x0B7B decodes a 0x0B7B packet (struct PACKET_ZC_GUILD_INFO).
func ZcGuildInfo_0x0B7B(data []byte, packetver uint32) events.ZcGuildInfo {
	var e events.ZcGuildInfo
	if packetver >= 20200916 {
		e.GDID = data[2:]                           // rAthena: GDID (offset 2, size 4)
		e.Level = data[6:]                          // rAthena: level (offset 6, size 4)
		e.UserNum = data[10:]                       // rAthena: userNum (offset 10, size 4)
		e.MaxUserNum = data[14:]                    // rAthena: maxUserNum (offset 14, size 4)
		e.UserAverageLevel = data[18:]              // rAthena: userAverageLevel (offset 18, size 4)
		e.Exp = data[22:]                           // rAthena: exp (offset 22, size 4)
		e.MaxExp = data[26:]                        // rAthena: maxExp (offset 26, size 4)
		e.Point = data[30:]                         // rAthena: point (offset 30, size 4)
		e.Honor = data[34:]                         // rAthena: honor (offset 34, size 4)
		e.Virtue = data[38:]                        // rAthena: virtue (offset 38, size 4)
		e.EmblemVersion = data[42:]                 // rAthena: emblemVersion (offset 42, size 4)
		e.Guildname = nullTermString(data[46:70])   // rAthena: guildname (offset 46, size 24)
		e.ManageLand = nullTermString(data[70:86])  // rAthena: manageLand (offset 70, size 16)
		e.Zeny = data[86:]                          // rAthena: zeny (offset 86, size 4)
		e.MasterGID = data[90:]                     // rAthena: masterGID (offset 90, size 4)
		e.MasterName = nullTermString(data[94:118]) // rAthena: masterName (offset 94, size 24)
	} else {
		e.GDID = data[2:]                          // rAthena: GDID (offset 2, size 4)
		e.Level = data[6:]                         // rAthena: level (offset 6, size 4)
		e.UserNum = data[10:]                      // rAthena: userNum (offset 10, size 4)
		e.MaxUserNum = data[14:]                   // rAthena: maxUserNum (offset 14, size 4)
		e.UserAverageLevel = data[18:]             // rAthena: userAverageLevel (offset 18, size 4)
		e.Exp = data[22:]                          // rAthena: exp (offset 22, size 4)
		e.MaxExp = data[26:]                       // rAthena: maxExp (offset 26, size 4)
		e.Point = data[30:]                        // rAthena: point (offset 30, size 4)
		e.Honor = data[34:]                        // rAthena: honor (offset 34, size 4)
		e.Virtue = data[38:]                       // rAthena: virtue (offset 38, size 4)
		e.EmblemVersion = data[42:]                // rAthena: emblemVersion (offset 42, size 4)
		e.Guildname = nullTermString(data[46:70])  // rAthena: guildname (offset 46, size 24)
		e.ManageLand = nullTermString(data[70:86]) // rAthena: manageLand (offset 70, size 16)
		e.Zeny = data[86:]                         // rAthena: zeny (offset 86, size 4)
		e.MasterGID = data[90:]                    // rAthena: masterGID (offset 90, size 4)
	}
	return e
}
